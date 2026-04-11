package httpclient

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
