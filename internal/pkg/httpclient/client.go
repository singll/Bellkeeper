package httpclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net"
	"net/http"
	"sync"
	"time"
)

// Config holds configuration for the HTTP client
type Config struct {
	// Timeout is the overall request timeout
	Timeout time.Duration

	// ConnectTimeout is the timeout for establishing connections
	ConnectTimeout time.Duration

	// MaxIdleConns is the maximum number of idle connections
	MaxIdleConns int

	// MaxIdleConnsPerHost limits idle connections per host
	MaxIdleConnsPerHost int

	// IdleConnTimeout is how long idle connections are kept alive
	IdleConnTimeout time.Duration

	// MaxRetries is the number of retries for failed requests (0 = no retries)
	MaxRetries int

	// RetryDelay is the base delay between retries (will use exponential backoff)
	RetryDelay time.Duration

	// RetryableStatusCodes are HTTP status codes that should be retried
	RetryableStatusCodes []int

	// EnableMetrics enables request metrics collection
	EnableMetrics bool

	// Logger is used for logging retries and errors
	Logger Logger
}

// Logger interface for logging
type Logger interface {
	Printf(format string, v ...interface{})
}

// defaultLogger uses log.Printf
type defaultLogger struct{}

func (d *defaultLogger) Printf(format string, v ...interface{}) {
	log.Printf("[httpclient] "+format, v...)
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig(timeout time.Duration) *Config {
	return &Config{
		Timeout:             timeout,
		ConnectTimeout:      10 * time.Second,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		MaxRetries:         3,
		RetryDelay:          500 * time.Millisecond,
		RetryableStatusCodes: []int{
			http.StatusRequestTimeout,
			http.StatusTooManyRequests,
			502, 503, 504, // Bad Gateway, Service Unavailable, Gateway Timeout
		},
		EnableMetrics: false,
		Logger:        &defaultLogger{},
	}
}

// Client wraps http.Client with retry, connection pooling, and metrics
type Client struct {
	client  *http.Client
	config  *Config
	metrics *Metrics
}

// Metrics holds request metrics
type Metrics struct {
	mu                 sync.RWMutex
	TotalRequests      int64
	SuccessfulRequests  int64
	FailedRequests     int64
	RetriedRequests    int64
	TotalRetries       int64
	TotalDuration      time.Duration
	StatusCodeCounts   map[int]int64
	ErrorCounts        map[string]int64
}

// NewMetrics creates a new Metrics instance
func NewMetrics() *Metrics {
	return &Metrics{
		StatusCodeCounts: make(map[int]int64),
		ErrorCounts:      make(map[string]int64),
	}
}

// GetMetrics returns a copy of current metrics
func (m *Metrics) GetMetrics() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return MetricsSnapshot{
		TotalRequests:     m.TotalRequests,
		SuccessfulRequests: m.SuccessfulRequests,
		FailedRequests:    m.FailedRequests,
		RetriedRequests:   m.RetriedRequests,
		TotalRetries:     m.TotalRetries,
		AvgDuration:       avgDuration(m.TotalDuration, m.TotalRequests),
		StatusCodeCounts:  copyMap(m.StatusCodeCounts),
		ErrorCounts:       copyStringMap(m.ErrorCounts),
	}
}

// MetricsSnapshot is a point-in-time copy of metrics
type MetricsSnapshot struct {
	TotalRequests      int64
	SuccessfulRequests int64
	FailedRequests    int64
	RetriedRequests   int64
	TotalRetries      int64
	AvgDuration       time.Duration
	StatusCodeCounts   map[int]int64
	ErrorCounts        map[string]int64
}

func avgDuration(d time.Duration, n int64) time.Duration {
	if n == 0 {
		return 0
	}
	return d / time.Duration(n)
}

func copyMap(m map[int]int64) map[int]int64 {
	r := make(map[int]int64, len(m))
	for k, v := range m {
		r[k] = v
	}
	return r
}

func copyStringMap(m map[string]int64) map[string]int64 {
	r := make(map[string]int64, len(m))
	for k, v := range m {
		r[k] = v
	}
	return r
}

// NewClient creates an HTTP client with the given config
func NewClient(config *Config) *Client {
	if config == nil {
		config = DefaultConfig(30 * time.Second)
	}
	if config.Logger == nil {
		config.Logger = &defaultLogger{}
	}

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   config.ConnectTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        config.MaxIdleConns,
		MaxIdleConnsPerHost: config.MaxIdleConnsPerHost,
		IdleConnTimeout:     config.IdleConnTimeout,
		TLSHandshakeTimeout:  10 * time.Second,
	}

	client := &http.Client{
		Timeout:   config.Timeout,
		Transport: transport,
	}

	var metrics *Metrics
	if config.EnableMetrics {
		metrics = NewMetrics()
	}

	return &Client{
		client:  client,
		config:  config,
		metrics: metrics,
	}
}

// NewClientWithTimeout is a convenience function for creating a client with just a timeout
func NewClientWithTimeout(timeout time.Duration) *Client {
	return NewClient(DefaultConfig(timeout))
}

// Do performs an HTTP request with retry logic
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	if c.metrics != nil {
		return c.doWithMetrics(req)
	}
	return c.doWithRetry(req)
}

func (c *Client) doWithRetry(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := c.calculateBackoff(attempt)
			c.config.Logger.Printf("retrying request (attempt %d/%d) after %v: %s %s",
				attempt, c.config.MaxRetries, delay, req.Method, req.URL.String())
			time.Sleep(delay)

			// Rewind the request body before re-sending. The first Do() consumed the
			// body reader to EOF; without rebuilding it the transport sees a non-zero
			// ContentLength but a 0-byte body and fails with
			// "ContentLength=N with Body length 0" (never actually retrying).
			// http.NewRequest populates GetBody for in-memory bodies (bytes.Reader,
			// bytes.Buffer, strings.Reader), so this covers our JSON POSTs.
			if req.Body != nil {
				if req.GetBody == nil {
					// Non-rewindable body (e.g. a raw stream) — cannot safely retry.
					c.config.Logger.Printf("cannot retry request with non-rewindable body: %s %s",
						req.Method, req.URL.String())
					return resp, err
				}
				body, rewindErr := req.GetBody()
				if rewindErr != nil {
					return resp, fmt.Errorf("rewind request body for retry: %w", rewindErr)
				}
				req.Body = body
			}
		}

		resp, err = c.client.Do(req)

		if err == nil && c.isRetryableResponse(resp) {
			if c.metrics != nil {
				c.metrics.mu.Lock()
				c.metrics.RetriedRequests++
				c.metrics.TotalRetries++
				c.metrics.mu.Unlock()
			}
			// Drain and close the retryable response body so the connection can be
			// reused and the next attempt starts clean.
			if resp.Body != nil {
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close() //nolint:errcheck
			}
			continue
		}

		// Success or non-retryable error
		break
	}

	return resp, err
}

func (c *Client) doWithMetrics(req *http.Request) (*http.Response, error) {
	start := time.Now()

	c.metrics.mu.Lock()
	c.metrics.TotalRequests++
	c.metrics.mu.Unlock()

	resp, err := c.doWithRetry(req)

	duration := time.Since(start)

	c.metrics.mu.Lock()
	c.metrics.TotalDuration += duration

	if err != nil {
		c.metrics.FailedRequests++
		errType := classifyError(err)
		c.metrics.ErrorCounts[errType]++
	} else {
		c.metrics.SuccessfulRequests++
		c.metrics.StatusCodeCounts[resp.StatusCode]++
	}
	c.metrics.mu.Unlock()

	return resp, err
}

func (c *Client) isRetryableResponse(resp *http.Response) bool {
	if resp == nil {
		return true
	}

	for _, code := range c.config.RetryableStatusCodes {
		if resp.StatusCode == code {
			return true
		}
	}

	// Also retry 5xx errors
	if resp.StatusCode >= 500 && resp.StatusCode < 600 {
		return true
	}

	return false
}

func (c *Client) calculateBackoff(attempt int) time.Duration {
	// Exponential backoff with jitter
	base := float64(c.config.RetryDelay)
	exp := math.Pow(2, float64(attempt-1))
	maxDelay := float64(c.config.Timeout)

	delay := base * exp
	// Add jitter (±25%)
	jitter := delay * 0.25 * (rand.Float64()*2 - 1)
	delay += jitter

	// Cap at timeout
	if delay > maxDelay {
		delay = maxDelay
	}

	return time.Duration(delay)
}

func classifyError(err error) string {
	if err == nil {
		return "none"
	}

	if netErr, ok := err.(net.Error); ok {
		if netErr.Timeout() {
			return "timeout"
		}
		return "network"
	}

	if err == io.EOF {
		return "eof"
	}

	return "other"
}

// newJSONRequest creates a new request with a JSON body
func newJSONRequest(method, url string, body interface{}) (*http.Request, error) {
	var buf *bytes.Buffer
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		buf = bytes.NewBuffer(data)
	} else {
		buf = &bytes.Buffer{}
	}
	return http.NewRequest(method, url, buf)
}

// Get performs a GET request
func (c *Client) Get(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// Post performs a POST request with JSON body
func (c *Client) Post(url, contentType string, body interface{}) (*http.Response, error) {
	req, err := newJSONRequest(http.MethodPost, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return c.Do(req)
}

// PostJSON performs a POST request with application/json content type
func (c *Client) PostJSON(url string, body interface{}) (*http.Response, error) {
	return c.Post(url, "application/json", body)
}

// Put performs a PUT request
func (c *Client) Put(url, contentType string, body interface{}) (*http.Response, error) {
	req, err := newJSONRequest(http.MethodPut, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return c.Do(req)
}

// Delete performs a DELETE request
func (c *Client) Delete(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// UnderlyingClient returns the underlying http.Client for advanced use
func (c *Client) UnderlyingClient() *http.Client {
	return c.client
}

// Metrics returns the current metrics (nil if metrics not enabled)
func (c *Client) Metrics() *Metrics {
	return c.metrics
}

// HealthCheck creates a client optimized for health checks
func HealthCheck(timeout time.Duration) *Client {
	return NewClient(&Config{
		Timeout:             timeout,
		ConnectTimeout:      5 * time.Second,
		MaxIdleConns:        5,
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     30 * time.Second,
		MaxRetries:         0, // No retries for health checks
		Logger:             &defaultLogger{},
	})
}
