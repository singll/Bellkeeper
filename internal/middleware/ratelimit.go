package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimiter implements a simple in-memory rate limiter.
type RateLimiter struct {
	mu       sync.Mutex
	wg       sync.WaitGroup
	requests map[string]*clientRequests
	limit    int
	window   time.Duration
	stopCh   chan struct{}
	running  bool
}

type clientRequests struct {
	count       int
	windowStart time.Time
}

// NewRateLimiter creates a new rate limiter.
// limit: maximum requests per window
// window: time window duration
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		requests: make(map[string]*clientRequests),
		limit:    limit,
		window:   window,
		stopCh:   make(chan struct{}),
	}
	return rl
}

func (rl *RateLimiter) Start() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if rl.running {
		return
	}
	stopCh := rl.stopCh
	rl.running = true
	rl.wg.Add(1)
	go func() {
		defer rl.wg.Done()
		rl.cleanup(stopCh)
	}()
}

func (rl *RateLimiter) Stop() {
	rl.mu.Lock()
	if !rl.running {
		rl.mu.Unlock()
		return
	}
	close(rl.stopCh)
	rl.stopCh = make(chan struct{})
	rl.running = false
	rl.mu.Unlock()
	rl.wg.Wait()
}

// Allow checks if a request from the given key should be allowed.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	client, exists := rl.requests[key]

	if !exists || now.Sub(client.windowStart) >= rl.window {
		// Start new window
		rl.requests[key] = &clientRequests{
			count:       1,
			windowStart: now,
		}
		return true
	}

	if client.count >= rl.limit {
		return false
	}

	client.count++
	return true
}

// cleanup removes expired entries periodically.
func (rl *RateLimiter) cleanup(stopCh <-chan struct{}) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			for key, client := range rl.requests {
				if now.Sub(client.windowStart) >= rl.window {
					delete(rl.requests, key)
				}
			}
			rl.mu.Unlock()
		case <-stopCh:
			return
		}
	}
}

// RateLimitMiddleware returns a Gin middleware that rate limits requests.
func RateLimitMiddleware(limit int, window time.Duration) gin.HandlerFunc {
	limiter := NewRateLimiter(limit, window)
	limiter.Start()

	return func(c *gin.Context) {
		// Use client IP as the key
		key := c.ClientIP()

		if !limiter.Allow(key) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"error":   "rate limit exceeded",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
