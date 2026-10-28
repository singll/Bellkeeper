package llmgateway

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/llmgateway/balance"
	"github.com/singll/bellkeeper/internal/pkg/httpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type routingScenario struct {
	name          string
	firstStatus   int
	firstText     string
	secondStatus  int
	secondText    string
	wantStatus    int
	wantBody      string
	wantFirstHits int32
	wantSecond    int32
	wantHealth    float64
}

func newConsistencyService(group *ModelGroup, channels ...*Channel) *LLMProxyService {
	chMap := make(map[string]*Channel, len(channels))
	modelMap := make(map[string][]*Channel)
	for _, ch := range channels {
		chMap[ch.Config.Name] = ch
	}
	return &LLMProxyService{
		cfg:         config.LLMProxyConfig{MaxRetries: 0, MaxWaitSeconds: 1, DefaultBucketRPM: 1000},
		channels:    chMap,
		modelMap:    modelMap,
		modelGroups: map[string]*ModelGroup{"pool": group},
		taskRouter:  NewTaskRouter("complexity_aware", config.ComplexityConfig{}),
		convMgr:     NewConversationBindingManager(nil, time.Hour),
	}
}

func scenarioResponse(status int, text, mode string) string {
	if status >= 400 {
		return fmt.Sprintf(`{"error":%q}`, text)
	}
	if mode == "stream" {
		return fmt.Sprintf("data: {\"choices\":[{\"delta\":{\"content\":%q},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", text)
	}
	return fmt.Sprintf(`{"id":"x","choices":[{"index":0,"message":{"role":"assistant","content":%q},"finish_reason":"stop"}]}`, text)
}

func routingUpstream(t *testing.T, status int, text, mode string, hits *int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(hits, 1)
		if mode == "stream" {
			w.Header().Set("Content-Type", "text/event-stream")
		} else {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(scenarioResponse(status, text, mode)))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func runBufferedRouting(t *testing.T, svc *LLMProxyService) (int, string, error) {
	t.Helper()
	status, body, _, err := svc.ProxyRequest(
		http.MethodPost, "/v1/chat/completions", http.Header{},
		[]byte(`{"model":"pool","messages":[{"role":"user","content":"hi"}]}`), "tester", 0,
	)
	return status, string(body), err
}

func runStreamRouting(t *testing.T, svc *LLMProxyService) (int, string, error) {
	t.Helper()
	result, err := svc.ProxyStreamRequest(
		http.MethodPost, "/v1/chat/completions", http.Header{},
		[]byte(`{"model":"pool","messages":[{"role":"user","content":"hi"}]}`), "tester", 0,
	)
	if err != nil {
		return 0, "", err
	}
	defer result.BodyReader.Close() //nolint:errcheck
	body, readErr := io.ReadAll(result.BodyReader)
	if readErr != nil {
		return result.StatusCode, string(body), readErr
	}
	return result.StatusCode, string(body), nil
}

func TestBufferedAndStreamRoutingConsistency(t *testing.T) {
	scenarios := []routingScenario{
		{
			name:          "success uses first member",
			firstStatus:   200,
			firstText:     "ok-first",
			secondStatus:  200,
			secondText:    "ok-second",
			wantStatus:    200,
			wantBody:      "ok-first",
			wantFirstHits: 1,
			wantSecond:    0,
			wantHealth:    1.0,
		},
		{
			name:          "4xx fails over to next member",
			firstStatus:   400,
			firstText:     "bad-first",
			secondStatus:  200,
			secondText:    "ok-second",
			wantStatus:    200,
			wantBody:      "ok-second",
			wantFirstHits: 1,
			wantSecond:    1,
		},
		{
			name:          "all fail returns last upstream error",
			firstStatus:   400,
			firstText:     "bad-first",
			secondStatus:  400,
			secondText:    "bad-second",
			wantStatus:    400,
			wantBody:      "bad-second",
			wantFirstHits: 1,
			wantSecond:    1,
		},
		{
			name:          "context too long does not count against health",
			firstStatus:   400,
			firstText:     "This model's maximum context length is 100 tokens",
			secondStatus:  200,
			secondText:    "ok-second",
			wantStatus:    200,
			wantBody:      "ok-second",
			wantFirstHits: 1,
			wantSecond:    1,
			wantHealth:    1.0,
		},
	}

	for _, sc := range scenarios {
		for _, mode := range []string{"buffered", "stream"} {
			t.Run(sc.name+"/"+mode, func(t *testing.T) {
				var firstHits, secondHits int32
				first := routingUpstream(t, sc.firstStatus, sc.firstText, mode, &firstHits)
				second := routingUpstream(t, sc.secondStatus, sc.secondText, mode, &secondHits)

				ch1 := &Channel{
					Config: config.ChannelConfig{Name: "first", BaseURL: first.URL, ProviderType: "openai", Priority: 1},
					Bucket: NewTokenBucket(1000, 0, 1000),
					Client: httpclient.NewClientWithTimeout(5 * time.Second),
					Health: NewChannelHealth(config.CircuitBreakerConfig{FailureThreshold: 5, CooldownSeconds: 30}),
				}
				ch2 := &Channel{
					Config: config.ChannelConfig{Name: "second", BaseURL: second.URL, ProviderType: "openai", Priority: 2},
					Bucket: NewTokenBucket(1000, 0, 1000),
					Client: httpclient.NewClientWithTimeout(5 * time.Second),
					Health: NewChannelHealth(config.CircuitBreakerConfig{FailureThreshold: 5, CooldownSeconds: 30}),
				}
				group := newTestGroup(member(ch1, "m1"), member(ch2, "m2"))
				svc := newConsistencyService(group, ch1, ch2)

				var status int
				var body string
				var err error
				if mode == "stream" {
					status, body, err = runStreamRouting(t, svc)
				} else {
					status, body, err = runBufferedRouting(t, svc)
				}

				require.NoError(t, err)
				assert.Equal(t, sc.wantStatus, status)
				assert.Contains(t, body, sc.wantBody)
				assert.Equal(t, sc.wantFirstHits, atomic.LoadInt32(&firstHits))
				assert.Equal(t, sc.wantSecond, atomic.LoadInt32(&secondHits))
				if sc.wantHealth > 0 {
					assert.Equal(t, sc.wantHealth, ch1.Health.HealthScore())
					_, class, _ := ch1.Health.BreakdownInfo()
					assert.Empty(t, class, "context_too_long must not arm a breakdown")
				}
			})
		}
	}
}

func TestStreamDirectFailsOverOn4xx(t *testing.T) {
	var firstHits, secondHits int32
	first := routingUpstream(t, 400, "bad-first", "stream", &firstHits)
	second := routingUpstream(t, 200, "ok-second", "stream", &secondHits)

	ch1 := &Channel{
		Config: config.ChannelConfig{Name: "first", BaseURL: first.URL, ProviderType: "openai", Priority: 1},
		Bucket: NewTokenBucket(1000, 0, 1000),
		Client: httpclient.NewClientWithTimeout(5 * time.Second),
		Health: NewChannelHealth(config.CircuitBreakerConfig{FailureThreshold: 5, CooldownSeconds: 30}),
	}
	ch2 := &Channel{
		Config: config.ChannelConfig{Name: "second", BaseURL: second.URL, ProviderType: "openai", Priority: 2},
		Bucket: NewTokenBucket(1000, 0, 1000),
		Client: httpclient.NewClientWithTimeout(5 * time.Second),
		Health: NewChannelHealth(config.CircuitBreakerConfig{FailureThreshold: 5, CooldownSeconds: 30}),
	}

	svc := &LLMProxyService{
		cfg:         config.LLMProxyConfig{MaxRetries: 0, MaxWaitSeconds: 1, DefaultBucketRPM: 1000},
		channels:    map[string]*Channel{"first": ch1, "second": ch2},
		modelMap:    map[string][]*Channel{"direct-model": {ch1, ch2}},
		modelGroups: map[string]*ModelGroup{},
		taskRouter:  NewTaskRouter("complexity_aware", config.ComplexityConfig{}),
		convMgr:     NewConversationBindingManager(nil, time.Hour),
	}

	result, err := svc.ProxyStreamRequest(
		http.MethodPost, "/v1/chat/completions", http.Header{},
		[]byte(`{"model":"direct-model","messages":[{"role":"user","content":"hi"}]}`), "tester", 0,
	)
	require.NoError(t, err)
	defer result.BodyReader.Close() //nolint:errcheck
	body, err := io.ReadAll(result.BodyReader)
	require.NoError(t, err)

	assert.Equal(t, 200, result.StatusCode)
	assert.Contains(t, string(body), "ok-second")
	assert.Equal(t, int32(1), atomic.LoadInt32(&firstHits))
	assert.Equal(t, int32(1), atomic.LoadInt32(&secondHits))
}

func TestBalanceAwareSelect_NormalizesKinds(t *testing.T) {
	fracCh := newTestChannel("fraction-channel", "standard", false, nil)
	usdCh := newTestChannel("usd-channel", "standard", false, nil)
	group := newTestGroup(member(fracCh, "m"), member(usdCh, "m"))
	group.Config.Strategy = "balance_aware"

	// A healthy 0.5 quota fraction must not be crushed by comparing it directly
	// with a $100 USD balance. After per-kind normalisation both are the best of
	// their kind (1.0), so the first candidate wins on equal latency/error.
	balances := map[string]channelBalance{
		"fraction-channel": {Remaining: 0.5, Kind: balance.KindFraction},
		"usd-channel":      {Remaining: 100, Kind: balance.KindUSD},
	}
	ch, _ := group.SelectChannel("", TaskChat, "", balances, nil)
	require.NotNil(t, ch)
	assert.Equal(t, "fraction-channel", ch.Config.Name)

	// Within a kind, less headroom loses.
	low := newTestChannel("low", "standard", false, nil)
	high := newTestChannel("high", "standard", false, nil)
	group = newTestGroup(member(low, "m"), member(high, "m"))
	group.Config.Strategy = "balance_aware"
	ch, _ = group.SelectChannel("", TaskChat, "", map[string]channelBalance{
		"low":  {Remaining: 0.2, Kind: balance.KindFraction},
		"high": {Remaining: 0.9, Kind: balance.KindFraction},
	}, nil)
	require.NotNil(t, ch)
	assert.Equal(t, "high", ch.Config.Name)
}
