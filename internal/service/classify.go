package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/singll/bellkeeper/internal/llmgateway"
	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/llmclient"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/textutil"
)

// ClassifyService handles article classification using LLM
type ClassifyService struct {
	cfg          config.ClassifyConfig
	llm          llmgateway.Gateway
	llmJobs      *llmgateway.LLMJobQueueService
	activityLog  *ActivityLogService
	promptLoader *KBPromptLoader
}

// NewClassifyService creates a new classify service
func NewClassifyService(cfg config.ClassifyConfig, gateway llmgateway.Gateway, activityLog *ActivityLogService) *ClassifyService {
	return &ClassifyService{
		cfg:         cfg,
		llm:         gateway,
		activityLog: activityLog,
	}
}

func (s *ClassifyService) SetLLMJobQueue(queue *llmgateway.LLMJobQueueService) {
	s.llmJobs = queue
}

// SetPromptLoader 注入知识库提示词加载器（1.0 §2.1.3）。未注入时回退到内置默认提示词。
func (s *ClassifyService) SetPromptLoader(loader *KBPromptLoader) {
	s.promptLoader = loader
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

	// Build prompt — 1.0 §2.1.3：system/user 分离。
	// 优先级：s.cfg.Prompt（向后兼容整段覆盖）> promptLoader 外置 > 内置默认。
	systemPrompt := ""
	userTemplate := defaultClassifyPrompt
	if s.cfg.Prompt != "" {
		// 向后兼容：cfg.Prompt 为整段提示词（含占位符），全走 user 角色。
		userTemplate = s.cfg.Prompt
	} else if s.promptLoader != nil {
		systemPrompt = s.promptLoader.GetWithDefault("classify_system", "")
		userTemplate = s.promptLoader.GetWithDefault("classify_user", defaultClassifyPrompt)
	}
	userContent := fmt.Sprintf(userTemplate, req.Title, req.URL, content)

	// Call LLM
	llmResp, err := s.callLLM(systemPrompt, userContent)
	if err != nil {
		// 自修复重试：结构化输出解析失败时，带错误回喂一次（§2.1.3）。
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
		// 自修复重试：解析失败带错误回喂一次，要求严格 JSON。
		retryPrompt := fmt.Sprintf("上一次返回无法解析为 JSON：%s\n错误：%v\n请仅返回合法 JSON 对象，不要 markdown fence。", llmResp, err)
		if resp2, rerr := s.callLLM(systemPrompt, retryPrompt); rerr == nil {
			if result2, perr := s.parseClassifyResponse(resp2); perr == nil {
				result = result2
				err = nil
			}
		}
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

func (s *ClassifyService) callLLM(systemPrompt, userContent string) (string, error) {
	// 1.0 §2.1.3：system/user 分离 + ResponseFormat=json_object 强制结构化输出。
	systemMsgs := []llmclient.ChatMessage{}
	if systemPrompt != "" {
		systemMsgs = append(systemMsgs, llmclient.ChatMessage{Role: "system", Content: systemPrompt})
	}
	systemMsgs = append(systemMsgs, llmclient.ChatMessage{Role: "user", Content: userContent})
	jsonFmt := &llmclient.ResponseFormat{Type: "json_object"}
	if s.llmJobs != nil {
		job, err := s.llmJobs.EnqueueChat(llmgateway.EnqueueLLMChatOptions{
			TaskType:       "classify",
			CallerID:       "classify",
			Model:          s.cfg.Model,
			Messages:       systemMsgs,
			Temperature:    s.cfg.Temperature,
			Priority:       30,
			IdempotencyKey: llmgateway.LLMJobIdempotencyKey("classify", s.cfg.Model, userContent),
			ResponseFormat: jsonFmt,
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
			return "", llmgateway.LLMJobTerminalError(done)
		}
		return done.ResponseText, nil
	}
	resp, err := s.llm.Chat(
		context.Background(),
		llmclient.ChatRequest{
			Model:          s.cfg.Model,
			Messages:       systemMsgs,
			Temperature:    s.cfg.Temperature,
			ResponseFormat: jsonFmt,
		},
		llmclient.ChatOptions{CallerID: "classify", TaskType: "classify"},
	)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
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
