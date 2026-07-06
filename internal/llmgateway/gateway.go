// gateway.go 定义 LLM 代理池的进程内直调接口（Gateway），供 Bellkeeper 内部
// KB / Matrix Agent / 日报 / 分类 / RSS 规则优化 / LLM 任务队列等调用方使用，
// 替代原先经 localhost HTTP 自回环的 llmclient.New 构造方式。
//
// 设计要点（见《Bellkeeper 1.0 重构与架构演进规划》§2.2）：
//   - 进程内直调跳过 LLMTokenAuth（进程内可信），但仍保留路由 / 熔断 / 限流 /
//     粘性 / 计费——这些是 LLMProxyService.ProxyRequest 内置能力，无需重复实现。
//   - 接口签名复用 llmclient.ChatRequest / ChatResponse / ChatOptions，与进程外
//     客户端契约完全一致，调用方仅替换实例来源（Gateway 接口注入 vs HTTP 构造）。
//   - llmclient 包保留：外部脚本 / n8n 仍经 HTTP + Token 鉴权调用代理池。
//   - tokenID 传 0：进程内直调无外部 Token，计费按 callerID 归集（与既有
//     "unknown" 调用方一致），鉴权层由 ProxyRequest 上游的 LLMTokenAuth 仅对
//     HTTP 入口生效，进程内直调天然绕过。

package llmgateway

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/llmclient"
	"go.uber.org/zap"
)

// Gateway 是 LLM 代理池的进程内直调接口。
//
// 实现方为 *LLMProxyService（见本包 Chat / Rerank 方法）。
// 注入给调用方后，调用方调用 gateway.Chat(...) / gateway.Rerank(...) 而非经 HTTP 自回环。
type Gateway interface {
	Chat(ctx context.Context, req llmclient.ChatRequest, opts llmclient.ChatOptions) (*llmclient.ChatResponse, error)
	ChatStream(ctx context.Context, req llmclient.ChatRequest, opts llmclient.ChatOptions) (*llmclient.ChatStreamResult, error)
	Rerank(ctx context.Context, req llmclient.RerankRequest) (*llmclient.RerankResponse, error)
}

// Compile-time assertion: *LLMProxyService implements Gateway.
var _ Gateway = (*LLMProxyService)(nil)

// ChatStream 执行流式 chat.completions 调用，返回 token 通道供调用方消费。
// 内部调用 ProxyStreamRequest，从 SSE 事件流中解析 content delta 投递到通道。
func (s *LLMProxyService) ChatStream(ctx context.Context, req llmclient.ChatRequest, opts llmclient.ChatOptions) (*llmclient.ChatStreamResult, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("llmgateway: marshal stream request: %w", err)
	}

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	if opts.CallerID != "" {
		headers.Set("X-Caller-ID", opts.CallerID)
	}
	if opts.TaskType != "" {
		headers.Set("X-Task-Type", opts.TaskType)
	}

	const callerInternal = "internal"
	streamResult, err := s.ProxyStreamRequest(http.MethodPost, "/v1/chat/completions", headers, body, callerInternal, 0)
	if err != nil {
		return nil, fmt.Errorf("llmgateway: proxy stream request: %w", err)
	}
	if streamResult.StatusCode >= 400 {
		respBytes, _ := io.ReadAll(streamResult.BodyReader)
		streamResult.BodyReader.Close()
		return nil, &llmclient.HTTPError{
			StatusCode: streamResult.StatusCode,
			Body:       string(respBytes),
		}
	}

	tokenCh := make(chan string, 64)
	go func() {
		defer close(tokenCh)
		defer streamResult.BodyReader.Close()
		scanner := bufio.NewScanner(streamResult.BodyReader)
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
		if err := scanner.Err(); err != nil {
			middleware.GetLogger().Warn("chat stream: scanner error",
				zap.Error(err))
		}
	}()

	return &llmclient.ChatStreamResult{Tokens: tokenCh}, nil
}

// Chat 执行一次非流式 OpenAI 兼容 chat.completions 调用，进程内直调 LLM 代理池。
//
// 路由路径固定为 "/v1/chat/completions"（OpenAI 兼容），callerID 经 opts 透传到
// ProxyRequest 的 X-Caller-ID header（用于计费归集与 sticky task-key 派生），
// TaskType 透传 X-Task-Type（任务感知路由），ConversationID 透传 X-Conversation-ID
// （粘性会话）。tokenID=0 表示进程内直调无外部 Token。
func (s *LLMProxyService) Chat(ctx context.Context, req llmclient.ChatRequest, opts llmclient.ChatOptions) (*llmclient.ChatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("llmgateway: marshal chat request: %w", err)
	}

	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	if opts.CallerID != "" {
		headers.Set("X-Caller-ID", opts.CallerID)
	}
	if opts.TaskType != "" {
		headers.Set("X-Task-Type", opts.TaskType)
	}
	if opts.ConversationID != "" {
		headers.Set("X-Conversation-ID", opts.ConversationID)
	}

	const callerInternal = "internal"
	statusCode, respBody, _, err := s.ProxyRequest(http.MethodPost, "/v1/chat/completions", headers, body, callerInternal, 0)
	if err != nil {
		return nil, fmt.Errorf("llmgateway: proxy request: %w", err)
	}
	if statusCode >= 400 {
		return nil, &llmclient.HTTPError{
			StatusCode: statusCode,
			Body:       string(respBody),
		}
	}

	var out struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content   string             `json:"content"`
				ToolCalls []llmclient.ToolCall `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("llmgateway: decode chat response: %w", err)
	}
	if len(out.Choices) == 0 {
		return nil, fmt.Errorf("llmgateway: llm returned no choices")
	}
	choice := out.Choices[0]
	return &llmclient.ChatResponse{
		Content:      choice.Message.Content,
		ToolCalls:    choice.Message.ToolCalls,
		FinishReason: choice.FinishReason,
		Model:        out.Model,
	}, nil
}
// Rerank 执行进程内 rerank 调用（路由 /v1/rerank，绕 HTTP+鉴权，保留熔断/限流/路由）。
// rerank channel 由 provider_type=="rerank" 的通道承担，proxyRerank 内部处理路由。
func (s *LLMProxyService) Rerank(ctx context.Context, req llmclient.RerankRequest) (*llmclient.RerankResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("llmgateway: marshal rerank request: %w", err)
	}
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	statusCode, respBody, _, err := s.ProxyRequest(http.MethodPost, "/v1/rerank", headers, body, "internal", 0)
	if err != nil {
		return nil, fmt.Errorf("llmgateway: proxy rerank: %w", err)
	}
	if statusCode >= 400 {
		return nil, &llmclient.HTTPError{StatusCode: statusCode, Body: string(respBody)}
	}
	var out llmclient.RerankResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("llmgateway: decode rerank response: %w", err)
	}
	return &out, nil
}
