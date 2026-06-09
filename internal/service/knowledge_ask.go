package service

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/singll/bellkeeper/internal/llmclient"
	"github.com/singll/bellkeeper/internal/model"
)

// AskRequest 问答请求
type AskRequest struct {
	Question string   `json:"question"`
	Layers   []string `json:"layers,omitempty"`
	TopK     int      `json:"top_k,omitempty"`
}

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

// AskService 问答服务
type AskService struct {
	searchService *FileSearchService
	llmClient     *llmclient.Client
	llmJobs       *LLMJobQueueService
	allowedLayers map[string]struct{}
}

// NewAskService 创建问答服务
func NewAskService(search *FileSearchService, llmProxyURL, apiKey string, queue ...*LLMJobQueueService) *AskService {
	var llmJobs *LLMJobQueueService
	if len(queue) > 0 {
		llmJobs = queue[0]
	}
	return &AskService{
		searchService: search,
		llmJobs:       llmJobs,
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

	if len(searchResult.Files) == 0 {
		return &AskResponse{
			Answer:     "知识库中未找到相关信息",
			References: []Reference{},
			SearchMs:   searchMs,
		}, nil
	}

	// 2. 组装上下文
	context := s.buildContext(searchResult.Files)

	// 3. 调用 LLM
	llmStart := time.Now()
	answer, err := s.callLLM(ctx, req.Question, context)
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
	const maxLen = 6000
	var ctx bytes.Buffer

	for _, f := range files {
		ctx.WriteString(fmt.Sprintf("【文件】%s (%s)\n", f.Title, f.FilePath))
		if len(f.Snippets) > 0 {
			ctx.WriteString(f.Snippets[0])
		}
		ctx.WriteString("\n\n")
	}

	content := ctx.String()
	if len(content) > maxLen {
		content = content[:maxLen]
	}

	return content
}

// callLLM 调用 LLM
func (s *AskService) callLLM(ctx context.Context, question, contextContent string) (string, error) {
	systemPrompt := `你是一个知识库助手。基于以下知识库内容回答问题。
要求:
1. 只基于提供的内容回答，不要编造
2. 在回答中引用来源文件名
3. 如果知识库中没有相关信息，明确告知用户`

	messages := []llmclient.ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: fmt.Sprintf("问题: %s\n\n相关文档:\n%s", question, contextContent)},
	}
	if s.llmJobs != nil {
		job, err := s.llmJobs.EnqueueChat(EnqueueLLMChatOptions{
			TaskType:       "qa",
			CallerID:       "knowledge-ask",
			Model:          "pool-chat-balanced",
			Messages:       messages,
			Temperature:    0.3,
			Priority:       50,
			IdempotencyKey: llmJobIdempotencyKey("qa", "pool-chat-balanced", question, contextContent),
		})
		if err != nil {
			return "", err
		}
		done, err := s.llmJobs.Wait(ctx, job.ID, time.Second)
		if err != nil {
			return "", err
		}
		if done.Status != model.LLMJobSuccess {
			return "", LLMJobTerminalError(done)
		}
		return done.ResponseText, nil
	}
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
