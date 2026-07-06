package llmgateway

import (
	"testing"

	"github.com/singll/bellkeeper/internal/config"
)

func TestTaskRouterDetectComplexityUsesConfiguredThresholds(t *testing.T) {
	router := NewTaskRouter("complexity_aware", config.ComplexityConfig{
		SimpleThresholdTokens:  200,
		ComplexThresholdTokens: 800,
	})

	if got := router.DetectComplexity(nil, nil, 199); got != ComplexitySimple {
		t.Fatalf("complexity = %s, want simple", got)
	}
	if got := router.DetectComplexity(nil, nil, 801); got != ComplexityComplex {
		t.Fatalf("complexity = %s, want complex", got)
	}
}

func TestTaskRouterDetectComplexityUsesConfiguredKeywords(t *testing.T) {
	router := NewTaskRouter("complexity_aware", config.ComplexityConfig{
		ComplexKeywords: []string{"saga"},
	})
	body := []byte(`{"messages":[{"content":"please design a saga workflow"}]}`)

	if got := router.DetectComplexity(nil, body, 0); got != ComplexityComplex {
		t.Fatalf("complexity = %s, want complex", got)
	}
}

func TestTaskRouterDetectComplexityHeaderOverride(t *testing.T) {
	router := NewTaskRouter("complexity_aware", config.ComplexityConfig{})

	got := router.DetectComplexity(map[string]string{"X-Task-Complexity": "simple"}, nil, 9000)
	if got != ComplexitySimple {
		t.Fatalf("complexity = %s, want simple", got)
	}
}
