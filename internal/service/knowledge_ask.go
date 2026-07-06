package service

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/singll/bellkeeper/internal/llmclient"
	"github.com/singll/bellkeeper/internal/llmgateway"
)

// ChatTurn 一轮对话历史
type ChatTurn struct {
	Role    string `json:"role"`    // user | assistant
	Content string `json:"content"`
}

// AskRequest 问答请求
type AskRequest struct {
	Question string     `json:"question"`
	Layers   []string   `json:"layers,omitempty"`
	TopK     int        `json:"top_k,omitempty"`
	History  []ChatTurn `json:"history,omitempty"` // 多轮上下文（前端传最近 N 轮，后端再封顶）
}

// maxHistoryMessages 后端拼入 LLM 的历史消息数上限（防上下文膨胀，6 轮 ≈ 12 条）
const maxHistoryMessages = 12

// AskResponse 问答响应
type AskResponse struct {
	Answer     string      `json:"answer"`
	References []Reference `json:"references"`
	SearchMs   int64       `json:"search_ms"`
	LLMMs      int64       `json:"llm_ms"`
}

// Reference 引用
type Reference struct {
	Title     string `json:"title"`
	FilePath  string `json:"file_path"`
	SourceURL string `json:"source_url,omitempty"`
	Snippet   string `json:"snippet"`
}

type AskService struct {
	searchService   *FileSearchService
	llm             llmgateway.Gateway
	allowedLayers   map[string]struct{}
	maxContextRunes int
	promptLoader    *KBPromptLoader
}

func NewAskService(search *FileSearchService, llm llmgateway.Gateway) *AskService {
	return &AskService{
		searchService: search,
		llm:           llm,
	}
}

// SetPromptLoader 注入知识库提示词加载器（1.0 §2.1.3）。
func (s *AskService) SetPromptLoader(loader *KBPromptLoader) {
	s.promptLoader = loader
}

func (s *AskService) SetAllowedLayers(layers []string) {
	s.allowedLayers = make(map[string]struct{}, len(layers))
	for _, layer := range layers {
		if layer != "" {
			s.allowedLayers[layer] = struct{}{}
		}
	}
}

func (s *AskService) SetMaxContextRunes(n int) {
	s.maxContextRunes = n
}

// Ask 问答
//
// 1.0 §2.1.3：召回 top-20 → rerank → top-5 → buildContext。
// rerank 经 Gateway.Rerank 进程内直调代理池 /v1/rerank，无 rerank channel 时
// 降级为 Meili 原始顺序取 top-5（不阻断问答）。
func (s *AskService) Ask(ctx context.Context, req AskRequest) (*AskResponse, error) {
	if err := s.validateLayers(req.Layers); err != nil {
		return nil, err
	}

	// 1. 召回 top-20（rerank 候选集）
	recallLimit := 20
	if req.TopK > 0 {
		// TopK 用户指定的是最终返回数量；召回扩大到 max(20, TopK+5)。
		if req.TopK+5 > recallLimit {
			recallLimit = req.TopK + 5
		}
	}
	searchReq := FileSearchRequest{
		Query:  req.Question,
		Layers: req.Layers,
		Limit:  recallLimit,
	}

	searchStart := time.Now()
	searchResult, err := s.searchService.Search(ctx, searchReq)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	searchMs := time.Since(searchStart).Milliseconds()

	// 2. rerank → top-5（finalTopK）
	finalTopK := 5
	if req.TopK > 0 {
		finalTopK = req.TopK
	}
	reranked := s.rerankHits(ctx, req.Question, searchResult.Files, finalTopK)

	// 3. 组装上下文（无命中时为空，仍走通用回答）
	var contextContent string
	if len(reranked) > 0 {
		contextContent = s.buildContext(reranked)
	}

	// 4. 调用 LLM
	llmStart := time.Now()
	answer, err := s.callLLM(ctx, req.Question, contextContent, req.History)
	if err != nil {
		return nil, fmt.Errorf("call llm: %w", err)
	}
	llmMs := time.Since(llmStart).Milliseconds()

	// 5. 构建引用（用 rerank 后的结果）
	refs := make([]Reference, 0, len(reranked))
	for _, f := range reranked {
		ref := Reference{
			Title:     f.Title,
			FilePath:  f.FilePath,
			SourceURL: f.SourceURL,
		}
		if len(f.Snippets) > 0 {
			ref.Snippet = f.Snippets[0]
		}
		refs = append(refs, ref)
	}

	return &AskResponse{
		Answer:     answer,
		References: refs,
		SearchMs:   searchMs,
		LLMMs:      llmMs,
	}, nil
}

// AskStreamChunk 是 SSE 流式问答的单个推送单元。
type AskStreamChunk struct {
	Type string      `json:"type"` // "references" | "delta" | "done" | "error"
	Data interface{} `json:"data"`
}

// AskStream 流式问答（1.0 §4 [fe] 知识问答 SSE 流式）。
//
// 实现搜索+rerank 同步完成，然后通过 Gateway.ChatStream 实现真 token 级流式，
// 将 stream chunk 转换为 SSE delta event 实时推送到前端。
// 消费者（handler）读取 channel 并转 SSE；ctx 取消时关闭 channel。
func (s *AskService) AskStream(ctx context.Context, req AskRequest) <-chan AskStreamChunk {
	ch := make(chan AskStreamChunk, 16)
	go func() {
		defer close(ch)

		llmStart := time.Now()
		searchMs := int64(0)

		// 1. 校验层
		if err := s.validateLayers(req.Layers); err != nil {
			ch <- AskStreamChunk{Type: "error", Data: err.Error()}
			return
		}

		// 2. 召回 + rerank（同步，结果决定 refs）
		recallLimit := 20
		if req.TopK > 0 {
			if req.TopK+5 > recallLimit {
				recallLimit = req.TopK + 5
			}
		}
		searchStart := time.Now()
		searchResult, err := s.searchService.Search(ctx, FileSearchRequest{
			Query:  req.Question,
			Layers: req.Layers,
			Limit:  recallLimit,
		})
		if err != nil {
			ch <- AskStreamChunk{Type: "error", Data: "search failed: " + err.Error()}
			return
		}
		searchMs = time.Since(searchStart).Milliseconds()

		finalTopK := 5
		if req.TopK > 0 {
			finalTopK = req.TopK
		}
		reranked := s.rerankHits(ctx, req.Question, searchResult.Files, finalTopK)
		var contextContent string
		if len(reranked) > 0 {
			contextContent = s.buildContext(reranked)
		}

		// 3. 先推引用（search+rerank 结果），前端可立即展示
		refs := make([]Reference, 0, len(reranked))
		for _, f := range reranked {
			ref := Reference{
				Title:     f.Title,
				FilePath:  f.FilePath,
				SourceURL: f.SourceURL,
			}
			if len(f.Snippets) > 0 {
				ref.Snippet = f.Snippets[0]
			}
			refs = append(refs, ref)
		}
		select {
		case <-ctx.Done():
			return
		case ch <- AskStreamChunk{Type: "references", Data: refs}:
		}

		// 4. 构造 system prompt + messages
		systemPrompt := `你是 Bellkeeper 的 AI 助手。
- 若提供了知识库片段：优先据其回答，并在相关处标注来源卡片标题（只读引用，不复制正文）。
- 若片段为空或不相关：作为通用助手正常回答，不要回「未找到」。
- 不要编造知识库里没有的事实来源。中文回答，简洁实用。`
		if s.promptLoader != nil {
			systemPrompt = s.promptLoader.GetWithDefault("knowledge_ask_system", systemPrompt)
		}

		messages := make([]llmclient.ChatMessage, 0, len(req.History)+2)
		messages = append(messages, llmclient.ChatMessage{Role: "system", Content: systemPrompt})
		if n := len(req.History); n > 0 {
			start := 0
			if n > maxHistoryMessages {
				start = n - maxHistoryMessages
			}
			for _, turn := range req.History[start:] {
				if turn.Role == "" || turn.Content == "" {
					continue
				}
				messages = append(messages, llmclient.ChatMessage{Role: turn.Role, Content: turn.Content})
			}
		}
		var userContent string
		if contextContent != "" {
			userContent = fmt.Sprintf("问题: %s\n\n相关文档:\n%s", req.Question, contextContent)
		} else {
			userContent = req.Question
		}
		messages = append(messages, llmclient.ChatMessage{Role: "user", Content: userContent})

		// 5. 进程内直调流式 LLM，逐 token 推送 delta
		streamResult, err := s.llm.ChatStream(ctx, llmclient.ChatRequest{
			Model:       "pool-chat-balanced",
			Messages:    messages,
			Temperature: 0.3,
		}, llmclient.ChatOptions{CallerID: "knowledge-ask", TaskType: "qa"})

		if err != nil {
			ch <- AskStreamChunk{Type: "error", Data: "llm stream failed: " + err.Error()}
			return
		}

		llmMs := time.Since(llmStart).Milliseconds()
		for token := range streamResult.Tokens {
			select {
			case <-ctx.Done():
				return
			case ch <- AskStreamChunk{Type: "delta", Data: token}:
			}
		}

		// 6. 推送完成事件（含计时）
		ch <- AskStreamChunk{Type: "done", Data: map[string]int64{"search_ms": searchMs, "llm_ms": llmMs}}
	}()
	return ch
}

// rerankHits 用 Gateway.Rerank 对召回结果重排，返回 topN。
// rerank 失败或未配置 rerank channel 时降级为原始顺序取 topN（不阻断问答）。
func (s *AskService) rerankHits(ctx context.Context, query string, hits []FileHit, topN int) []FileHit {
	if len(hits) <= topN {
		return hits
	}
	if s.llm == nil {
		if len(hits) > topN {
			return hits[:topN]
		}
		return hits
	}
	// 构造 documents：每条用 title + 首个 snippet 作为 rerank 文本。
	docs := make([]string, len(hits))
	for i, h := range hits {
		text := h.Title
		if len(h.Snippets) > 0 && h.Snippets[0] != "" {
			text = h.Title + "\n" + h.Snippets[0]
		}
		docs[i] = text
	}
	resp, err := s.llm.Rerank(ctx, llmclient.RerankRequest{
		Model:     "pool-rerank",
		Query:     query,
		Documents: docs,
		TopN:      topN,
	})
	if err != nil || len(resp.Results) == 0 {
		// 降级：Meili 原始顺序取 topN
		return hits[:topN]
	}
	out := make([]FileHit, 0, len(resp.Results))
	for _, r := range resp.Results {
		if r.Index >= 0 && r.Index < len(hits) {
			out = append(out, hits[r.Index])
		}
	}
	if len(out) == 0 {
		return hits[:topN]
	}
	return out
}

// buildContext 构建 LLM 上下文（1.0 §2.1.3：rerank 后 top-5 片段拼接，每片段限长）。
// snippet 本身已是 Meili 高亮片段（摘要性质），无需额外 LLM 摘要压缩（成本/延迟考量）。
func (s *AskService) buildContext(files []FileHit) string {
	const defaultMaxLen = 6000
	maxLen := defaultMaxLen
	if s.maxContextRunes > 0 {
		maxLen = s.maxContextRunes
	}
	const perSnippetMax = 1200 // 单片段上限，避免一长文挤占其他片段
	var ctx bytes.Buffer

	for i, f := range files {
		ctx.WriteString(fmt.Sprintf("【片段%d】%s (%s)\n", i+1, f.Title, f.FilePath))
		if len(f.Snippets) > 0 {
			snippet := f.Snippets[0]
			if r := []rune(snippet); len(r) > perSnippetMax {
				snippet = string(r[:perSnippetMax]) + "…"
			}
			ctx.WriteString(snippet)
		}
		ctx.WriteString("\n\n")
	}

	runes := []rune(ctx.String())
	if len(runes) > maxLen {
		runes = runes[:maxLen]
	}

	return string(runes)
}

func (s *AskService) callLLM(ctx context.Context, question, contextContent string, history []ChatTurn) (string, error) {
	// 1.0 §2.1.3：system prompt 外置到 config/prompts/knowledge_ask_system.md，
	// loader 缺失时回退到内置默认（保证服务可启动）。
	systemPrompt := `你是 Bellkeeper 的 AI 助手。
- 若提供了知识库片段：优先据其回答，并在相关处标注来源卡片标题（只读引用，不复制正文）。
- 若片段为空或不相关：作为通用助手正常回答，不要回「未找到」。
- 不要编造知识库里没有的事实来源。中文回答，简洁实用。`
	if s.promptLoader != nil {
		systemPrompt = s.promptLoader.GetWithDefault("knowledge_ask_system", systemPrompt)
	}

	messages := make([]llmclient.ChatMessage, 0, len(history)+2)
	messages = append(messages, llmclient.ChatMessage{Role: "system", Content: systemPrompt})

	// 拼入最近 maxHistoryMessages 条历史（取尾部，防上下文膨胀）
	if n := len(history); n > 0 {
		start := 0
		if n > maxHistoryMessages {
			start = n - maxHistoryMessages
		}
		for _, turn := range history[start:] {
			if turn.Role == "" || turn.Content == "" {
				continue
			}
			messages = append(messages, llmclient.ChatMessage{Role: turn.Role, Content: turn.Content})
		}
	}

	// 当前问题：命中时附知识库片段，未命中时只含问题
	var userContent string
	if contextContent != "" {
		userContent = fmt.Sprintf("问题: %s\n\n相关文档:\n%s", question, contextContent)
	} else {
		userContent = question
	}
	messages = append(messages, llmclient.ChatMessage{Role: "user", Content: userContent})

	resp, err := s.llm.Chat(
		ctx,
		llmclient.ChatRequest{
			Model:       "pool-chat-balanced",
			Messages:    messages,
			Temperature: 0.3,
		},
		llmclient.ChatOptions{CallerID: "knowledge-ask", TaskType: "qa"},
	)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func (s *AskService) validateLayers(layers []string) error {
	if len(s.allowedLayers) == 0 || len(layers) == 0 {
		return nil
	}
	for _, layer := range layers {
		if _, ok := s.allowedLayers[layer]; !ok {
			return fmt.Errorf("invalid knowledge layer: %s", layer)
		}
	}
	return nil
}
