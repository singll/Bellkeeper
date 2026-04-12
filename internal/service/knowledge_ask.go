package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/singll/bellkeeper/internal/pkg/httpclient"
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
	llmProxyURL  string
	apiKey       string
	httpClient   *httpclient.Client
}

// NewAskService 创建问答服务
func NewAskService(search *FileSearchService, llmProxyURL, apiKey string) *AskService {
	return &AskService{
		searchService: search,
		llmProxyURL:  llmProxyURL,
		apiKey:      apiKey,
		httpClient:  httpclient.NewClientWithTimeout(60 * time.Second),
	}
}

// Ask 问答
func (s *AskService) Ask(ctx context.Context, req AskRequest) (*AskResponse, error) {
	if req.TopK <= 0 {
		req.TopK = 5
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

	payload := map[string]interface{}{
		"model": "pool-chat-balanced",
		"messages": []map[string]interface{}{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": fmt.Sprintf("问题: %s\n\n相关文档:\n%s", question, contextContent)},
		},
		"temperature": 0.3,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}

	url := s.llmProxyURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-Key", s.apiKey)

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("llm returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	choices, ok := result["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return "", fmt.Errorf("invalid response format: missing choices")
	}

	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid response format: invalid choice")
	}

	msg, ok := choice["message"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid response format: missing message")
	}

	content, ok := msg["content"].(string)
	if !ok {
		return "", fmt.Errorf("invalid response format: missing content")
	}

	return content, nil
}
