package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ClassifyService handles article classification using LLM
type ClassifyService struct {
	llmProxyURL string
	httpClient  *http.Client
}

// NewClassifyService creates a new classify service
func NewClassifyService(llmProxyURL string) *ClassifyService {
	return &ClassifyService{
		llmProxyURL: llmProxyURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// ClassifyRequest represents the classification request
type ClassifyRequest struct {
	Title   string `json:"title" binding:"required"`
	URL     string `json:"url" binding:"required"`
	Content string `json:"content" binding:"required"`
}

// ClassifyResponse represents the classification result
type ClassifyResponse struct {
	PrimaryDomain string   `json:"primary_domain"`
	Tags          []string `json:"tags"`
	DatasetID     string   `json:"dataset_id"`
	Confidence    float64  `json:"confidence"`
	Reasoning     string   `json:"reasoning"`
}

const classifyPrompt = `你是一个内容分类专家。请分析以下文章，返回分类结果。

文章标题：%s
文章URL：%s
文章摘要/内容片段：%s

请返回JSON格式：
{
  "primary_domain": "security|ai|programming|general",
  "tags": ["domain-subdomain", ...],
  "dataset": "security-tech|ai-tech|programming|daily-digest",
  "confidence": 0.0-1.0,
  "reasoning": "简短说明分类依据"
}

标签规则：
- security细分: web, network, vulnerability, tool, pentest
- ai细分: nlp, cv, ml, paper, tool
- programming细分: python, go, rust, javascript, dotnet, web, system, data
- 可返回多个标签（1-3个为宜）
- 标签格式: {domain}-{subdomain}

示例：
- 关于SQL注入的文章 → {"primary_domain": "security", "tags": ["security-web", "security-vulnerability"], "dataset": "security-tech"}
- 关于GPT-4的论文 → {"primary_domain": "ai", "tags": ["ai-nlp", "ai-paper"], "dataset": "ai-tech"}
- 关于Python异步编程 → {"primary_domain": "programming", "tags": ["programming-python", "programming-web"], "dataset": "programming"}`

// ClassifyArticle classifies an article using LLM
func (s *ClassifyService) ClassifyArticle(req *ClassifyRequest) (*ClassifyResponse, error) {
	// Truncate content to 1000 chars
	content := req.Content
	if len(content) > 1000 {
		content = content[:1000]
	}

	// Build prompt
	prompt := fmt.Sprintf(classifyPrompt, req.Title, req.URL, content)

	// Call LLM
	llmResp, err := s.callLLM(prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// Parse response
	result, err := s.parseClassifyResponse(llmResp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	return result, nil
}

func (s *ClassifyService) callLLM(prompt string) (string, error) {
	reqBody := map[string]interface{}{
		"model": "glm-4-flash",
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"temperature": 0.3,
	}

	jsonData, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", s.llmProxyURL+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var llmResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	if err := json.Unmarshal(body, &llmResp); err != nil {
		return "", err
	}

	if len(llmResp.Choices) == 0 {
		return "", fmt.Errorf("no response from LLM")
	}

	return llmResp.Choices[0].Message.Content, nil
}

func (s *ClassifyService) parseClassifyResponse(content string) (*ClassifyResponse, error) {
	// Extract JSON from markdown code block if present
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	}

	var result struct {
		PrimaryDomain string   `json:"primary_domain"`
		Tags          []string `json:"tags"`
		Dataset       string   `json:"dataset"`
		Confidence    float64  `json:"confidence"`
		Reasoning     string   `json:"reasoning"`
	}

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, err
	}

	return &ClassifyResponse{
		PrimaryDomain: result.PrimaryDomain,
		Tags:          result.Tags,
		DatasetID:     result.Dataset,
		Confidence:    result.Confidence,
		Reasoning:     result.Reasoning,
	}, nil
}
