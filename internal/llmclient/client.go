package llmclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Client is the shared internal caller for Bellkeeper's OpenAI-compatible LLM Proxy.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

type Options struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
}

type ChatRequest struct {
	Model          string          `json:"model"`
	Messages       []ChatMessage  `json:"messages"`
	Temperature    float64         `json:"temperature,omitempty"`
	Tools          []Tool          `json:"tools,omitempty"`
	ToolChoice     interface{}     `json:"tool_choice,omitempty"`
	// 1.0 §2.1.3：ResponseFormat 强制 JSON 输出（"json_object"），供 classify/PKB 结构化输出；
	// MaxTokens 限制输出长度，避免长文兜底。proxy 透传给底层 provider。
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
}

// ResponseFormat 控制 LLM 输出格式（OpenAI 兼容）。
type ResponseFormat struct {
	Type string `json:"type"` // "json_object" 或 "text"
}

// RerankRequest 是 /v1/rerank 请求（OpenAI 兼容 rerank API）。
// Documents 为待重排的文档文本列表，Query 为用户问题，TopN 指定返回数量。
type RerankRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n,omitempty"`
}

// RerankResult 单条重排结果。
type RerankResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
}

// RerankResponse 是 /v1/rerank 响应。
type RerankResponse struct {
	Results []RerankResult `json:"results"`
}

type Tool struct {
	Type     string   `json:"type"` // "function"
	Function Function `json:"function"`
}

type Function struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"` // "function"
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatStreamResult is the streaming chat result used by the Gateway interface's
// ChatStream method. Tokens channel delivers each token delta as it arrives.
type ChatStreamResult struct {
	Tokens <-chan string
}

type ChatOptions struct {
	CallerID       string
	TaskType       string
	ConversationID string
}

type ChatResponse struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	FinishReason string  `json:"finish_reason,omitempty"`
	Model     string     `json:"model,omitempty"` // 实际命中的底层模型（proxy 回填）
}

type HTTPError struct {
	StatusCode int
	Body       string
	RetryAfter time.Duration
}

func (e *HTTPError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("llm returned %d after retry-after %s: %s", e.StatusCode, e.RetryAfter, e.Body)
	}
	return fmt.Sprintf("llm returned %d: %s", e.StatusCode, e.Body)
}

func New(opts Options) *Client {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &Client{
		baseURL:    strings.TrimRight(opts.BaseURL, "/"),
		apiKey:     opts.APIKey,
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (c *Client) ChatCompletion(ctx context.Context, req ChatRequest, opts ChatOptions) (string, error) {
	resp, err := c.ChatCompletionFull(ctx, req, opts)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// Chat 是 llmgateway.Gateway 接口的实现，供进程外 CLI（如 cmd/bellkeeper 的
// pkb-curate 子命令）将 *Client 作为 Gateway 注入 LLMJobQueueService 等需要
// Gateway 的组件。签名与 ChatCompletionFull 一致，返回完整 ChatResponse。
//
// 区别于进程内直调的 *llmgateway.LLMProxyService.Chat（绕 HTTP+鉴权），
// 本方法仍经 localhost HTTP + Token 鉴权，适用于独立 CLI 进程。
func (c *Client) Chat(ctx context.Context, req ChatRequest, opts ChatOptions) (*ChatResponse, error) {
	return c.ChatCompletionFull(ctx, req, opts)
}

// ChatStream 是 llmgateway.Gateway 接口的流式实现。与 Chat 不同的是，它在请求
// 体中设置 stream:true 后发送 HTTP 请求，并解析 SSE 事件流中的 content delta，
// 通过 token 通道逐 token 投递。
func (c *Client) ChatStream(ctx context.Context, req ChatRequest, opts ChatOptions) (*ChatStreamResult, error) {
	// Clone the request struct and add stream:true at the serialization level
	bodyMap := marshalStreamBody(req)
	bodyBytes, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("llmclient: marshal stream request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("llmclient: create stream request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
		httpReq.Header.Set("X-API-Key", c.apiKey)
	}
	if opts.CallerID != "" {
		httpReq.Header.Set("X-Caller-ID", opts.CallerID)
	}
	if opts.TaskType != "" {
		httpReq.Header.Set("X-Task-Type", opts.TaskType)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("llmclient: stream request failed: %w", err)
	}
	if resp.StatusCode >= 400 {
		respBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, &HTTPError{StatusCode: resp.StatusCode, Body: string(respBytes)}
	}

	tokenCh := make(chan string, 64)
	go func() {
		defer close(tokenCh)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				return
			}
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
					FinishReason *string `json:"finish_reason"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
			for _, choice := range chunk.Choices {
				if choice.Delta.Content != "" {
					select {
					case tokenCh <- choice.Delta.Content:
					case <-ctx.Done():
						return
					}
				}
				if choice.FinishReason != nil {
					return
				}
			}
		}
	}()

	return &ChatStreamResult{Tokens: tokenCh}, nil
}

// marshalStreamBody 生成带 stream:true 的请求体 map，保留 ChatRequest 所有字段。
func marshalStreamBody(req ChatRequest) map[string]interface{} {
	m := map[string]interface{}{
		"model":    req.Model,
		"messages": req.Messages,
		"stream":   true,
	}
	if req.Temperature != 0 {
		m["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		m["max_tokens"] = req.MaxTokens
	}
	if req.ResponseFormat != nil {
		m["response_format"] = req.ResponseFormat
	}
	if len(req.Tools) > 0 {
		m["tools"] = req.Tools
	}
	if req.ToolChoice != nil {
		m["tool_choice"] = req.ToolChoice
	}
	return m
}

// Rerank 调用 /v1/rerank 重排文档（经 localhost HTTP + Token 鉴权）。
// 进程内调用方应优先用 llmgateway.Gateway.Rerank（绕 HTTP）。
func (c *Client) Rerank(ctx context.Context, req RerankRequest) (*RerankResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/rerank", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
		httpReq.Header.Set("X-API-Key", c.apiKey)
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("rerank request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, &HTTPError{StatusCode: resp.StatusCode, Body: string(raw)}
	}
	var out RerankResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode rerank response: %w", err)
	}
	return &out, nil
}

func (c *Client) ChatCompletionFull(ctx context.Context, req ChatRequest, opts ChatOptions) (*ChatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
		httpReq.Header.Set("X-API-Key", c.apiKey)
	}
	if opts.CallerID != "" {
		httpReq.Header.Set("X-Caller-ID", opts.CallerID)
	}
	if opts.TaskType != "" {
		httpReq.Header.Set("X-Task-Type", opts.TaskType)
	}
	if opts.ConversationID != "" {
		httpReq.Header.Set("X-Conversation-ID", opts.ConversationID)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("llm request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, &HTTPError{
			StatusCode: resp.StatusCode,
			Body:       string(raw),
			RetryAfter: ParseRetryAfter(resp.Header.Get("Retry-After")),
		}
	}
	var out struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content    string     `json:"content"`
				ToolCalls  []ToolCall `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode llm response: %w", err)
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("llm returned no choices")
	}
	choice := out.Choices[0]
	return &ChatResponse{
		Content:      choice.Message.Content,
		ToolCalls:    choice.Message.ToolCalls,
		FinishReason: choice.FinishReason,
		Model:        out.Model,
	}, nil
}

func ParseRetryAfter(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if t, err := http.ParseTime(raw); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}
