package service

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/singll/bellkeeper/internal/middleware"
	"go.uber.org/zap"
)

// TaskType represents the classification of an LLM request.
type TaskType string

const (
	TaskCoding       TaskType = "coding"
	TaskClassify     TaskType = "classify"
	TaskSummary      TaskType = "summary"
	TaskQA           TaskType = "qa"
	TaskLongContext  TaskType = "long_context"
	TaskChat         TaskType = "chat"
)

// TaskRouter determines which pool of channels to use based on task type.
type TaskRouter struct {
	mu      sync.RWMutex
	codingStrategy string // free_first | quality_first | complexity_aware
}

// NewTaskRouter creates a task router with the given coding strategy.
func NewTaskRouter(codingStrategy string) *TaskRouter {
	if codingStrategy == "" {
		codingStrategy = "complexity_aware"
	}
	return &TaskRouter{codingStrategy: codingStrategy}
}

// DetectTaskType determines the task type from request metadata.
func (r *TaskRouter) DetectTaskType(headers map[string]string, modelName string, body []byte, promptTokens int) TaskType {
	// 1. Explicit header
	if tt := headers["X-Task-Type"]; tt != "" {
		return TaskType(tt)
	}

	// 2. Caller ID inference (internal services)
	callerID := headers["X-Caller-ID"]
	if strings.Contains(callerID, "classify") || strings.Contains(callerID, "categorize") {
		return TaskClassify
	}
	if strings.Contains(callerID, "summary") || strings.Contains(callerID, "digest") {
		return TaskSummary
	}
	if strings.Contains(callerID, "qa") || strings.Contains(callerID, "ask") {
		return TaskQA
	}

	// 3. Model name matching
	lowerModel := strings.ToLower(modelName)
	if strings.Contains(lowerModel, "coding") || strings.Contains(lowerModel, "code") ||
		strings.Contains(lowerModel, "kimi-for-coding") {
		return TaskCoding
	}

	// 4. Context length
	if promptTokens > 32000 {
		return TaskLongContext
	}

	// 5. Default
	return TaskChat
}

// ComplexityLevel represents prompt complexity for coding tasks.
type ComplexityLevel string

const (
	ComplexitySimple  ComplexityLevel = "simple"
	ComplexityMedium  ComplexityLevel = "medium"
	ComplexityComplex ComplexityLevel = "complex"
)

// DetectComplexity determines coding task complexity.
func (r *TaskRouter) DetectComplexity(body []byte, promptTokens int) ComplexityLevel {
	// Check explicit header
	// (would be extracted from headers in real implementation)

	// Token length heuristic
	if promptTokens < 1000 {
		return ComplexitySimple
	}
	if promptTokens > 4000 {
		return ComplexityComplex
	}

	// Keyword matching
	var req struct {
		Messages []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(body, &req)
	content := ""
	for _, msg := range req.Messages {
		content += msg.Content + " "
	}
	lower := strings.ToLower(content)

	complexKeywords := []string{
		"refactor", "architecture", "debug", "implement entire",
		"重构", "架构", "设计", "调试", "实现整个",
	}
	for _, kw := range complexKeywords {
		if strings.Contains(lower, kw) {
			return ComplexityComplex
		}
	}

	return ComplexityMedium
}

// GetCodingStrategy returns the configured coding routing strategy.
func (r *TaskRouter) GetCodingStrategy() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.codingStrategy
}

// SetCodingStrategy updates the coding routing strategy.
func (r *TaskRouter) SetCodingStrategy(strategy string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.codingStrategy = strategy
	middleware.GetLogger().Info("coding routing strategy updated", zap.String("strategy", strategy))
}

// IsTaskRoutable returns true if the task type should bypass normal model-group routing.
func IsTaskRoutable(tt TaskType) bool {
	switch tt {
	case TaskCoding, TaskClassify, TaskSummary, TaskQA, TaskLongContext:
		return true
	default:
		return false
	}
}
