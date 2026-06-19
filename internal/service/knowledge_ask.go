package service

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/singll/bellkeeper/internal/llmclient"
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
	llmClient       *llmclient.Client
	allowedLayers   map[string]struct{}
	maxContextRunes int
}

func NewAskService(search *FileSearchService, llmProxyURL, apiKey string) *AskService {
	return &AskService{
		searchService: search,
		llmClient: llmclient.New(llmclient.Options{
			BaseURL: llmProxyURL,
			APIKey:  apiKey,
			Timeout: 90 * time.Second,
		}),
	}
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
func (s *AskService) Ask(ctx context.Context, req AskRequest) (*AskResponse, error) {
	if req.TopK <= 0 {
		req.TopK = 5
	}
	if err := s.validateLayers(req.Layers); err != nil {
		return nil, err
	}

	// 1. 搜索相关文档
	searchReq := FileSearchRequest{
		Query:  req.Question,
		Layers: req.Layers,
		Limit:  req.TopK,
	}

	searchStart := time.Now()
	searchResult, err := s.searchService.Search(ctx, searchReq)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	searchMs := time.Since(searchStart).Milliseconds()

	// 2. 组装上下文（无命中时为空，仍走通用回答，不再拒答）
	var contextContent string
	if len(searchResult.Files) > 0 {
		contextContent = s.buildContext(searchResult.Files)
	}

	// 3. 调用 LLM（命中=优先据库回答，未命中=通用助手）
	llmStart := time.Now()
	answer, err := s.callLLM(ctx, req.Question, contextContent, req.History)
	if err != nil {
		return nil, fmt.Errorf("call llm: %w", err)
	}
	llmMs := time.Since(llmStart).Milliseconds()

	// 4. 构建引用
	refs := make([]Reference, 0, len(searchResult.Files))
	for _, f := range searchResult.Files {
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

// buildContext 构建 LLM 上下文
func (s *AskService) buildContext(files []FileHit) string {
	const defaultMaxLen = 6000
	maxLen := defaultMaxLen
	if s.maxContextRunes > 0 {
		maxLen = s.maxContextRunes
	}
	var ctx bytes.Buffer

	for _, f := range files {
		ctx.WriteString(fmt.Sprintf("【文件】%s (%s)\n", f.Title, f.FilePath))
		if len(f.Snippets) > 0 {
			ctx.WriteString(f.Snippets[0])
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
	systemPrompt := `你是 Bellkeeper 的 AI 助手。
- 若提供了知识库片段：优先据其回答，并在相关处标注来源卡片标题（只读引用，不复制正文）。
- 若片段为空或不相关：作为通用助手正常回答，不要回「未找到」。
- 不要编造知识库里没有的事实来源。中文回答，简洁实用。`

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

	return s.llmClient.ChatCompletion(
		ctx,
		llmclient.ChatRequest{
			Model:       "pool-chat-balanced",
			Messages:    messages,
			Temperature: 0.3,
		},
		llmclient.ChatOptions{CallerID: "knowledge-ask", TaskType: "qa"},
	)
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
