package pkb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/singll/bellkeeper/internal/llmclient"
)

// ArticleMeta 是 GET /api/files/list 返回的 ArticleTag 子集（pkb-curate 只需这些字段）。
// 字段 tag 对齐 model.ArticleTag 的 json tag。
type ArticleMeta struct {
	ID         uint   `json:"id"`
	DocumentID string `json:"document_id"`
	Title      string `json:"article_title"`
	URL        string `json:"article_url"`
	FilePath   string `json:"file_path"`
	Layer      string `json:"layer"`
}

// Client 封装 pkb-curate 对本机 Bellkeeper server 的 HTTP 调用（list/search/rebuild/LLM）。
// 所有调用走 localhost，复用现有端点与 LLM Proxy 的熔断/限速/计费。
type Client struct {
	apiBase    string // http://localhost:8080
	llmBase    string // http://localhost:8080/api/llm/v1
	apiKey     string // Bellkeeper API key for local /api/files/* calls
	llmKey     string // Dedicated LLM token when configured, else apiKey
	httpClient *http.Client
	llmClient  *llmclient.Client
}

// NewClient 构造客户端。llmBase 形如 http://localhost:8080/api/llm/v1（取自 classify.llm_proxy_url）。
func NewClient(llmBase, apiKey, llmKey string, timeout time.Duration) *Client {
	apiBase := strings.TrimSuffix(llmBase, "/api/llm/v1")
	if apiBase == llmBase { // 容错：未按预期结尾
		apiBase = "http://localhost:8080"
	}
	if llmKey == "" {
		llmKey = apiKey
	}
	return &Client{
		apiBase:    apiBase,
		llmBase:    llmBase,
		apiKey:     apiKey,
		llmKey:     llmKey,
		httpClient: &http.Client{Timeout: timeout},
		llmClient: llmclient.New(llmclient.Options{
			BaseURL: llmBase,
			APIKey:  llmKey,
			Timeout: timeout,
		}),
	}
}

func (c *Client) newReq(method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	key := c.apiKey
	if strings.HasPrefix(url, c.llmBase) && c.llmKey != "" {
		key = c.llmKey
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("X-API-Key", key)
	}
	return req, nil
}

// ListRaw 列出 raw 层待处理文章（GET /api/files/list?layer=raw&per_page=N）。
// excludeProcessed=true 时附加 exclude_processed=true，跳过已被 pkb-curate 处理过的条目（幂等）。
func (c *Client) ListRaw(perPage int, excludeProcessed bool) ([]ArticleMeta, error) {
	url := fmt.Sprintf("%s/api/files/list?layer=raw&page=1&per_page=%d", c.apiBase, perPage)
	if excludeProcessed {
		url += "&exclude_processed=true"
	}
	req, err := c.newReq(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list raw: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("list raw returned %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		Data []ArticleMeta `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode list response: %w", err)
	}
	return out.Data, nil
}

// SearchTitles 检索指定层的卡片概念名（供 vault 重构的 wikilink 候选）。
// 优先返回 atomic_concept（稳定身份锚），无则回退 title。
func (c *Client) SearchTitles(query string, layers []string, limit int) ([]string, error) {
	reqBody := map[string]interface{}{
		"query":  query,
		"layers": layers,
		"limit":  limit,
	}
	jsonData, _ := json.Marshal(reqBody)
	req, err := c.newReq(http.MethodPost, c.apiBase+"/api/files/search", bytes.NewReader(jsonData))
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("search returned %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		Data struct {
			Files []struct {
				Title         string `json:"title"`
				AtomicConcept string `json:"atomic_concept"`
			} `json:"files"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}
	titles := make([]string, 0, len(out.Data.Files))
	for _, f := range out.Data.Files {
		name := f.AtomicConcept
		if name == "" {
			name = f.Title
		}
		if name != "" {
			titles = append(titles, name)
		}
	}
	return titles, nil
}

// SearchContent 检索指定层的内容级匹配（供语义去重）。
// 返回每张匹配卡的 atomic_concept（无则回退标题）+ 摘要（前 200 字）。
func (c *Client) SearchContent(query string, layers []string, limit int) ([]ContentMatch, error) {
	reqBody := map[string]interface{}{
		"query":  query,
		"layers": layers,
		"limit":  limit,
	}
	jsonData, _ := json.Marshal(reqBody)
	req, err := c.newReq(http.MethodPost, c.apiBase+"/api/files/search", bytes.NewReader(jsonData))
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search content: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("search content returned %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		Data struct {
			Files []struct {
				Title         string `json:"title"`
				AtomicConcept string `json:"atomic_concept"`
				Excerpt       string `json:"excerpt"`
			} `json:"files"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode search content response: %w", err)
	}
	matches := make([]ContentMatch, 0, len(out.Data.Files))
	for _, f := range out.Data.Files {
		concept := f.AtomicConcept
		if concept == "" {
			concept = f.Title
		}
		if concept != "" {
			matches = append(matches, ContentMatch{
				Concept: concept,
				Excerpt: truncateRunes(f.Excerpt, 200),
			})
		}
	}
	return matches, nil
}

// ContentMatch 内容级搜索结果
type ContentMatch struct {
	Concept string
	Excerpt string
}

// Rebuild 触发全量重建索引（POST /api/files/rebuild）。
func (c *Client) Rebuild() error {
	req, err := c.newReq(http.MethodPost, c.apiBase+"/api/files/rebuild", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("rebuild: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("rebuild returned %d: %s", resp.StatusCode, string(raw))
	}
	return nil
}

// ChatCompletion 调 LLM Proxy（POST {llmBase}/chat/completions），返回 assistant 文本内容。
func (c *Client) ChatCompletion(model, systemPrompt, userPrompt string, temperature float64, taskType string) (string, error) {
	messages := make([]map[string]string, 0, 2)
	if systemPrompt != "" {
		messages = append(messages, map[string]string{"role": "system", "content": systemPrompt})
	}
	messages = append(messages, map[string]string{"role": "user", "content": userPrompt})

	reqBody := llmclient.ChatRequest{
		Model:       model,
		Temperature: temperature,
	}
	for _, msg := range messages {
		reqBody.Messages = append(reqBody.Messages, llmclient.ChatMessage{
			Role:    msg["role"],
			Content: msg["content"],
		})
	}
	return c.llmClient.ChatCompletion(
		context.Background(),
		reqBody,
		llmclient.ChatOptions{CallerID: "pkb-curate", TaskType: taskType},
	)
}
