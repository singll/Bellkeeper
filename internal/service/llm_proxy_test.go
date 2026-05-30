package service

import (
	"testing"

	"github.com/singll/bellkeeper/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestTokenBucket_TryAcquire(t *testing.T) {
	tests := []struct {
		name        string
		rpm         int
		rpd         int
		acquireNum  int
		wantAllowed int
	}{
		{
			name:        "basic acquire",
			rpm:         60,
			rpd:         0,
			acquireNum:  5,
			wantAllowed: 5,
		},
		{
			name:        "exhaust tokens",
			rpm:         3,
			rpd:         0,
			acquireNum:  5,
			wantAllowed: 3,
		},
		{
			name:        "default bucket rpm fallback",
			rpm:         0,
			rpd:         0,
			acquireNum:  3,
			wantAllowed: 3, // Falls back to defaultBucketRPM (60)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tb := NewTokenBucket(tt.rpm, tt.rpd, 60)
			allowed := 0
			for i := 0; i < tt.acquireNum; i++ {
				if ok, _ := tb.TryAcquire(); ok {
					allowed++
				}
			}
			assert.Equal(t, tt.wantAllowed, allowed)
		})
	}
}

func TestTokenBucket_DailyLimit(t *testing.T) {
	tb := NewTokenBucket(1000, 5, 60) // 5 requests per day

	// Should allow up to daily limit
	for i := 0; i < 5; i++ {
		ok, _ := tb.TryAcquire()
		assert.True(t, ok, "request %d should be allowed", i+1)
	}

	// Should block after daily limit
	ok, wait := tb.TryAcquire()
	assert.False(t, ok, "should be blocked by daily limit")
	assert.True(t, wait > 0, "should suggest wait time")
}

func TestComputeMicroCents(t *testing.T) {
	// DeepSeek V3: 14¢/1M input, 28¢/1M output. 1000 prompt tokens of input alone
	// is 0.014¢ — old integer-cent math truncated this to 0 (audit #13). Micro-cents
	// must capture it as 14 micro-cents.
	deepseek := &model.LLMModelPricing{InputPricePer1MCents: 14, OutputPricePer1MCents: 28}
	got := computeMicroCents(deepseek, Usage{PromptTokens: 1000})
	assert.Equal(t, int64(14), got, "1000 deepseek input tokens should cost 14 micro-cents, not 0")

	// input + output
	got = computeMicroCents(deepseek, Usage{PromptTokens: 1000, CompletionTokens: 1000})
	assert.Equal(t, int64(14+28), got)

	// #14: cached default price = input/10. With input=14, cached rate = 1.4¢/1M.
	// 1000 cached tokens = 1.4 micro-cents → 1 (floor of 1.4) — NOT 0. The old
	// GetCachedInputPrice()=input/10=1 then *tokens/1M floored the rate first.
	got = computeMicroCents(deepseek, Usage{PromptTokens: 1000, CachedTokens: 1000})
	assert.Equal(t, int64(1), got, "1000 cached tokens at input/10 should be 1 micro-cent, not 0")

	// Explicit cached price overrides the default.
	withCached := &model.LLMModelPricing{InputPricePer1MCents: 300, OutputPricePer1MCents: 1500, CachedInputPricePer1MCents: 30}
	got = computeMicroCents(withCached, Usage{PromptTokens: 10000, CachedTokens: 8000, CompletionTokens: 500})
	// uncached input: 2000*300/1000=600; cached: 8000*30/1000=240; output: 500*1500/1000=750
	assert.Equal(t, int64(600+240+750), got)

	// Rounding to display cents (half up): 1499 micro-cents → 1¢, 1500 → 2¢.
	assert.Equal(t, int64(1), MicroCentsToCents(1499))
	assert.Equal(t, int64(2), MicroCentsToCents(1500))
	assert.Equal(t, int64(0), MicroCentsToCents(14))
}
