package llmclient

import (
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
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatOptions struct {
	CallerID string
	TaskType string
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
	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
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
		return "", fmt.Errorf("llm request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", &HTTPError{
			StatusCode: resp.StatusCode,
			Body:       string(raw),
			RetryAfter: ParseRetryAfter(resp.Header.Get("Retry-After")),
		}
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("decode llm response: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("llm returned no choices")
	}
	return out.Choices[0].Message.Content, nil
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
