package llmgateway

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/pkg/httpclient"
	"go.uber.org/zap"
)

// --- Channel ---

// Channel represents a single upstream LLM API endpoint with its own rate limiter
// and health tracker.
type Channel struct {
	Config      config.ChannelConfig
	Bucket      *TokenBucket
	Client      *httpclient.Client
	Health      *ChannelHealth
	ewmaLatency float64 // ms, exponential weighted moving average
	mu          sync.Mutex
}

// RecordLatency updates the channel's EWMA latency.
func (ch *Channel) RecordLatency(durationMs int) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	if ch.ewmaLatency == 0 {
		ch.ewmaLatency = float64(durationMs)
	} else {
		const alpha = 0.1
		ch.ewmaLatency = alpha*float64(durationMs) + (1-alpha)*ch.ewmaLatency
	}
}

// EWMALatency returns the current EWMA latency in milliseconds.
func (ch *Channel) EWMALatency() float64 {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	return ch.ewmaLatency
}

// prepareUpstreamRequest is the single request-preparation path shared by the
// buffered and streaming paths, so protocol conversion, vendor hooks and auth
// headers can never drift between them.
func (s *LLMProxyService) prepareUpstreamRequest(
	ch *Channel,
	method, path string,
	headers http.Header,
	body []byte,
) (*http.Request, PreparedRequest, UpstreamAdapter, error) {
	adapter, err := adapterFor(ch)
	if err != nil {
		return nil, PreparedRequest{}, nil, err
	}
	prep, err := adapter.PrepareRequest(body, path, extractModelFromBody(body), ch)
	if err != nil {
		return nil, PreparedRequest{}, nil, err
	}

	targetURL := strings.TrimRight(ch.Config.BaseURL, "/") + prep.Path
	req, err := http.NewRequest(method, targetURL, bytes.NewReader(prep.Body))
	if err != nil {
		return nil, PreparedRequest{}, nil, fmt.Errorf("create request: %w", err)
	}

	// Copy caller headers, override auth.
	for k, vv := range headers {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}
	// Strip client headers that must not leak upstream:
	//   X-API-Key       — internal Bellkeeper auth; some upstreams (e.g. kimi-code) reject it
	//   Accept-Encoding — forwarding the client's value disables Go's transparent gzip
	//                     decompression, leaking compressed bytes to the caller
	//                     (moonshot gzips large responses → JSON decode '\x1f' failures).
	req.Header.Del("X-API-Key")
	req.Header.Del("Accept-Encoding")
	adapter.ApplyAuth(req, ch, prep)

	return req, prep, adapter, nil
}

func (s *LLMProxyService) tryChannel(
	ch *Channel,
	method, path string,
	headers http.Header,
	body []byte,
	callerID string,
	tokenID uint,
) (int, []byte, http.Header, error) {
	maxRetries := s.cfg.MaxRetries
	modelName := extractModelFromBody(body)
	var lastStatusCode int
	var lastBody []byte
	var lastHeaders http.Header

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Client-side rate limiting
		allowed, waitTime := ch.Bucket.TryAcquire()
		if !allowed {
			if attempt < maxRetries {
				maxWait := time.Duration(s.cfg.MaxWaitSeconds) * time.Second
				if waitTime > maxWait {
					waitTime = maxWait
				}
				middleware.GetLogger().Warn("bucket throttle, waiting",
					zap.String("channel", ch.Config.Name), zap.Duration("wait", waitTime),
					zap.Int("attempt", attempt+1), zap.Int("max_retries", maxRetries))
				time.Sleep(waitTime)
				continue
			}
			s.logRequest(ch.Config.Name, modelName, path, 429, true, attempt, 0, 0, 0, 0,
				"client rate limit exhausted", callerID, tokenID)
			return 429, []byte(`{"error":"rate limit: bucket exhausted after retries"}`), nil, nil
		}

		req, prep, adapter, err := s.prepareUpstreamRequest(ch, method, path, headers, body)
		if err != nil {
			return 400, []byte(`{"error":"upstream request preparation failed"}`), nil, err
		}

		// Forward request upstream
		start := time.Now()
		// Track half-open inflight requests for circuit breaker concurrency control
		ch.Health.TrackHalfOpenRequest()

		resp, err := ch.Client.Do(req)
		durationMs := int(time.Since(start).Milliseconds())

		if err != nil {
			s.logRequest(ch.Config.Name, prep.Model, path, 0, false, attempt, durationMs, 0, 0, 0,
				err.Error(), callerID, tokenID)
			return 0, nil, nil, fmt.Errorf("upstream request: %w", err)
		}

		respBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close() //nolint:errcheck

		respBytes, usage, decodeErr := adapter.DecodeBuffered(respBytes, resp.StatusCode, prep.Model)
		if decodeErr != nil {
			middleware.GetLogger().Warn("upstream response decode failed",
				zap.String("channel", ch.Config.Name), zap.Error(decodeErr))
		}

		lastStatusCode = resp.StatusCode
		lastBody = respBytes
		lastHeaders = resp.Header

		isRateLimit := resp.StatusCode == 429
		s.logRequest(ch.Config.Name, modelName, path, resp.StatusCode, isRateLimit,
			attempt, durationMs, usage.PromptTokens, usage.CompletionTokens, usage.CachedTokens, "", callerID, tokenID)

		// Record latency for routing strategy scoring
		if resp.StatusCode < 500 && resp.StatusCode != 429 {
			ch.RecordLatency(durationMs)
		}

		if resp.StatusCode == 429 {
			// Adaptive rate-limit learning (stage 3): an upstream 429 means our
			// configured RPM is too high. Record it so the learner downgrades the
			// learned-safe RPM to ~85% of the configured rate. Use prep.Model (the
			// rewritten upstream model), which is what the learner keys on.
			if s.rateLimitLearner != nil && ch.Config.ID != 0 {
				s.rateLimitLearner.Record429(ch.Config.ID, prep.Model, ch.Config.RPM)
			}
			if attempt < maxRetries {
				backoff := s.calculateBackoff(attempt)
				middleware.GetLogger().Warn("upstream 429, backing off",
					zap.String("channel", ch.Config.Name), zap.Duration("backoff", backoff),
					zap.Int("attempt", attempt+1), zap.Int("max_retries", maxRetries))
				time.Sleep(backoff)
				continue
			}
		}

		return resp.StatusCode, respBytes, resp.Header, nil
	}

	return lastStatusCode, lastBody, lastHeaders, nil
}

// calculateBackoff returns exponential backoff with jitter, using configured cap and jitter factor.
func (s *LLMProxyService) calculateBackoff(attempt int) time.Duration {
	base := math.Pow(2, float64(attempt+1))
	cap := float64(s.cfg.BackoffCapSeconds)
	if base > cap {
		base = cap
	}
	jitter := base * s.cfg.BackoffJitter * rand.Float64()
	return time.Duration((base + jitter) * float64(time.Second))
}

// tryChannelStream sends a streaming request to a single channel.
// No retries — once a stream starts, you can't retry.
func (s *LLMProxyService) tryChannelStream(
	ch *Channel,
	method, path string,
	headers http.Header,
	body []byte,
	callerID string,
) (*StreamResult, error) {
	// Single rate limit check (no retry loop)
	allowed, waitTime := ch.Bucket.TryAcquire()
	if !allowed {
		maxWait := time.Duration(s.cfg.MaxWaitSeconds) * time.Second
		if waitTime > maxWait {
			waitTime = maxWait
		}
		// For streaming, we can wait once but not retry
		if waitTime <= maxWait {
			middleware.GetLogger().Warn("stream bucket throttle, waiting once",
				zap.String("channel", ch.Config.Name), zap.Duration("wait", waitTime))
			time.Sleep(waitTime)
		}
	}

	req, prep, adapter, err := s.prepareUpstreamRequest(ch, method, path, headers, body)
	if err != nil {
		return nil, fmt.Errorf("prepare stream request: %w", err)
	}

	// Use the underlying http.Client without timeout for streaming
	// (the httpclient.Client has an overall Timeout which kills long streams).
	streamClient := ch.Client.UnderlyingClient()
	noTimeoutClient := &http.Client{
		Transport: streamClient.Transport,
		Timeout:   0,
	}

	startedAt := time.Now()
	// Track half-open inflight requests for circuit breaker concurrency control
	ch.Health.TrackHalfOpenRequest()
	resp, err := noTimeoutClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream stream request: %w", err)
	}

	if resp.StatusCode >= 400 {
		respBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close() //nolint:errcheck

		respBytes, _, _ = adapter.DecodeBuffered(respBytes, resp.StatusCode, prep.Model)

		return &StreamResult{
			StatusCode:   resp.StatusCode,
			BodyReader:   io.NopCloser(bytes.NewReader(respBytes)),
			RespHeaders:  resp.Header,
			ProviderType: ch.Config.ProviderType,
			ChannelName:  ch.Config.Name,
			StartedAt:    startedAt,
		}, nil
	}

	// Wrap in the usage tracker first so it observes the raw upstream stream
	// (Anthropic usage events are dropped by the OpenAI SSE conversion), then let
	// the adapter normalise the protocol before the caller sees it.
	tracker := newStreamUsageTracker(resp.Body)
	reader := adapter.WrapStream(tracker)

	return &StreamResult{
		StatusCode:   resp.StatusCode,
		BodyReader:   reader,
		RespHeaders:  resp.Header,
		ProviderType: ch.Config.ProviderType,
		ChannelName:  ch.Config.Name,
		StartedAt:    startedAt,
		tracker:      tracker,
	}, nil
}
