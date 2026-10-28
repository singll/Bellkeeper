package llmgateway

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/singll/bellkeeper/internal/config"
	llmerrors "github.com/singll/bellkeeper/internal/llmgateway/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdapterPrepareRequestAndAuth(t *testing.T) {
	tests := []struct {
		name       string
		ch         *Channel
		body       string
		path       string
		wantProto  string
		wantPath   string
		wantModel  string
		wantHeader map[string]string
		absent     []string
		checkBody  func(t *testing.T, body []byte)
	}{
		{
			name: "openai deepseek strips reasoning",
			ch: &Channel{Config: config.ChannelConfig{
				Name: "deepseek", ProviderType: "openai", Vendor: "deepseek",
				BaseURL: "https://api.deepseek.com", APIKey: "sk-ds",
			}},
			body:      `{"model":"deepseek-v4-flash-high","reasoning_effort":"high","messages":[{"role":"assistant","content":"a","reasoning_content":"r"}]}`,
			path:      "/v1/chat/completions",
			wantProto: "openai",
			wantPath:  "/v1/chat/completions",
			wantModel: "deepseek-v4-flash",
			wantHeader: map[string]string{
				"Authorization": "Bearer sk-ds",
				"Content-Type":  "application/json",
			},
			checkBody: func(t *testing.T, body []byte) {
				var m map[string]interface{}
				require.NoError(t, json.Unmarshal(body, &m))
				assert.NotContains(t, m, "reasoning_effort")
				assert.Equal(t, map[string]interface{}{"effort": "high"}, m["reasoning"])
				msgs := m["messages"].([]interface{})
				assert.NotContains(t, msgs[0].(map[string]interface{}), "reasoning_content")
			},
		},
		{
			name: "openai opencodego adds session header",
			ch: &Channel{Config: config.ChannelConfig{
				Name: "ocgo", ProviderType: "openai", Vendor: "opencodego",
				BaseURL: "https://opencode.ai/zen/go", APIKey: "sk-oc",
			}},
			body:      `{"model":"gpt-5","messages":[{"role":"user","content":"hi"}]}`,
			path:      "/v1/chat/completions",
			wantProto: "openai",
			wantPath:  "/v1/chat/completions",
			wantModel: "gpt-5",
			wantHeader: map[string]string{
				"Authorization":      "Bearer sk-oc",
				"x-opencode-session": HashCachedPrefix([]byte(`{"model":"gpt-5","messages":[{"role":"user","content":"hi"}]}`)),
			},
		},
		{
			name: "anthropic converts request and auth headers",
			ch: &Channel{Config: config.ChannelConfig{
				Name: "claude", ProviderType: "anthropic", Vendor: "anthropic",
				BaseURL: "https://api.anthropic.com", APIKey: "sk-ant",
			}},
			body:      `{"model":"claude-sonnet-4-6","messages":[{"role":"system","content":"sys"},{"role":"user","content":"hi"}],"max_tokens":64}`,
			path:      "/v1/chat/completions",
			wantProto: "anthropic",
			wantPath:  "/v1/messages",
			wantModel: "claude-sonnet-4-6",
			wantHeader: map[string]string{
				"x-api-key":         "sk-ant",
				"anthropic-version": "2023-06-01",
				"Content-Type":      "application/json",
			},
			absent: []string{"Authorization"},
			checkBody: func(t *testing.T, body []byte) {
				var m map[string]interface{}
				require.NoError(t, json.Unmarshal(body, &m))
				assert.Equal(t, "sys", m["system"])
				assert.Equal(t, float64(64), m["max_tokens"])
				msgs := m["messages"].([]interface{})
				assert.Len(t, msgs, 1, "system message moves to top-level system")
			},
		},
		{
			name: "rerank forwards body verbatim",
			ch: &Channel{Config: config.ChannelConfig{
				Name: "reranker", ProviderType: "rerank", Vendor: "siliconflow",
				BaseURL: "https://api.siliconflow.cn", APIKey: "sk-rr",
			}},
			body:      `{"model":"bge-reranker-v2-m3","query":"q","documents":["a","b"]}`,
			path:      "/v1/rerank",
			wantProto: "rerank",
			wantPath:  "/v1/rerank",
			wantModel: "bge-reranker-v2-m3",
			wantHeader: map[string]string{
				"Authorization": "Bearer sk-rr",
				"Content-Type":  "application/json",
			},
			checkBody: func(t *testing.T, body []byte) {
				assert.JSONEq(t, `{"model":"bge-reranker-v2-m3","query":"q","documents":["a","b"]}`, string(body))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := adapterFor(tt.ch)
			require.NoError(t, err)
			assert.Equal(t, tt.wantProto, adapter.Protocol())

			prep, err := adapter.PrepareRequest([]byte(tt.body), tt.path, extractModelFromBody([]byte(tt.body)), tt.ch)
			require.NoError(t, err)
			assert.Equal(t, tt.wantPath, prep.Path)
			assert.Equal(t, tt.wantModel, prep.Model)

			req := httptest.NewRequest(http.MethodPost, "http://upstream"+prep.Path, strings.NewReader(string(prep.Body)))
			adapter.ApplyAuth(req, tt.ch, prep)
			for k, want := range tt.wantHeader {
				assert.Equal(t, want, req.Header.Get(k), k)
			}
			for _, k := range tt.absent {
				assert.Empty(t, req.Header.Get(k), k)
			}
			if tt.checkBody != nil {
				tt.checkBody(t, prep.Body)
			}
		})
	}
}

func TestAdapterErrorClass(t *testing.T) {
	tests := []struct {
		name       string
		protocol   string
		vendor     string
		statusCode int
		body       string
		want       llmerrors.Class
	}{
		{
			name:       "openai context too long",
			protocol:   "openai",
			statusCode: 400,
			body:       `{"error":{"message":"This model's maximum context length is 100 tokens"}}`,
			want:       llmerrors.ContextTooLong,
		},
		{
			name:       "anthropic protocol with kimi vendor quota",
			protocol:   "anthropic",
			vendor:     "kimi-code",
			statusCode: 403,
			body:       `{"error":{"message":"usage limit reached"}}`,
			want:       llmerrors.QuotaExhausted,
		},
		{
			name:       "deepseek insufficient balance",
			protocol:   "openai",
			vendor:     "deepseek",
			statusCode: 403,
			body:       `{"error":{"message":"Insufficient Balance"}}`,
			want:       llmerrors.BalanceZero,
		},
		{
			name:       "rerank transient rate limit",
			protocol:   "rerank",
			statusCode: 429,
			body:       `{"error":"rate limit exceeded"}`,
			want:       llmerrors.RateLimitedRetry,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := &Channel{Config: config.ChannelConfig{ProviderType: tt.protocol, Vendor: tt.vendor}}
			adapter, err := adapterFor(ch)
			require.NoError(t, err)
			assert.Equal(t, tt.want, adapter.ErrorClass(tt.statusCode, []byte(tt.body), ch).Class)
			assert.Equal(t, tt.want, classifyChannelError(ch, tt.statusCode, []byte(tt.body)).Class)
		})
	}
}

func TestErrorVendorDoesNotUseProtocol(t *testing.T) {
	anthropicProtocol := &Channel{Config: config.ChannelConfig{ProviderType: "anthropic"}}
	assert.Empty(t, errorVendor(anthropicProtocol), "protocol must not impersonate a vendor")
	assert.Empty(t, errorVendor(&Channel{Config: config.ChannelConfig{
		ProviderType: "openai", BaseURL: "https://unknown.example.com",
	}}))

	explicit := &Channel{Config: config.ChannelConfig{ProviderType: "anthropic", Vendor: "kimi-code"}}
	assert.Equal(t, "kimi-code", errorVendor(explicit))

	inferred := &Channel{Config: config.ChannelConfig{ProviderType: "openai", BaseURL: "https://api.deepseek.com"}}
	assert.Equal(t, "deepseek", errorVendor(inferred))
}

func TestOpenAIAdapterDecodeBufferedUsage(t *testing.T) {
	body := []byte(`{"id":"x","choices":[],"usage":{"prompt_tokens":12,"completion_tokens":7,"prompt_tokens_details":{"cached_tokens":3}}}`)

	out, usage, err := openAIAdapter{}.DecodeBuffered(body, 200, "m")
	require.NoError(t, err)
	assert.Equal(t, body, out)
	assert.Equal(t, Usage{PromptTokens: 12, CompletionTokens: 7, CachedTokens: 3}, usage)
}

func TestAnthropicAdapterDecodeBuffered(t *testing.T) {
	raw := []byte(`{"id":"msg_1","model":"claude-sonnet-4-6","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":3}}`)

	out, usage, err := anthropicAdapter{}.DecodeBuffered(raw, 200, "claude")
	require.NoError(t, err)
	assert.Equal(t, Usage{PromptTokens: 13, CompletionTokens: 5, CachedTokens: 3}, usage)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(out, &resp))
	assert.Equal(t, "chat.completion", resp["object"])
	choice := resp["choices"].([]interface{})[0].(map[string]interface{})
	assert.Equal(t, "hi", choice["message"].(map[string]interface{})["content"])

	errBody := []byte(`{"type":"error","error":{"type":"invalid_request_error","message":"bad request"}}`)
	out, _, err = anthropicAdapter{}.DecodeBuffered(errBody, 400, "claude")
	require.NoError(t, err)
	var openaiErr map[string]interface{}
	require.NoError(t, json.Unmarshal(out, &openaiErr))
	assert.Equal(t, "bad request", openaiErr["error"].(map[string]interface{})["message"])
}

func TestAnthropicAdapterWrapStream(t *testing.T) {
	raw := "event: message_start\n" +
		"data: {\"message\":{\"id\":\"msg_1\",\"model\":\"claude\"},\"usage\":{\"input_tokens\":5,\"cache_read_input_tokens\":2}}\n\n" +
		"event: content_block_delta\n" +
		"data: {\"delta\":{\"type\":\"text_delta\",\"text\":\"hi\"}}\n\n" +
		"event: message_delta\n" +
		"data: {\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":9}}\n\n" +
		"event: message_stop\n" +
		"data: {}\n\n"

	tracker := newStreamUsageTracker(io.NopCloser(strings.NewReader(raw)))
	out, err := io.ReadAll(anthropicAdapter{}.WrapStream(tracker))
	require.NoError(t, err)

	assert.Contains(t, string(out), `"content":"hi"`)
	assert.Contains(t, string(out), `"finish_reason":"stop"`)
	assert.Contains(t, string(out), "data: [DONE]")

	prompt, comp, cached := tracker.Usage()
	assert.Equal(t, 7, prompt, "tracker must observe raw Anthropic usage events")
	assert.Equal(t, 9, comp)
	assert.Equal(t, 2, cached)
}
