package httpclient

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig(30 * time.Second)

	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, 10*time.Second, config.ConnectTimeout)
	assert.Equal(t, 100, config.MaxIdleConns)
	assert.Equal(t, 10, config.MaxIdleConnsPerHost)
	assert.Equal(t, 90*time.Second, config.IdleConnTimeout)
	assert.Equal(t, 3, config.MaxRetries)
	assert.Equal(t, 500*time.Millisecond, config.RetryDelay)
	assert.True(t, len(config.RetryableStatusCodes) > 0)
	assert.False(t, config.EnableMetrics)
	assert.NotNil(t, config.Logger)
}

func TestNewClient(t *testing.T) {
	config := &Config{
		Timeout:        30 * time.Second,
		ConnectTimeout: 10 * time.Second,
		MaxIdleConns:  50,
		MaxRetries:    3,
	}

	client := NewClient(config)

	assert.NotNil(t, client)
	assert.NotNil(t, client.client)
	assert.Equal(t, 30*time.Second, client.client.Timeout)
}

func TestNewClientWithTimeout(t *testing.T) {
	client := NewClientWithTimeout(45 * time.Second)

	assert.NotNil(t, client)
	assert.NotNil(t, client.client)
	assert.Equal(t, 45*time.Second, client.client.Timeout)
}

func TestNewClientNilConfig(t *testing.T) {
	// Should use default config
	client := NewClient(nil)

	assert.NotNil(t, client)
	assert.NotNil(t, client.client)
	assert.Equal(t, 30*time.Second, client.client.Timeout)
}

func TestHealthCheck(t *testing.T) {
	client := HealthCheck(5 * time.Second)

	assert.NotNil(t, client)
	assert.NotNil(t, client.client)
	assert.Equal(t, 5*time.Second, client.client.Timeout)
	// HealthCheck should have MaxRetries = 0
	assert.Equal(t, 0, client.config.MaxRetries)
}

func TestClient_UnderlyingClient(t *testing.T) {
	client := NewClientWithTimeout(30 * time.Second)

	underlying := client.UnderlyingClient()

	assert.NotNil(t, underlying)
	assert.IsType(t, &http.Client{}, underlying)
}

func TestClient_MetricsEnabled(t *testing.T) {
	config := &Config{
		Timeout:       30 * time.Second,
		EnableMetrics: true,
	}

	client := NewClient(config)

	assert.NotNil(t, client.Metrics())
}

func TestClient_MetricsDisabled(t *testing.T) {
	config := &Config{
		Timeout:       30 * time.Second,
		EnableMetrics: false,
	}

	client := NewClient(config)

	assert.Nil(t, client.Metrics())
}

func TestMetrics_GetMetrics(t *testing.T) {
	m := NewMetrics()

	snapshot := m.GetMetrics()

	assert.Equal(t, int64(0), snapshot.TotalRequests)
	assert.Equal(t, int64(0), snapshot.SuccessfulRequests)
	assert.Equal(t, int64(0), snapshot.FailedRequests)
}

func TestClient_calculateBackoff(t *testing.T) {
	config := &Config{
		Timeout:    10 * time.Second,
		RetryDelay: 500 * time.Millisecond,
	}
	client := &Client{config: config}

	// Test exponential backoff
	backoff1 := client.calculateBackoff(1)
	backoff2 := client.calculateBackoff(2)
	backoff3 := client.calculateBackoff(3)

	// Each backoff should be roughly 2x the previous (with jitter)
	assert.True(t, backoff1 > 0)
	assert.True(t, backoff2 > backoff1/2)
	assert.True(t, backoff3 > backoff2/2)

	// Should not exceed timeout
	assert.True(t, backoff1 <= 10*time.Second)
	assert.True(t, backoff2 <= 10*time.Second)
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "none",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestDoWithRetry_RewindsBodyOnRetry 坐实修复：可重试响应（503）后再次发送时，
// 请求体必须被 rewind，服务端在重试中应收到完整 body，而不是空 body（这正是线上
// "ContentLength=N with Body length 0" → unknown 失败的根因）。
func TestDoWithRetry_RewindsBodyOnRetry(t *testing.T) {
	const payload = `{"url":"http://example.com/v1/scrape"}`

	var mu sync.Mutex
	var receivedBodies []string
	var attempts int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		attempts++
		n := attempts
		receivedBodies = append(receivedBodies, string(body))
		mu.Unlock()

		if n == 1 {
			// 首次返回可重试的 503，迫使客户端重试。
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`ok`))
	}))
	defer srv.Close()

	client := NewClient(&Config{
		Timeout:              5 * time.Second,
		ConnectTimeout:       2 * time.Second,
		MaxRetries:           2,
		RetryDelay:           1 * time.Millisecond,
		RetryableStatusCodes: []int{http.StatusServiceUnavailable},
		Logger:               &defaultLogger{},
	})

	req, err := http.NewRequest(http.MethodPost, srv.URL, bytes.NewReader([]byte(payload)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck

	mu.Lock()
	defer mu.Unlock()
	// 关键断言：发生了重试，且两次请求 body 都是完整 payload（第二次没有被耗空）。
	require.GreaterOrEqual(t, attempts, 2, "expected the request to be retried")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	for i, got := range receivedBodies {
		assert.Equal(t, payload, got, "attempt %d received truncated/empty body", i+1)
	}
}

// TestDoWithRetry_NonRewindableBodyNotRetried 断言不可 rewind 的 body（GetBody==nil）
// 不会被盲目重发成空 body —— 首个响应原样返回，不再触发注定失败的空体重试。
func TestDoWithRetry_NonRewindableBodyNotRetried(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client := NewClient(&Config{
		Timeout:              5 * time.Second,
		MaxRetries:           2,
		RetryDelay:           1 * time.Millisecond,
		RetryableStatusCodes: []int{http.StatusServiceUnavailable},
		Logger:               &defaultLogger{},
	})

	// 用 io.NopCloser 包裹，使 http.NewRequest 无法自动填充 GetBody（body 不可重放）。
	req, err := http.NewRequest(http.MethodPost, srv.URL, io.NopCloser(bytes.NewReader([]byte(`{}`))))
	require.NoError(t, err)
	require.Nil(t, req.GetBody, "precondition: GetBody must be nil for this case")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.Equal(t, 1, attempts, "non-rewindable body must not be re-sent as an empty body")
}
