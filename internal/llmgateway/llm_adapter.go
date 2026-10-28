package llmgateway

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/singll/bellkeeper/internal/llmgateway/converter"
	llmerrors "github.com/singll/bellkeeper/internal/llmgateway/errors"
)

// PreparedRequest is the result of protocol/vendor normalisation: the body and
// path actually sent upstream, the real (suffix-stripped) model, and any
// vendor-specific session identifier needed by ApplyAuth.
type PreparedRequest struct {
	Body      []byte
	Path      string
	Model     string
	SessionID string
}

// UpstreamAdapter isolates everything protocol-specific: request conversion,
// auth headers, buffered response conversion, streaming normalisation, error
// classification and the recovery probe shape. Both the buffered and streaming
// proxy paths go through the same adapter, so a capability cannot accidentally
// exist in only one of them.
type UpstreamAdapter interface {
	Protocol() string
	PrepareRequest(body []byte, path, model string, ch *Channel) (PreparedRequest, error)
	ApplyAuth(req *http.Request, ch *Channel, prep PreparedRequest)
	DecodeBuffered(respBody []byte, statusCode int, model string) ([]byte, Usage, error)
	WrapStream(rc io.ReadCloser) io.ReadCloser
	ErrorClass(statusCode int, body []byte, ch *Channel) llmerrors.Result
	ProbeRequest(model string) (body []byte, path string)
}

func adapterFor(ch *Channel) (UpstreamAdapter, error) {
	switch ch.Config.ProviderType {
	case "", "openai":
		return openAIAdapter{}, nil
	case "anthropic":
		return anthropicAdapter{}, nil
	case "rerank":
		return rerankAdapter{}, nil
	default:
		return nil, fmt.Errorf("unsupported provider_type %q", ch.Config.ProviderType)
	}
}

func classifyVendorError(statusCode int, body []byte, ch *Channel) llmerrors.Result {
	return llmerrors.Classify(statusCode, string(body), errorVendor(ch))
}

// classifyChannelError routes error classification through the channel's
// protocol adapter while always keying vendor-specific semantics on the vendor
// (never on provider_type).
func classifyChannelError(ch *Channel, statusCode int, body []byte) llmerrors.Result {
	adapter, err := adapterFor(ch)
	if err != nil {
		return classifyVendorError(statusCode, body, ch)
	}
	return adapter.ErrorClass(statusCode, body, ch)
}

// --- OpenAI-compatible ---

type openAIAdapter struct{}

func (openAIAdapter) Protocol() string { return "openai" }

func (openAIAdapter) PrepareRequest(body []byte, path, model string, ch *Channel) (PreparedRequest, error) {
	forwardBody, realModel := parseModelSuffixes(body)
	if realModel == "" {
		realModel = model
	}
	if isDeepSeekOfficial(ch) {
		forwardBody = stripReasoningForDeepSeek(forwardBody)
	}
	prep := PreparedRequest{Body: forwardBody, Path: path, Model: realModel}
	if isOpenCodeGo(ch) {
		prep.SessionID = HashCachedPrefix(body)
	}
	return prep, nil
}

func (openAIAdapter) ApplyAuth(req *http.Request, ch *Channel, prep PreparedRequest) {
	req.Header.Set("Authorization", "Bearer "+ch.Config.APIKey)
	req.Header.Set("Content-Type", "application/json")
	if prep.SessionID != "" {
		req.Header.Set("x-opencode-session", prep.SessionID)
	}
}

func (openAIAdapter) DecodeBuffered(respBody []byte, _ int, _ string) ([]byte, Usage, error) {
	prompt, comp, cached := extractTokenUsage(respBody)
	return respBody, Usage{PromptTokens: prompt, CompletionTokens: comp, CachedTokens: cached}, nil
}

func (openAIAdapter) WrapStream(rc io.ReadCloser) io.ReadCloser { return rc }

func (openAIAdapter) ErrorClass(statusCode int, body []byte, ch *Channel) llmerrors.Result {
	return classifyVendorError(statusCode, body, ch)
}

func (openAIAdapter) ProbeRequest(model string) ([]byte, string) {
	body := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"ping"}],"max_tokens":1}`, model)
	return []byte(body), "/v1/chat/completions"
}

// --- Anthropic ---

type anthropicAdapter struct{}

func (anthropicAdapter) Protocol() string { return "anthropic" }

func (anthropicAdapter) PrepareRequest(body []byte, path, model string, _ *Channel) (PreparedRequest, error) {
	converted, err := converter.ConvertOpenAIToAnthropic(body)
	if err != nil {
		return PreparedRequest{}, fmt.Errorf("anthropic request conversion: %w", err)
	}
	if path == "/v1/chat/completions" {
		path = "/v1/messages"
	}
	return PreparedRequest{Body: converted, Path: path, Model: model}, nil
}

func (anthropicAdapter) ApplyAuth(req *http.Request, ch *Channel, _ PreparedRequest) {
	req.Header.Set("x-api-key", ch.Config.APIKey)
	req.Header.Set("anthropic-version", converter.AnthropicVersion)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Del("Authorization")
}

func (anthropicAdapter) DecodeBuffered(respBody []byte, statusCode int, _ string) ([]byte, Usage, error) {
	// Read usage from the RAW body before conversion: the OpenAI conversion drops
	// cache_read_input_tokens, which would make Claude prompt-cache discounts
	// invisible to billing.
	prompt, comp, cached := extractTokenUsage(respBody)
	usage := Usage{PromptTokens: prompt, CompletionTokens: comp, CachedTokens: cached}
	if statusCode >= 500 {
		return respBody, usage, nil
	}
	converted, err := converter.ConvertAnthropicToOpenAI(respBody)
	if err != nil {
		return respBody, usage, fmt.Errorf("anthropic response conversion: %w", err)
	}
	return converted, usage, nil
}

func (anthropicAdapter) WrapStream(rc io.ReadCloser) io.ReadCloser {
	return &anthropicSSEReader{
		rc:   rc,
		br:   bufio.NewReader(rc),
		conv: converter.NewAnthropicSSEConverter(),
	}
}

func (anthropicAdapter) ErrorClass(statusCode int, body []byte, ch *Channel) llmerrors.Result {
	return classifyVendorError(statusCode, body, ch)
}

func (anthropicAdapter) ProbeRequest(model string) ([]byte, string) {
	body := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"ping"}],"max_tokens":1}`, model)
	return []byte(body), "/v1/chat/completions"
}

// anthropicSSEReader converts Anthropic SSE events to OpenAI SSE on Read, so the
// caller (HTTP handler, Gateway.ChatStream, ...) always consumes OpenAI chunks.
type anthropicSSEReader struct {
	rc        io.ReadCloser
	br        *bufio.Reader
	conv      *converter.AnthropicSSEConverter
	eventType string
	pending   []byte
	err       error
}

func (r *anthropicSSEReader) Read(p []byte) (int, error) {
	for len(r.pending) == 0 {
		if r.err != nil {
			return 0, r.err
		}
		line, err := r.br.ReadString('\n')
		if len(line) > 0 {
			r.consume(line)
		}
		if err != nil {
			r.err = err
		}
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

func (r *anthropicSSEReader) Close() error { return r.rc.Close() }

func (r *anthropicSSEReader) consume(line string) {
	line = strings.TrimRight(line, "\r\n")
	if strings.HasPrefix(line, "event:") {
		r.eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		return
	}
	if !strings.HasPrefix(line, "data:") {
		return
	}
	data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	out := r.conv.ConvertEvent(r.eventType, data)
	r.eventType = ""
	if out != "" {
		r.pending = append(r.pending, out...)
	}
}

// --- Rerank ---

type rerankAdapter struct{}

func (rerankAdapter) Protocol() string { return "rerank" }

func (rerankAdapter) PrepareRequest(body []byte, path, model string, _ *Channel) (PreparedRequest, error) {
	return PreparedRequest{Body: body, Path: path, Model: model}, nil
}

func (rerankAdapter) ApplyAuth(req *http.Request, ch *Channel, _ PreparedRequest) {
	req.Header.Set("Authorization", "Bearer "+ch.Config.APIKey)
	req.Header.Set("Content-Type", "application/json")
}

func (rerankAdapter) DecodeBuffered(respBody []byte, _ int, _ string) ([]byte, Usage, error) {
	prompt, comp, cached := extractTokenUsage(respBody)
	return respBody, Usage{PromptTokens: prompt, CompletionTokens: comp, CachedTokens: cached}, nil
}

func (rerankAdapter) WrapStream(rc io.ReadCloser) io.ReadCloser { return rc }

func (rerankAdapter) ErrorClass(statusCode int, body []byte, ch *Channel) llmerrors.Result {
	return classifyVendorError(statusCode, body, ch)
}

func (rerankAdapter) ProbeRequest(model string) ([]byte, string) {
	body := fmt.Sprintf(`{"model":%q,"query":"ping","documents":["ping"]}`, model)
	return []byte(body), "/v1/rerank"
}

// --- Vendor / body helpers shared by adapters ---

// parseModelSuffixes handles reasoning effort (-high/-medium/-low) and thinking mode (-thinking-N) suffixes.
// Returns modified request body and the real model name to send upstream.
func parseModelSuffixes(body []byte) ([]byte, string) {
	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return body, ""
	}
	originalModel := req.Model
	realModel := originalModel

	// Reasoning effort suffixes: o4-mini-high → o4-mini + reasoning.effort=high
	if strings.HasSuffix(originalModel, "-high") {
		realModel = strings.TrimSuffix(originalModel, "-high")
		body = injectField(body, "reasoning", map[string]string{"effort": "high"})
	} else if strings.HasSuffix(originalModel, "-medium") {
		realModel = strings.TrimSuffix(originalModel, "-medium")
		body = injectField(body, "reasoning", map[string]string{"effort": "medium"})
	} else if strings.HasSuffix(originalModel, "-low") {
		realModel = strings.TrimSuffix(originalModel, "-low")
		body = injectField(body, "reasoning", map[string]string{"effort": "low"})
	}

	// Thinking mode suffix: claude-sonnet-thinking-2048 → thinking.budget_tokens=2048
	if idx := strings.LastIndex(realModel, "-thinking-"); idx > 0 {
		budgetStr := realModel[idx+len("-thinking-"):]
		if budget, err := strconv.Atoi(budgetStr); err == nil {
			realModel = realModel[:idx]
			body = injectField(body, "thinking", map[string]int{"budget_tokens": budget})
		}
	}

	// Replace model name in body
	if realModel != originalModel {
		body = replaceModelInBody(body, realModel)
	}
	return body, realModel
}

func injectField(body []byte, key string, value interface{}) []byte {
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return body
	}
	m[key] = value
	b, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return b
}

func replaceModelInBody(body []byte, model string) []byte {
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return body
	}
	m["model"] = model
	b, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return b
}

// isDeepSeekOfficial reports whether the channel forwards to DeepSeek's
// official API (api.deepseek.com). Its thinking mode requires reasoning_content
// to be passed back on assistant messages, which the gateway path does not
// reliably forward (the DSH gateway provider serializes reasoning under the
// upstream channel's own field name, e.g. SenseNova's "reasoning").
func isDeepSeekOfficial(ch *Channel) bool {
	if ch.Config.Vendor != "" {
		return ch.Config.Vendor == "deepseek"
	}
	return strings.Contains(ch.Config.BaseURL, "deepseek.com")
}

// isOpenCodeGo reports whether the channel forwards to OpenCode Go
// (opencode.ai/zen/go). Its API requires a stable x-opencode-session header per
// conversation for routing and prompt-cache optimization.
func isOpenCodeGo(ch *Channel) bool {
	if ch.Config.Vendor != "" {
		return ch.Config.Vendor == "opencodego"
	}
	return strings.Contains(ch.Config.BaseURL, "opencode.ai")
}

// stripReasoningForDeepSeek disables thinking mode before forwarding to
// DeepSeek official. DeepSeek returns 400 "reasoning_content in the thinking
// mode must be passed back" when a request carries reasoning_effort but the
// conversation's assistant messages lack DeepSeek's reasoning_content field.
// Dropping reasoning_effort and any carried reasoning fields turns the official
// channel into a plain non-thinking fallback, which is the intended last-resort
// role of deepseek-secagent in pool-secagent.
func stripReasoningForDeepSeek(body []byte) []byte {
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return body
	}
	changed := false
	if _, ok := m["reasoning_effort"]; ok {
		delete(m, "reasoning_effort")
		changed = true
	}
	if msgs, ok := m["messages"].([]interface{}); ok {
		for _, mi := range msgs {
			msg, ok := mi.(map[string]interface{})
			if !ok {
				continue
			}
			if role, _ := msg["role"].(string); role != "assistant" {
				continue
			}
			for _, k := range []string{"reasoning_content", "reasoning"} {
				if _, ok := msg[k]; ok {
					delete(msg, k)
					changed = true
				}
			}
		}
	}
	if !changed {
		return body
	}
	b, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return b
}
