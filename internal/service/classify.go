package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/llmclient"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/textutil"
)

// ClassifyService handles article classification using LLM
type ClassifyService struct {
	cfg         config.ClassifyConfig
	llmClient   *llmclient.Client
	llmJobs     *LLMJobQueueService
	activityLog *ActivityLogService
}

// NewClassifyService creates a new classify service
func NewClassifyService(cfg config.ClassifyConfig, apiKey string, activityLog *ActivityLogService) *ClassifyService {
	return &ClassifyService{
		cfg: cfg,
		llmClient: llmclient.New(llmclient.Options{
			BaseURL: cfg.LLMProxyURL,
			APIKey:  apiKey,
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		}),
		activityLog: activityLog,
	}
}

func (s *ClassifyService) SetLLMJobQueue(queue *LLMJobQueueService) {
	s.llmJobs = queue
}

// ClassifyRequest represents the classification request
type ClassifyRequest struct {
	Title   string `json:"title" binding:"required"`
	URL     string `json:"url" binding:"required"`
	Content string `json:"content" binding:"required"`
}

// ClassifyResponse represents the classification result
type ClassifyResponse struct {
	PrimaryDomain  string             `json:"primary_domain"`
	Tags           []string           `json:"tags"`
	TagConfidences map[string]float64 `json:"tag_confidences,omitempty"`
	DatasetID      string             `json:"dataset_id"`
	Confidence     float64            `json:"confidence"`
	Reasoning      string             `json:"reasoning"`
}

const defaultClassifyPrompt = `你是一个内容分类专家。请分析以下文章，返回分类结果。

文章标题：%s
文章URL：%s
文章摘要/内容片段：%s

请返回JSON格式：
{
  "primary_domain": "security|ai|programming|general",
  "tags": ["domain-subdomain", ...],
  "tag_confidences": {"domain-subdomain": 0.0-1.0},
  "dataset": "security-tech|ai-tech|dev-tech|daily-digest",
  "confidence": 0.0-1.0,
  "reasoning": "简短说明分类依据"
}

标签规则：
- security细分: web, network, vulnerability, tool, pentest
- ai细分: nlp, cv, ml, paper, tool, llm, agent, rag
- programming细分: python, go, rust, javascript, dotnet, web, system, data
- 标签数量建议 3-8 个，最多 10 个
- 标签覆盖三类：领域标签、技术实体标签、内容形态标签
- 标签要稳定、短、可复用，不返回一次性长短语
- 标签格式优先使用 {domain}-{subdomain}，技术实体可使用短横线格式，例如 llm, retrieval-augmented-generation, go-runtime

示例：
- 关于SQL注入的文章 → {"primary_domain": "security", "tags": ["security-web", "security-vulnerability", "pentest", "how-to"], "dataset": "security-tech"}
- 关于GPT-4的论文 → {"primary_domain": "ai", "tags": ["ai-llm", "ai-paper", "transformer", "benchmark"], "dataset": "ai-tech"}
- 关于Python异步编程 → {"primary_domain": "programming", "tags": ["programming-python", "async-io", "web-backend", "tutorial"], "dataset": "dev-tech"}`

// ClassifyArticle classifies an article using LLM
func (s *ClassifyService) ClassifyArticle(req *ClassifyRequest) (*ClassifyResponse, error) {
	start := time.Now()
	// Truncate content to configured max length
	content := req.Content
	if len([]rune(content)) > s.cfg.MaxContentLen {
		content = string([]rune(content)[:s.cfg.MaxContentLen])
	}

	// Build prompt — use configured prompt if set, otherwise built-in default
	promptTemplate := defaultClassifyPrompt
	if s.cfg.Prompt != "" {
		promptTemplate = s.cfg.Prompt
	}
	prompt := fmt.Sprintf(promptTemplate, req.Title, req.URL, content)

	// Call LLM
	llmResp, err := s.callLLM(prompt)
	if err != nil {
		if s.activityLog != nil {
			s.activityLog.LogActivity(LogActivityParams{
				Module: "classify", Action: "classify_article", Status: "error",
				Summary:    fmt.Sprintf("分类失败: %s - %v", req.Title, err),
				DurationMs: int(time.Since(start).Milliseconds()),
			})
		}
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// Parse response
	result, err := s.parseClassifyResponse(llmResp)
	if err != nil {
		if s.activityLog != nil {
			s.activityLog.LogActivity(LogActivityParams{
				Module: "classify", Action: "classify_article", Status: "error",
				Summary:    fmt.Sprintf("分类解析失败: %s - %v", req.Title, err),
				DurationMs: int(time.Since(start).Milliseconds()),
			})
		}
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	if s.activityLog != nil {
		s.activityLog.LogActivity(LogActivityParams{
			Module: "classify", Action: "classify_article", Status: "success",
			Summary:    fmt.Sprintf("分类完成: %s → %s (%.0f%%)", req.Title, result.DatasetID, result.Confidence*100),
			DurationMs: int(time.Since(start).Milliseconds()),
			Detail:     map[string]any{"domain": result.PrimaryDomain, "tags": result.Tags, "dataset": result.DatasetID},
		})
	}

	return result, nil
}

func (s *ClassifyService) callLLM(prompt string) (string, error) {
	if s.llmJobs != nil {
		job, err := s.llmJobs.EnqueueChat(EnqueueLLMChatOptions{
			TaskType:       "classify",
			CallerID:       "classify",
			Model:          s.cfg.Model,
			Messages:       []llmclient.ChatMessage{{Role: "user", Content: prompt}},
			Temperature:    s.cfg.Temperature,
			Priority:       30,
			IdempotencyKey: llmJobIdempotencyKey("classify", s.cfg.Model, prompt),
		})
		if err != nil {
			return "", err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
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
		context.Background(),
		llmclient.ChatRequest{
			Model: s.cfg.Model,
			Messages: []llmclient.ChatMessage{
				{Role: "user", Content: prompt},
			},
			Temperature: s.cfg.Temperature,
		},
		llmclient.ChatOptions{CallerID: "classify", TaskType: "classify"},
	)
}

func (s *ClassifyService) parseClassifyResponse(content string) (*ClassifyResponse, error) {
	content = textutil.StripJSONFence(content)

	var result struct {
		PrimaryDomain  string             `json:"primary_domain"`
		Tags           []string           `json:"tags"`
		TagConfidences map[string]float64 `json:"tag_confidences"`
		Dataset        string             `json:"dataset"`
		Confidence     float64            `json:"confidence"`
		Reasoning      string             `json:"reasoning"`
	}

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, err
	}

	return &ClassifyResponse{
		PrimaryDomain:  result.PrimaryDomain,
		Tags:           normalizeAutoTagList(result.Tags),
		TagConfidences: result.TagConfidences,
		DatasetID:      result.Dataset,
		Confidence:     result.Confidence,
		Reasoning:      result.Reasoning,
	}, nil
}
