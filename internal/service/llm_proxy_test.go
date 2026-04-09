package service

import (
	"testing"

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
