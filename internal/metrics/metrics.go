package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// HTTP request metrics
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bellkeeper_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "bellkeeper_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	// LLM Proxy metrics
	LLMProxyRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bellkeeper_llm_proxy_requests_total",
			Help: "Total number of LLM proxy requests",
		},
		[]string{"channel", "model", "status"},
	)

	LLMProxyRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "bellkeeper_llm_proxy_request_duration_seconds",
			Help:    "LLM proxy request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"channel", "model"},
	)

	LLMProxyTokensTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bellkeeper_llm_proxy_tokens_total",
			Help: "Total number of LLM tokens",
		},
		[]string{"channel", "type"}, // type: prompt, completion
	)

	// Activity log metrics
	ActivityLogTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bellkeeper_activity_log_total",
			Help: "Total number of activity log entries",
		},
		[]string{"module", "action", "status"},
	)

	// File ingestion metrics
	FileIngestionTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bellkeeper_file_ingestion_total",
			Help: "Total number of file ingestion operations",
		},
		[]string{"status", "extractor"},
	)

	// Matrix metrics
	MatrixMessagesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bellkeeper_matrix_messages_total",
			Help: "Total number of Matrix messages processed",
		},
		[]string{"type", "status"},
	)

	MatrixCommandsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bellkeeper_matrix_commands_total",
			Help: "Total number of Matrix commands executed",
		},
		[]string{"command", "status"},
	)
)
