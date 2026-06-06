package pkb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ArticleMeta 是 GET /api/files/list 返回的 ArticleTag 子集（pkb-curate 只需这些字段）。
// 字段 tag 对齐 model.ArticleTag 的 json tag。
type ArticleMeta struct {
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
	apiKey     string // X-API-Key（noauth 模式下亦透传，与 classify/ask 一致）
	httpClient *http.Client
}

// NewClient 构造客户端。llmBase 形如 http://localhost:8080/api/llm/v1（取自 classify.llm_proxy_url）。
func NewClient(llmBase, apiKey string, timeout time.Duration) *Client {
	apiBase := strings.TrimSuffix(llmBase, "/api/llm/v1")
	if apiBase == llmBase { // 容错：未按预期结尾
		apiBase = "http://localhost:8080"
	}
	return &Client{
		apiBase:    apiBase,
		llmBase:    llmBase,
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (c *Client) newReq(method, url string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}
	return req, nil
}

// ListRaw 列出 raw 层待处理文章（GET /api/files/list?layer=raw&per_page=N）。
func (c *Client) ListRaw(perPage int) ([]ArticleMeta, error) {
	url := fmt.Sprintf("%s/api/files/list?layer=raw&page=1&per_page=%d", c.apiBase, perPage)
	req, err := c.newReq(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list raw: %w", err)
	}
	defer resp.Body.Close()
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

// SearchTitles 检索指定层的卡片标题（供 vault 重构的 wikilink 候选）。
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
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("search returned %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		Data struct {
			Files []struct {
				Title string `json:"title"`
			} `json:"files"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}
	titles := make([]string, 0, len(out.Data.Files))
	for _, f := range out.Data.Files {
		if f.Title != "" {
			titles = append(titles, f.Title)
		}
	}
	return titles, nil
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
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("rebuild returned %d: %s", resp.StatusCode, string(raw))
	}
	return nil
}

// ChatCompletion 调 LLM Proxy（POST {llmBase}/chat/completions），返回 assistant 文本内容。
func (c *Client) ChatCompletion(model, systemPrompt, userPrompt string, temperature float64) (string, error) {
	messages := make([]map[string]string, 0, 2)
	if systemPrompt != "" {
		messages = append(messages, map[string]string{"role": "system", "content": systemPrompt})
	}
	messages = append(messages, map[string]string{"role": "user", "content": userPrompt})

	reqBody := map[string]interface{}{
		"model":       model,
		"messages":    messages,
		"temperature": temperature,
	}
	jsonData, _ := json.Marshal(reqBody)
	req, err := c.newReq(http.MethodPost, c.llmBase+"/chat/completions", bytes.NewReader(jsonData))
	if err != nil {
		return "", err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm request: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("llm returned %d: %s", resp.StatusCode, string(raw))
	}
	var llmResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &llmResp); err != nil {
		return "", fmt.Errorf("decode llm response: %w", err)
	}
	if len(llmResp.Choices) == 0 {
		return "", fmt.Errorf("llm returned no choices")
	}
	return llmResp.Choices[0].Message.Content, nil
}
