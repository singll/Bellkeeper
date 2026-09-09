package llmgateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/singll/bellkeeper/internal/config"
	llmerrors "github.com/singll/bellkeeper/internal/llmgateway/errors"
	"github.com/singll/bellkeeper/internal/pkg/httpclient"
)

// --- classifier ---

func TestClassifyContextTooLong(t *testing.T) {
	// SenseNova / OpenAI phrasing
	r := llmerrors.Classify(400, `{"error":{"message":"This model's maximum context length is 262144 tokens. However, you requested 16 output tokens and your prompt contains 472970 input tokens","type":"invalid_request_error"}}`, "openai")
	assert.Equal(t, llmerrors.ContextTooLong, r.Class)
	assert.True(t, r.CanRetry)

	// Anthropic phrasing (routed via the kimi-code branch normally — the
	// context check must run before provider-specific classification)
	r = llmerrors.Classify(400, `{"error":{"message":"prompt is too long: 300000 tokens > 200000 maximum","type":"invalid_request_error"}}`, "anthropic")
	assert.Equal(t, llmerrors.ContextTooLong, r.Class)

	// OpenRouter-style code
	r = llmerrors.Classify(400, `{"error":{"code":"context_length_exceeded"}}`, "openai")
	assert.Equal(t, llmerrors.ContextTooLong, r.Class)

	// A generic 400 must NOT be classified as context_too_long
	r = llmerrors.Classify(400, `{"error":{"message":"invalid parameter: foo"}}`, "openai")
	assert.Equal(t, llmerrors.Unknown, r.Class)

	// No breakdown duration for a non-fault
	assert.Equal(t, time.Duration(0), llmerrors.BreakdownDuration("none"))
}

func TestClassifyMaxOutputExceeded(t *testing.T) {
	// SenseNova flash-lite: max_tokens capped at 65536.
	r := llmerrors.Classify(400, `{"error":{"message":"field MaxTokens invalid, should be in [1, 65536]","type":"invalid_request_error","param":"max_tokens","code":"3"}}`, "openai")
	assert.Equal(t, llmerrors.MaxOutputExceeded, r.Class)
	assert.True(t, r.CanRetry)
	assert.Equal(t, time.Duration(0), llmerrors.BreakdownDuration(r.BreakdownUntil))

	// A generic max_tokens-free 400 must stay Unknown.
	r = llmerrors.Classify(400, `{"error":{"message":"invalid parameter: foo"}}`, "openai")
	assert.Equal(t, llmerrors.Unknown, r.Class)
}

func TestClassify401BalanceExhausted(t *testing.T) {
	// OpenCode Go: 401 with a CreditsError body is a balance problem, not auth.
	r := llmerrors.Classify(401, `{"type":"error","error":{"type":"CreditsError","message":"Insufficient balance. Manage your billing here: https://opencode.ai/workspace/x/billing"}}`, "openai")
	assert.Equal(t, llmerrors.BalanceZero, r.Class)
	assert.False(t, r.CanRetry)

	// A plain 401 with no balance hint is still auth_failed.
	r = llmerrors.Classify(401, `{"error":{"message":"invalid api key"}}`, "openai")
	assert.Equal(t, llmerrors.AuthFailed, r.Class)
}

func TestCapOutputTokens(t *testing.T) {
	// max_tokens over the cap is clamped.
	body := []byte(`{"model":"m","max_tokens":131072,"messages":[]}`)
	out := capOutputTokens(body, 65536)
	assert.JSONEq(t, `{"model":"m","max_tokens":65536,"messages":[]}`, string(out))

	// max_completion_tokens is clamped too.
	body = []byte(`{"model":"m","max_completion_tokens":131072}`)
	out = capOutputTokens(body, 65536)
	assert.JSONEq(t, `{"model":"m","max_completion_tokens":65536}`, string(out))

	// Within the cap → unchanged (same bytes).
	body = []byte(`{"model":"m","max_tokens":100}`)
	assert.Equal(t, body, capOutputTokens(body, 65536))

	// No cap declared → unchanged.
	body = []byte(`{"model":"m","max_tokens":131072}`)
	assert.Equal(t, body, capOutputTokens(body, 0))
}

func TestNewModelGroup_DeduplicatesMembers(t *testing.T) {
	ch := newTestChannel("sense", "", false, nil)
	cfg := config.ModelGroupConfig{
		Name: "g",
		Members: []config.ModelGroupMember{
			{Channel: "sense", Model: "glm-5.2", Weight: 5},
			{Channel: "sense", Model: "glm-5.2", Weight: 1},
			{Channel: "sense", Model: "flash-lite", Weight: 6},
		},
	}
	g, err := NewModelGroup(cfg, map[string]*Channel{"sense": ch})
	assert.NoError(t, err)
	assert.Len(t, g.Members, 2)
	assert.Equal(t, "glm-5.2", g.Members[0].Config.Model)
	assert.Equal(t, 5, g.Members[0].Config.Weight, "first occurrence wins")
	assert.Equal(t, "flash-lite", g.Members[1].Config.Model)
}

// --- token estimation ---

func TestEstimatePromptTokens(t *testing.T) {
	// CJK runes count 1:1, ASCII 4 bytes/token, plus requested max_tokens.
	body := []byte(`{"model":"m","messages":[{"role":"user","content":"你好世界 hello world"}],"max_tokens":100}`)
	assert.Equal(t, 107, estimatePromptTokens(body)) // 4 + 3 + 100

	// Block content: text + image_url (1024) + default 4096 output budget.
	body = []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"abc"},{"type":"image_url","image_url":{"url":"http://x"}}]}]}`)
	assert.Equal(t, 5120, estimatePromptTokens(body)) // 0 + 1024 + 4096

	// max_completion_tokens is honored as the output budget.
	body = []byte(`{"messages":[{"role":"user","content":"abcd"}],"max_completion_tokens":50}`)
	assert.Equal(t, 51, estimatePromptTokens(body))

	// Invalid JSON → 0 (caller treats as "unknown").
	assert.Equal(t, 0, estimatePromptTokens([]byte(`not json`)))
}

// --- context-aware member exclusion ---

func memberCtx(ch *Channel, model string, maxCtx int) *ModelGroupMemberRuntime {
	m := member(ch, model)
	m.Config.MaxContextTokens = maxCtx
	return m
}

func TestContextExcluded(t *testing.T) {
	ch := newTestChannel("sense", "", false, nil)
	g := newTestGroup(memberCtx(ch, "flash-lite", 262144), memberCtx(ch, "glm-5.2", 0))

	// Budget over the flash-lite window → only flash-lite excluded.
	ex := g.contextExcluded(0, 300000)
	assert.Equal(t, map[string]bool{"sense:flash-lite": true}, ex)

	// Small budget → nothing excluded.
	assert.Nil(t, g.contextExcluded(0, 1000))

	// Zero budget (unparseable) → nothing excluded.
	assert.Nil(t, g.contextExcluded(0, 0))
}

func TestContextExcluded_MemberOutputCap(t *testing.T) {
	ch := newTestChannel("sense", "", false, nil)
	flash := memberCtx(ch, "flash-lite", 262144)
	flash.Config.MaxOutputTokens = 65536
	glm := memberCtx(ch, "glm-5.2", 1048576)
	glm.Config.MaxOutputTokens = 131072
	g := newTestGroup(flash, glm)

	// 185K input + 131072 output = 316K. flash-lite's 262144 window is exceeded
	// only if we DON'T clamp its output cap: with cap 65536 the budget is
	// 250K < 262144, so flash-lite stays eligible while glm-5.2 remains too.
	ex := g.contextExcluded(185000, 131072)
	assert.Nil(t, ex)
}

func TestSelectChannel_SkipsContextExcludedMember(t *testing.T) {
	ch := newTestChannel("sense", "", false, nil)
	g := newTestGroup(memberCtx(ch, "flash-lite", 262144), member(ch, "glm-5.2"))

	// Huge budget excludes flash-lite even though it is declared first.
	chSel, model := g.SelectChannel("", TaskClassify, "", nil, g.contextExcluded(0, 300000))
	assert.Equal(t, "glm-5.2", model)
	assert.Same(t, ch, chSel)
}

// --- member-level breakdown isolation ---

func TestEligibleMembers_MemberBreakdownIsolation(t *testing.T) {
	ch := newTestChannel("sense", "", false, nil)
	glm := member(ch, "glm-5.2")
	flash := member(ch, "sensenova-6.8-flash-lite")
	g := newTestGroup(glm, flash)

	// glm's quota pool exhausted → flash-lite (same channel!) stays selectable.
	glm.RecordMemberBreakdown("quota_exhausted", time.Hour)
	eligible := g.eligibleMembers(TaskClassify, nil)
	assert.Len(t, eligible, 1)
	assert.Same(t, flash, eligible[0])

	// SelectChannel actually returns the surviving sibling.
	chSel, model := g.SelectChannel("", TaskClassify, "", nil, nil)
	assert.Equal(t, "sensenova-6.8-flash-lite", model)
	assert.Same(t, ch, chSel)

	// All members in breakdown → fallback: still selectable (never fail on it).
	flash.RecordMemberBreakdown("quota_exhausted", time.Hour)
	assert.Len(t, g.eligibleMembers(TaskClassify, nil), 2)

	// Success clears the member breakdown.
	flash.RecordMemberSuccess()
	assert.False(t, flash.memberBreakdownActive())
}

// --- end-to-end group routing against httptest upstreams ---

// newUpstreamChannel builds a Channel pointed at an httptest server.
func newUpstreamChannel(name string, url string, priority int) *Channel {
	return &Channel{
		Config: config.ChannelConfig{
			Name: name, BaseURL: url, ProviderType: "openai", Priority: priority,
		},
		Bucket: NewTokenBucket(1000, 0, 1000),
		Client: httpclient.NewClientWithTimeout(5 * time.Second),
		Health: NewChannelHealth(config.CircuitBreakerConfig{FailureThreshold: 5, CooldownSeconds: 30}),
	}
}

func newTestService() *LLMProxyService {
	return &LLMProxyService{
		cfg: config.LLMProxyConfig{MaxRetries: 0, MaxWaitSeconds: 1, DefaultBucketRPM: 1000},
	}
}

// dispatchUpstream returns a server that routes on the "model" field of the
// request body, mirroring one physical channel serving several models.
func dispatchUpstream(t *testing.T, handlers map[string]http.HandlerFunc) (*httptest.Server, map[string]*int32) {
	t.Helper()
	hits := map[string]*int32{}
	for m := range handlers {
		hits[m] = new(int32)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		model := extractModelFromBody(readBody(t, r))
		h, ok := handlers[model]
		if !ok {
			w.WriteHeader(404)
			fmt.Fprintf(w, `{"error":"no handler for model %s"}`, model)
			return
		}
		if c, ok := hits[model]; ok {
			atomic.AddInt32(c, 1)
		}
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, hits
}

func readBody(t *testing.T, r *http.Request) []byte {
	t.Helper()
	buf := make([]byte, 0)
	chunk := make([]byte, 4096)
	for {
		n, err := r.Body.Read(chunk)
		buf = append(buf, chunk[:n]...)
		if err != nil {
			break
		}
	}
	return buf
}

func okHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(200)
	w.Write([]byte(`{"id":"x","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)) //nolint:errcheck
}

// A 400 context-length response from the top member must fail over to the next
// member instead of being passed through to the caller.
func TestProxyViaGroup_FailoverOn4xxContextTooLong(t *testing.T) {
	srv, hits := dispatchUpstream(t, map[string]http.HandlerFunc{
		"flash-lite": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":{"message":"This model's maximum context length is 262144 tokens. However, your prompt contains 472970 input tokens","type":"invalid_request_error"}}`)) //nolint:errcheck
		},
		"glm-5.2": okHandler,
	})
	ch := newUpstreamChannel("sense", srv.URL, 1)
	flash := member(ch, "flash-lite")
	glm := member(ch, "glm-5.2")
	g := newTestGroup(flash, glm)

	// Small budget → flash-lite selected first (proactive filter passes).
	status, body, _, err := newTestService().proxyViaGroup(g, "", "POST", "/v1/chat/completions",
		http.Header{}, []byte(`{"model":"pool","messages":[{"role":"user","content":"hi"}]}`), "t", 0, TaskClassify)

	assert.NoError(t, err)
	assert.Equal(t, 200, status)
	assert.Contains(t, string(body), "ok")
	assert.Equal(t, int32(1), atomic.LoadInt32(hits["flash-lite"]))
	assert.Equal(t, int32(1), atomic.LoadInt32(hits["glm-5.2"]))
	// context_too_long is not a fault: channel health untouched.
	assert.Equal(t, 1.0, ch.Health.HealthScore())
}

// The proactive context filter must keep an oversized prompt away from the
// small-window member entirely (no wasted upstream call).
func TestProxyViaGroup_ContextAwarePreFilter(t *testing.T) {
	srv, hits := dispatchUpstream(t, map[string]http.HandlerFunc{
		"flash-lite": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":{"message":"maximum context length"}}`)) //nolint:errcheck
		},
		"glm-5.2": okHandler,
	})
	ch := newUpstreamChannel("sense", srv.URL, 1)
	flash := memberCtx(ch, "flash-lite", 262144)
	glm := member(ch, "glm-5.2")
	g := newTestGroup(flash, glm)

	// max_tokens 300000 pushes the estimated budget over flash-lite's window.
	status, _, _, err := newTestService().proxyViaGroup(g, "", "POST", "/v1/chat/completions",
		http.Header{}, []byte(`{"model":"pool","messages":[{"role":"user","content":"hi"}],"max_tokens":300000}`), "t", 0, TaskClassify)

	assert.NoError(t, err)
	assert.Equal(t, 200, status)
	assert.Equal(t, int32(0), atomic.LoadInt32(hits["flash-lite"]), "oversized prompt must never reach the small-window member")
	assert.Equal(t, int32(1), atomic.LoadInt32(hits["glm-5.2"]))
}

// A member with a declared max_output_tokens must have the caller's over-large
// max_tokens clamped before forwarding, so a low-cap model (flash-lite) serves
// the request instead of rejecting it.
func TestProxyViaGroup_CapsOutputTokens(t *testing.T) {
	var gotMaxTokens int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model     string `json:"model"`
			MaxTokens int    `json:"max_tokens"`
		}
		_ = json.Unmarshal(readBody(t, r), &req)
		gotMaxTokens = req.MaxTokens
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"x","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)) //nolint:errcheck
	}))
	t.Cleanup(srv.Close)

	ch := newUpstreamChannel("sense", srv.URL, 1)
	flash := member(ch, "flash-lite")
	flash.Config.MaxContextTokens = 262144
	flash.Config.MaxOutputTokens = 65536
	g := newTestGroup(flash)

	// Caller requests 131072; flash-lite caps at 65536 and must serve it.
	status, _, _, err := newTestService().proxyViaGroup(g, "", "POST", "/v1/chat/completions",
		http.Header{}, []byte(`{"model":"pool","messages":[{"role":"user","content":"hi"}],"max_tokens":131072}`), "t", 0, TaskClassify)

	assert.NoError(t, err)
	assert.Equal(t, 200, status)
	assert.Equal(t, 65536, gotMaxTokens, "flash-lite must receive a capped max_tokens")
}

// Per-model quota exhaustion must only take that member out of rotation — the
// sibling model on the same channel keeps serving.
func TestProxyViaGroup_MemberScopedQuotaIsolation(t *testing.T) {
	srv, hits := dispatchUpstream(t, map[string]http.HandlerFunc{
		"glm-5.2": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(429)
			w.Write([]byte(`{"error":{"message":"Insufficient balance / quota exhausted"}}`)) //nolint:errcheck
		},
		"flash-lite": okHandler,
	})
	ch := newUpstreamChannel("sense", srv.URL, 1)
	glm := member(ch, "glm-5.2")
	flash := member(ch, "flash-lite")
	g := newTestGroup(glm, flash)

	status, _, _, err := newTestService().proxyViaGroup(g, "", "POST", "/v1/chat/completions",
		http.Header{}, []byte(`{"model":"pool","messages":[{"role":"user","content":"hi"}]}`), "t", 0, TaskClassify)

	assert.NoError(t, err)
	assert.Equal(t, 200, status, "flash-lite must serve the request after glm's quota 429")
	// The httpclient transport layer retries a retryable 429 (default MaxRetries
	// 3), so glm may be hit several times before the failure surfaces — what
	// matters is that the error body SURVIVES those retries (regression: the
	// drained-body bug made the classifier see "" and misread quota as generic
	// rate limiting).
	assert.GreaterOrEqual(t, atomic.LoadInt32(hits["glm-5.2"]), int32(1))
	assert.Equal(t, int32(1), atomic.LoadInt32(hits["flash-lite"]))
	// Member-scoped: glm member is in cooldown, but the shared channel took no
	// circuit hit (consecutive_fails still 0 via BreakdownInfo-less check).
	class, until := glm.MemberBreakdownInfo()
	assert.Equal(t, "quota_exhausted", class)
	assert.True(t, until.After(time.Now()))
	assert.Equal(t, 1.0, ch.Health.HealthScore(), "quota 429 must not count against the shared channel")
}

// When every member fails, the last real upstream error is passed through —
// not a synthetic 503 that hides the cause.
func TestProxyViaGroup_AllFailReturnsLastUpstreamError(t *testing.T) {
	srv, _ := dispatchUpstream(t, map[string]http.HandlerFunc{
		"flash-lite": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":{"message":"bad request flash"}}`)) //nolint:errcheck
		},
		"glm-5.2": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(400)
			w.Write([]byte(`{"error":{"message":"bad request glm"}}`)) //nolint:errcheck
		},
	})
	ch := newUpstreamChannel("sense", srv.URL, 1)
	g := newTestGroup(member(ch, "flash-lite"), member(ch, "glm-5.2"))

	status, body, _, err := newTestService().proxyViaGroup(g, "", "POST", "/v1/chat/completions",
		http.Header{}, []byte(`{"model":"pool","messages":[{"role":"user","content":"hi"}]}`), "t", 0, TaskClassify)

	assert.NoError(t, err)
	assert.Equal(t, 400, status)
	assert.Contains(t, string(body), "bad request glm", "caller sees the last member's real error")
}
