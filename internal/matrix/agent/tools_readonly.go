package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/singll/bellkeeper/internal/llmclient"
)

func jsonSchema(properties map[string]interface{}, required []string) map[string]interface{} {
	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

type HealthChecker interface {
	Detailed() interface{}
}

type DashboardStatser interface {
	Stats() (interface{}, error)
}

type KnowledgeSearcher interface {
	SearchKnowledge(ctx context.Context, query string, layers []string, limit int) (interface{}, error)
}

type KnowledgeAsker interface {
	AskKnowledge(ctx context.Context, question string, layers []string) (interface{}, error)
}

type LLMUsageQuerier interface {
	LLMUsageSince(since time.Time) (interface{}, error)
}

type CrawlStatser interface {
	CrawlStats() (interface{}, error)
}

type ToolDependencies struct {
	HealthChecker  HealthChecker
	DashboardSvc   DashboardStatser
	SearchSvc      KnowledgeSearcher
	AskSvc         KnowledgeAsker
	LLMUsageSvc    LLMUsageQuerier
	CrawlSvc       CrawlStatser
}

func RegisterReadonlyTools(registry *ToolRegistry, deps ToolDependencies) {
	registerSystemHealth(registry, deps)
	registerDashboardStats(registry, deps)
	registerKnowledgeSearch(registry, deps)
	registerKnowledgeAsk(registry, deps)
	registerLLMUsage(registry, deps)
	registerCrawlStatus(registry, deps)
}

func registerSystemHealth(registry *ToolRegistry, deps ToolDependencies) {
	registry.Register(&ToolDefinition{
		Level: LevelReadonly,
		Tool: llmclient.Tool{
			Type: "function",
			Function: llmclient.Function{
				Name:        "system_health",
				Description: "查看系统各服务的健康状态和运行指标",
				Parameters:  jsonSchema(map[string]interface{}{}, nil),
			},
		},
		Handler: func(ctx context.Context, _ json.RawMessage) (string, error) {
			health := deps.HealthChecker.Detailed()
			data, err := json.Marshal(health)
			if err != nil {
				return "", fmt.Errorf("marshal health: %w", err)
			}
			return string(data), nil
		},
	})
}

func registerDashboardStats(registry *ToolRegistry, deps ToolDependencies) {
	registry.Register(&ToolDefinition{
		Level: LevelReadonly,
		Tool: llmclient.Tool{
			Type: "function",
			Function: llmclient.Function{
				Name:        "dashboard_stats",
				Description: "查看仪表盘统计数据：爬取队列、PKB知识库、LLM使用的今日统计",
				Parameters:  jsonSchema(map[string]interface{}{}, nil),
			},
		},
		Handler: func(ctx context.Context, _ json.RawMessage) (string, error) {
			stats, err := deps.DashboardSvc.Stats()
			if err != nil {
				return "", fmt.Errorf("get stats: %w", err)
			}
			data, err := json.Marshal(stats)
			if err != nil {
				return "", fmt.Errorf("marshal stats: %w", err)
			}
			return string(data), nil
		},
	})
}

func registerKnowledgeSearch(registry *ToolRegistry, deps ToolDependencies) {
	registry.Register(&ToolDefinition{
		Level: LevelReadonly,
		Tool: llmclient.Tool{
			Type: "function",
			Function: llmclient.Function{
				Name:        "knowledge_search",
				Description: "在知识库中搜索相关文档，返回匹配的文件和摘要",
				Parameters: jsonSchema(map[string]interface{}{
					"query":  map[string]interface{}{"type": "string", "description": "搜索关键词"},
					"layers": map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "限定知识层级，如 archive, vault"},
					"limit":  map[string]interface{}{"type": "integer", "description": "返回结果数量，默认5"},
				}, []string{"query"}),
			},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				Query  string   `json:"query"`
				Layers []string `json:"layers"`
				Limit  int      `json:"limit"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("parse args: %w", err)
			}
			if params.Limit <= 0 {
				params.Limit = 5
			}
			result, err := deps.SearchSvc.SearchKnowledge(ctx, params.Query, params.Layers, params.Limit)
			if err != nil {
				return "", fmt.Errorf("search: %w", err)
			}
			data, err := json.Marshal(result)
			if err != nil {
				return "", fmt.Errorf("marshal result: %w", err)
			}
			return string(data), nil
		},
	})
}

func registerKnowledgeAsk(registry *ToolRegistry, deps ToolDependencies) {
	registry.Register(&ToolDefinition{
		Level: LevelReadonly,
		Tool: llmclient.Tool{
			Type: "function",
			Function: llmclient.Function{
				Name:        "knowledge_ask",
				Description: "基于知识库内容回答问题，会先搜索相关文档再用LLM总结回答",
				Parameters: jsonSchema(map[string]interface{}{
					"question": map[string]interface{}{"type": "string", "description": "用户的问题"},
					"layers":   map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}, "description": "限定知识层级"},
				}, []string{"question"}),
			},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				Question string   `json:"question"`
				Layers   []string `json:"layers"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("parse args: %w", err)
			}
			resp, err := deps.AskSvc.AskKnowledge(ctx, params.Question, params.Layers)
			if err != nil {
				return "", fmt.Errorf("ask: %w", err)
			}
			data, err := json.Marshal(resp)
			if err != nil {
				return "", fmt.Errorf("marshal result: %w", err)
			}
			return string(data), nil
		},
	})
}

func registerLLMUsage(registry *ToolRegistry, deps ToolDependencies) {
	registry.Register(&ToolDefinition{
		Level: LevelReadonly,
		Tool: llmclient.Tool{
			Type: "function",
			Function: llmclient.Function{
				Name:        "llm_usage",
				Description: "查看LLM代理的使用统计：请求数、token用量、费用、错误率等",
				Parameters: jsonSchema(map[string]interface{}{
					"hours": map[string]interface{}{"type": "integer", "description": "查看最近N小时的统计，默认24"},
				}, nil),
			},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			var params struct {
				Hours int `json:"hours"`
			}
			if err := json.Unmarshal(args, &params); err != nil {
				return "", fmt.Errorf("parse args: %w", err)
			}
			if params.Hours <= 0 {
				params.Hours = 24
			}
			summary, err := deps.LLMUsageSvc.LLMUsageSince(time.Now().Add(-time.Duration(params.Hours) * time.Hour))
			if err != nil {
				return "", fmt.Errorf("get usage: %w", err)
			}
			data, err := json.Marshal(summary)
			if err != nil {
				return "", fmt.Errorf("marshal summary: %w", err)
			}
			return string(data), nil
		},
	})
}

func registerCrawlStatus(registry *ToolRegistry, deps ToolDependencies) {
	registry.Register(&ToolDefinition{
		Level: LevelReadonly,
		Tool: llmclient.Tool{
			Type: "function",
			Function: llmclient.Function{
				Name:        "crawl_status",
				Description: "查看爬取队列状态：待处理URL数、成功率、今日活动、Feed订阅情况",
				Parameters:  jsonSchema(map[string]interface{}{}, nil),
			},
		},
		Handler: func(ctx context.Context, _ json.RawMessage) (string, error) {
			if deps.CrawlSvc == nil {
				return `{"status":"disabled","message":"爬取队列未启用"}`, nil
			}
			stats, err := deps.CrawlSvc.CrawlStats()
			if err != nil {
				return "", fmt.Errorf("get crawl stats: %w", err)
			}
			data, err := json.Marshal(stats)
			if err != nil {
				return "", fmt.Errorf("marshal stats: %w", err)
			}
			return string(data), nil
		},
	})
}
