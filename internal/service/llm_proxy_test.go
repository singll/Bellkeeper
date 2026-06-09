package service

import (
	"net/http"
	"testing"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	llmerrors "github.com/singll/bellkeeper/internal/llm/errors"
	"github.com/singll/bellkeeper/internal/model"
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

func TestComputeMicroCents(t *testing.T) {
	// DeepSeek V3: 14¢/1M input, 28¢/1M output. 1000 prompt tokens of input alone
	// is 0.014¢ — old integer-cent math truncated this to 0 (audit #13). Micro-cents
	// must capture it as 14 micro-cents.
	deepseek := &model.LLMModelPricing{InputPricePer1MCents: 14, OutputPricePer1MCents: 28}
	got := computeMicroCents(deepseek, Usage{PromptTokens: 1000})
	assert.Equal(t, int64(14), got, "1000 deepseek input tokens should cost 14 micro-cents, not 0")

	// input + output
	got = computeMicroCents(deepseek, Usage{PromptTokens: 1000, CompletionTokens: 1000})
	assert.Equal(t, int64(14+28), got)

	// #14: cached default price = input/10. With input=14, cached rate = 1.4¢/1M.
	// 1000 cached tokens = 1.4 micro-cents → 1 (floor of 1.4) — NOT 0. The old
	// GetCachedInputPrice()=input/10=1 then *tokens/1M floored the rate first.
	got = computeMicroCents(deepseek, Usage{PromptTokens: 1000, CachedTokens: 1000})
	assert.Equal(t, int64(1), got, "1000 cached tokens at input/10 should be 1 micro-cent, not 0")

	// Explicit cached price overrides the default.
	withCached := &model.LLMModelPricing{InputPricePer1MCents: 300, OutputPricePer1MCents: 1500, CachedInputPricePer1MCents: 30}
	got = computeMicroCents(withCached, Usage{PromptTokens: 10000, CachedTokens: 8000, CompletionTokens: 500})
	// uncached input: 2000*300/1000=600; cached: 8000*30/1000=240; output: 500*1500/1000=750
	assert.Equal(t, int64(600+240+750), got)

	// Rounding to display cents (half up): 1499 micro-cents → 1¢, 1500 → 2¢.
	assert.Equal(t, int64(1), MicroCentsToCents(1499))
	assert.Equal(t, int64(2), MicroCentsToCents(1500))
	assert.Equal(t, int64(0), MicroCentsToCents(14))
}

func TestImplicitConversationID(t *testing.T) {
	m := NewConversationBindingManager(nil, 0)

	// Explicit header always wins.
	got := m.ImplicitConversationID(map[string]string{"X-Conversation-ID": "abc"}, []byte(`{}`))
	assert.Equal(t, "abc", got)

	// No header + no cache_control → no implicit binding (audit #11: don't pin
	// stateless requests).
	got = m.ImplicitConversationID(nil, []byte(`{"messages":[{"role":"user","content":"hi"}]}`))
	assert.Equal(t, "", got)

	// cache_control present → stable id derived from the FIRST messages, so a
	// follow-up turn (extra trailing message) maps to the SAME id.
	turn1 := []byte(`{"messages":[{"role":"system","content":[{"type":"text","text":"sys","cache_control":{"type":"ephemeral"}}]},{"role":"user","content":"a"}]}`)
	turn2 := []byte(`{"messages":[{"role":"system","content":[{"type":"text","text":"sys","cache_control":{"type":"ephemeral"}}]},{"role":"user","content":"a"},{"role":"assistant","content":"b"},{"role":"user","content":"c"}]}`)
	id1 := m.ImplicitConversationID(nil, turn1)
	id2 := m.ImplicitConversationID(nil, turn2)
	assert.NotEmpty(t, id1)
	assert.Equal(t, id1, id2, "multi-turn requests sharing a prefix must hash to the same conversation id")
}

func newTestChannel(name, tier string, free bool, taskTypes []string) *Channel {
	return &Channel{
		Config: config.ChannelConfig{Name: name, Tier: tier, IsFree: free, TaskTypes: taskTypes},
		Bucket: NewTokenBucket(100, 0, 60),
		Health: NewChannelHealth(config.CircuitBreakerConfig{FailureThreshold: 5, CooldownSeconds: 30}),
	}
}

func newTestGroup(members ...*ModelGroupMemberRuntime) *ModelGroup {
	return &ModelGroup{
		Config:  config.ModelGroupConfig{Strategy: "priority-health"},
		Members: members,
	}
}

func member(ch *Channel, model string) *ModelGroupMemberRuntime {
	return &ModelGroupMemberRuntime{Config: config.ModelGroupMember{Channel: ch.Config.Name, Model: model, Weight: 1}, Channel: ch}
}

func TestSelectChannel_CodingTierOrdering(t *testing.T) {
	freeCh := newTestChannel("free", "free", true, nil)
	kimiCh := newTestChannel("kimi", "standard", false, []string{"coding"}) // coding-only
	paidCh := newTestChannel("paid", "premium", false, nil)
	g := newTestGroup(member(paidCh, "paid-m"), member(kimiCh, "kimi-m"), member(freeCh, "free-m"))

	// free_first → free tier wins for coding
	ch, _ := g.SelectChannel("", TaskCoding, "free_first", nil, nil)
	assert.Equal(t, "free", ch.Config.Name)

	// quality_first → standard (Kimi/sunk-cost) tier first
	ch, _ = g.SelectChannel("", TaskCoding, "quality_first", nil, nil)
	assert.Equal(t, "kimi", ch.Config.Name)

	// complex → standard first
	ch, _ = g.SelectChannel("", TaskCoding, "complex", nil, nil)
	assert.Equal(t, "kimi", ch.Config.Name)

	// Excluding the free tier (already tried) advances to standard under free_first.
	ch, _ = g.SelectChannel("", TaskCoding, "free_first", nil, map[string]bool{"free": true})
	assert.Equal(t, "kimi", ch.Config.Name)
}

func TestSelectChannel_TaskEligibility(t *testing.T) {
	freeCh := newTestChannel("free", "free", true, nil)                     // general-purpose
	kimiCh := newTestChannel("kimi", "standard", false, []string{"coding"}) // coding-only
	g := newTestGroup(member(kimiCh, "kimi-m"), member(freeCh, "free-m"))

	// A classify task must NOT route to the coding-only kimi channel.
	ch, _ := g.SelectChannel("", TaskClassify, "", nil, nil)
	assert.Equal(t, "free", ch.Config.Name, "classify must skip coding-only members")
}

// TestChannelHealth_BreakdownInfo covers the getter the Kimi Code probe loop keys on:
// a classified quota_exhausted failure must trip the circuit open and be reported
// verbatim (class + ~5h until), and Reset must close it again.
func TestChannelHealth_BreakdownInfo(t *testing.T) {
	h := NewChannelHealth(config.CircuitBreakerConfig{FailureThreshold: 1, CooldownSeconds: 30})

	state, class, until := h.BreakdownInfo()
	assert.Equal(t, CircuitClosed, state)
	assert.Empty(t, class)
	assert.True(t, until.IsZero(), "no breakdown before any failure")

	h.RecordClassifiedFailure("403", string(llmerrors.QuotaExhausted), 5*time.Hour)
	state, class, until = h.BreakdownInfo()
	assert.Equal(t, CircuitOpen, state)
	assert.Equal(t, "quota_exhausted", class)
	assert.True(t, until.After(time.Now().Add(4*time.Hour)), "quota breakdown should last ~5h")

	h.Reset()
	state, _, _ = h.BreakdownInfo()
	assert.Equal(t, CircuitClosed, state, "probe success (Reset) must close the circuit")
}

// TestProxyRerank_RequiresRerankProvider verifies a /v1/rerank request is refused
// when the only channel serving the model is not provider_type=="rerank" — i.e. the
// rerank body is never sent to a chat channel that happens to share the model name.
func TestProxyRerank_RequiresRerankProvider(t *testing.T) {
	chatCh := newTestChannel("chat", "standard", false, nil)
	chatCh.Config.ProviderType = "openai"
	s := &LLMProxyService{modelMap: map[string][]*Channel{"bge-reranker-v2-m3": {chatCh}}}

	status, body, _, err := s.proxyRerank("bge-reranker-v2-m3", "POST", "/v1/rerank",
		http.Header{}, []byte(`{"model":"bge-reranker-v2-m3","query":"q","documents":["a"]}`), "tester", 0)

	assert.Equal(t, 400, status)
	assert.Error(t, err)
	assert.Contains(t, string(body), "no rerank channel")
}
