package service

import (
	"encoding/json"
	"strings"
	"sync"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/middleware"
	"go.uber.org/zap"
)

// TaskType represents the classification of an LLM request.
type TaskType string

const (
	TaskCoding      TaskType = "coding"
	TaskClassify    TaskType = "classify"
	TaskSummary     TaskType = "summary"
	TaskQA          TaskType = "qa"
	TaskLongContext TaskType = "long_context"
	TaskChat        TaskType = "chat"
)

// TaskRouter determines which pool of channels to use based on task type.
type TaskRouter struct {
	mu               sync.RWMutex
	codingStrategy   string // free_first | quality_first | complexity_aware
	simpleThreshold  int
	complexThreshold int
	complexKeywords  []string
}

// NewTaskRouter creates a task router with the given coding strategy.
func NewTaskRouter(codingStrategy string, complexity config.ComplexityConfig) *TaskRouter {
	if codingStrategy == "" {
		codingStrategy = "complexity_aware"
	}
	if complexity.SimpleThresholdTokens <= 0 {
		complexity.SimpleThresholdTokens = 1000
	}
	if complexity.ComplexThresholdTokens <= 0 {
		complexity.ComplexThresholdTokens = 4000
	}
	if len(complexity.ComplexKeywords) == 0 {
		complexity.ComplexKeywords = []string{
			"refactor", "architecture", "debug", "implement entire",
			"重构", "架构", "设计", "调试", "实现整个",
		}
	}
	return &TaskRouter{
		codingStrategy:   codingStrategy,
		simpleThreshold:  complexity.SimpleThresholdTokens,
		complexThreshold: complexity.ComplexThresholdTokens,
		complexKeywords:  append([]string(nil), complexity.ComplexKeywords...),
	}
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
func (r *TaskRouter) DetectComplexity(headers map[string]string, body []byte, promptTokens int) ComplexityLevel {
	if explicit := headers["X-Task-Complexity"]; explicit != "" {
		switch ComplexityLevel(strings.ToLower(explicit)) {
		case ComplexitySimple:
			return ComplexitySimple
		case ComplexityMedium:
			return ComplexityMedium
		case ComplexityComplex:
			return ComplexityComplex
		}
	}

	r.mu.RLock()
	simpleThreshold := r.simpleThreshold
	complexThreshold := r.complexThreshold
	keywords := append([]string(nil), r.complexKeywords...)
	r.mu.RUnlock()

	// Token length heuristic
	if promptTokens > 0 && promptTokens < simpleThreshold {
		return ComplexitySimple
	}
	if promptTokens > complexThreshold {
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

	for _, kw := range keywords {
		if strings.Contains(lower, strings.ToLower(kw)) {
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
