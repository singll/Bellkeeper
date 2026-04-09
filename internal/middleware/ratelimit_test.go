package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestRateLimiter_Allow(t *testing.T) {
	tests := []struct {
		name        string
		limit       int
		window      time.Duration
		requests    int
		wantAllowed int
	}{
		{
			name:        "within limit",
			limit:       5,
			window:      time.Minute,
			requests:    3,
			wantAllowed: 3,
		},
		{
			name:        "at limit",
			limit:       3,
			window:      time.Minute,
			requests:    3,
			wantAllowed: 3,
		},
		{
			name:        "over limit",
			limit:       2,
			window:      time.Minute,
			requests:    5,
			wantAllowed: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rl := NewRateLimiter(tt.limit, tt.window)
			allowed := 0
			for i := 0; i < tt.requests; i++ {
				if rl.Allow("test-key") {
					allowed++
				}
			}
			assert.Equal(t, tt.wantAllowed, allowed)
		})
	}
}

func TestRateLimiter_DifferentKeys(t *testing.T) {
	rl := NewRateLimiter(2, time.Minute)

	// Different keys should have separate limits
	assert.True(t, rl.Allow("key1"))
	assert.True(t, rl.Allow("key1"))
	assert.False(t, rl.Allow("key1")) // exhausted

	assert.True(t, rl.Allow("key2")) // different key, should be allowed
	assert.True(t, rl.Allow("key2"))
}

func TestRateLimitMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		limit       int
		window      time.Duration
		requests    int
		wantStatus  int
	}{
		{
			name:        "allowed requests",
			limit:       10,
			window:      time.Minute,
			requests:    3,
			wantStatus:  http.StatusOK,
		},
		{
			name:        "rate limited",
			limit:       2,
			window:      time.Minute,
			requests:    5,
			wantStatus:  http.StatusTooManyRequests,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := gin.New()
			r.Use(RateLimitMiddleware(tt.limit, tt.window))
			r.GET("/test", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"status": "ok"})
			})

			lastStatus := 0
			for i := 0; i < tt.requests; i++ {
				req := httptest.NewRequest("GET", "/test", nil)
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				lastStatus = w.Code
			}
			assert.Equal(t, tt.wantStatus, lastStatus)
		})
	}
}
