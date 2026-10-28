package llmgateway

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/model"
	"go.uber.org/zap"
)

// touchConversation renews a conversation binding and accumulates the tokens and
// cost of a successful buffered request. Cost is best-effort: an unpriced model
// contributes 0 rather than failing the request path.
func (s *LLMProxyService) touchConversation(convID string, ch *Channel, modelName string, respBody []byte) {
	if convID == "" || s.convMgr == nil {
		return
	}
	prompt, comp, cached := extractTokenUsage(respBody)
	s.touchConversationUsage(convID, ch.Config.Name, modelName, Usage{
		PromptTokens:     prompt,
		CompletionTokens: comp,
		CachedTokens:     cached,
	})
}

func (s *LLMProxyService) touchConversationUsage(convID, channelName, modelName string, usage Usage) {
	if convID == "" || s.convMgr == nil {
		return
	}
	costCents := 0
	if s.pricer != nil && usage.PromptTokens+usage.CompletionTokens > 0 {
		if micro, err := s.pricer.CalcMicroCents(channelName, modelName, usage); err == nil {
			costCents = int(MicroCentsToCents(micro))
		}
	}
	s.convMgr.Touch(convID, usage.PromptTokens+usage.CompletionTokens, costCents)
}

// headerToMap converts http.Header to a simple map[string]string (first value only).
func headerToMap(h http.Header) map[string]string {
	m := make(map[string]string, len(h))
	for k, v := range h {
		if len(v) > 0 {
			m[k] = v[0]
		}
	}
	return m
}
func extractTokenUsage(body []byte) (prompt, comp, cached int) {
	// Try OpenAI format first
	var openaiResp struct {
		Usage struct {
			PromptTokens        int `json:"prompt_tokens"`
			CompletionTokens    int `json:"completion_tokens"`
			CachedTokens        int `json:"cached_tokens,omitempty"`
			PromptTokensDetails struct {
				CachedTokens int `json:"cached_tokens"`
			} `json:"prompt_tokens_details,omitempty"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &openaiResp); err == nil &&
		(openaiResp.Usage.PromptTokens > 0 || openaiResp.Usage.CompletionTokens > 0 ||
			openaiResp.Usage.CachedTokens > 0 || openaiResp.Usage.PromptTokensDetails.CachedTokens > 0) {
		cached := openaiResp.Usage.CachedTokens
		if cached == 0 {
			cached = openaiResp.Usage.PromptTokensDetails.CachedTokens
		}
		return openaiResp.Usage.PromptTokens, openaiResp.Usage.CompletionTokens, cached
	}

	// Try Anthropic format (raw, pre-conversion). input_tokens EXCLUDES cached reads,
	// so total prompt = input + cache_read + cache_creation; cached = cache_read.
	var anthropicResp struct {
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &anthropicResp); err == nil &&
		(anthropicResp.Usage.InputTokens > 0 || anthropicResp.Usage.OutputTokens > 0 ||
			anthropicResp.Usage.CacheReadInputTokens > 0 || anthropicResp.Usage.CacheCreationInputTokens > 0) {
		cached := anthropicResp.Usage.CacheReadInputTokens
		prompt := anthropicResp.Usage.InputTokens +
			anthropicResp.Usage.CacheReadInputTokens +
			anthropicResp.Usage.CacheCreationInputTokens
		return prompt, anthropicResp.Usage.OutputTokens, cached
	}

	return 0, 0, 0
}

type streamUsageTracker struct {
	rc      io.ReadCloser
	mu      sync.Mutex
	pending string
	prompt  int
	comp    int
	cached  int
}

func newStreamUsageTracker(rc io.ReadCloser) *streamUsageTracker {
	return &streamUsageTracker{rc: rc}
}

func (t *streamUsageTracker) Read(p []byte) (int, error) {
	n, err := t.rc.Read(p)
	if n > 0 {
		t.observe(p[:n])
	}
	return n, err
}

func (t *streamUsageTracker) Close() error {
	return t.rc.Close()
}

func (t *streamUsageTracker) Usage() (prompt, comp, cached int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.prompt, t.comp, t.cached
}

func (t *streamUsageTracker) observe(chunk []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pending += string(chunk)
	for {
		i := strings.IndexByte(t.pending, '\n')
		if i < 0 {
			if len(t.pending) > 64*1024 {
				t.pending = t.pending[len(t.pending)-64*1024:]
			}
			return
		}
		line := strings.TrimSpace(t.pending[:i])
		t.pending = t.pending[i+1:]
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		prompt, comp, cached := extractTokenUsage([]byte(data))
		if prompt > t.prompt {
			t.prompt = prompt
		}
		if comp > t.comp {
			t.comp = comp
		}
		if cached > t.cached {
			t.cached = cached
		}
	}
}

type proxyLogEntry struct {
	channelName  string
	modelName    string
	path         string
	statusCode   int
	isRateLimit  bool
	retryCount   int
	durationMs   int
	promptTokens int
	compTokens   int
	cachedTokens int
	errMsg       string
	callerID     string
	tokenID      uint
	createdAt    time.Time
}

func (s *LLMProxyService) logRequest(channelName, modelName, path string, statusCode int,
	isRateLimit bool, retryCount, durationMs, promptTokens, compTokens, cachedTokens int,
	errMsg, callerID string, tokenID uint) {
	if s.repo == nil {
		return
	}
	entry := proxyLogEntry{
		channelName:  channelName,
		modelName:    modelName,
		path:         path,
		statusCode:   statusCode,
		isRateLimit:  isRateLimit,
		retryCount:   retryCount,
		durationMs:   durationMs,
		promptTokens: promptTokens,
		compTokens:   compTokens,
		cachedTokens: cachedTokens,
		errMsg:       errMsg,
		callerID:     callerID,
		tokenID:      tokenID,
		createdAt:    time.Now(),
	}
	select {
	case s.logQueue <- entry:
	case <-s.logStopCh:
		return
	default:
		middleware.GetLogger().Warn("llm proxy log queue full; dropping log entry",
			zap.String("channel", channelName), zap.String("model", modelName), zap.Int("status", statusCode))
	}
}

func (s *LLMProxyService) startLogWorkers() {
	for i := 0; i < llmProxyLogWorkers; i++ {
		s.logWG.Add(1)
		go func() {
			defer s.logWG.Done()
			for {
				select {
				case entry := <-s.logQueue:
					s.persistProxyLog(entry)
				case <-s.logStopCh:
					for {
						select {
						case entry := <-s.logQueue:
							s.persistProxyLog(entry)
						default:
							return
						}
					}
				}
			}
		}()
	}
}

func (s *LLMProxyService) persistProxyLog(e proxyLogEntry) {
	pricingModel := e.modelName
	if i := strings.Index(e.modelName, "→"); i >= 0 {
		pricingModel = e.modelName[i+len("→"):]
	}

	var costMicroCents int64
	var costCents int
	if s.pricer != nil && e.promptTokens+e.compTokens > 0 {
		costMicroCents, _ = s.pricer.CalcMicroCents(e.channelName, pricingModel, Usage{
			PromptTokens:     e.promptTokens,
			CompletionTokens: e.compTokens,
			CachedTokens:     e.cachedTokens,
		})
		costCents = int(MicroCentsToCents(costMicroCents))
	}

	entry := &model.LLMProxyLog{
		ChannelName:    e.channelName,
		Model:          e.modelName,
		RequestPath:    e.path,
		StatusCode:     e.statusCode,
		IsRateLimit:    e.isRateLimit,
		RetryCount:     e.retryCount,
		DurationMs:     e.durationMs,
		PromptTokens:   e.promptTokens,
		CompTokens:     e.compTokens,
		CachedTokens:   e.cachedTokens,
		CostCents:      costCents,
		CostMicroCents: costMicroCents,
		ErrorMessage:   e.errMsg,
		CallerID:       e.callerID,
		CreatedAt:      e.createdAt,
	}
	if err := s.repo.CreateLog(entry); err != nil {
		middleware.GetLogger().Warn("failed to log llm proxy request", zap.Error(err))
		return
	}

	if s.tokenUsageRepo != nil && e.tokenID != 0 {
		requests := 1
		errCount := 0
		if e.statusCode >= 400 {
			errCount = 1
			requests = 0
		}
		if err := s.tokenUsageRepo.AddUsage(e.tokenID, e.createdAt.Truncate(24*time.Hour),
			requests, e.promptTokens, e.compTokens, e.cachedTokens, costCents, costMicroCents, errCount); err != nil {
			middleware.GetLogger().Warn("failed to aggregate token usage", zap.Error(err))
		}
	}
}

// logGroupRequest logs a model group proxy attempt for traceability.
// errMsg 携带失败摘要（非流式路径为 error 字符串；流式路径为截断的上游响应体——
// 2026-09-03 排查 deepseek-secagent 400 时因响应体不落库而无从定位，自此必须可见）。
func (s *LLMProxyService) logGroupRequest(channelName, virtualModel, realModel, path string,
	statusCode int, errMsg string, callerID string, tokenID uint) {
	isRateLimit := statusCode == 429
	// Log with virtual model name so operators can trace group requests
	s.logRequest(channelName, virtualModel+"→"+realModel, path, statusCode, isRateLimit,
		0, 0, 0, 0, 0, errMsg, callerID, tokenID)
}
func estimateTokens(body []byte) int {
	runes := len([]rune(string(body)))
	if runes == 0 {
		return 0
	}
	byteLen := len(body)
	avgBytesPerRune := byteLen / runes
	if avgBytesPerRune >= 3 {
		return runes * 2
	}
	return byteLen / 4
}

// estimatePromptTokens estimates the token budget a chat request needs from a
// member model: message content (CJK-aware heuristic — 1 token per CJK rune,
// 1 token per 4 bytes otherwise, ~1024 per image block) plus the requested
// output budget (max_tokens / max_completion_tokens, defaulting to 4096 when
// unset). Used for context-aware member selection; a misestimate is caught
// reactively by context_too_long failover, so it only needs to be roughly right.
func estimatePromptTokens(body []byte) int {
	return estimateInputTokens(body) + estimateOutputBudget(body)
}

// estimateOutputBudget returns the requested output token budget
// (max_tokens / max_completion_tokens), defaulting to 4096 when unset. Returns 0
// when the body is unparseable.
func estimateOutputBudget(body []byte) int {
	var req struct {
		MaxTokens           int `json:"max_tokens"`
		MaxCompletionTokens int `json:"max_completion_tokens"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return 0
	}
	out := req.MaxTokens
	if req.MaxCompletionTokens > out {
		out = req.MaxCompletionTokens
	}
	if out <= 0 {
		out = 4096
	}
	return out
}

// estimateInputTokens estimates the input (message content) token budget,
// excluding the output budget. Returns 0 when the body is unparseable.
func estimateInputTokens(body []byte) int {
	var req struct {
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return 0
	}
	total := 0
	for _, m := range req.Messages {
		total += estimateContentTokens(m.Content)
	}
	return total
}

// estimateContentTokens estimates tokens in one message content, which may be a
// plain string or an array of content blocks (text / image_url / ...).
func estimateContentTokens(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return estimateTextTokens(s)
	}
	var blocks []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL *struct {
			URL string `json:"url"`
		} `json:"image_url"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		// Unknown shape — fall back to a whole-payload estimate.
		return estimateTextTokens(string(raw))
	}
	n := 0
	for _, b := range blocks {
		if b.Text != "" {
			n += estimateTextTokens(b.Text)
		}
		if b.ImageURL != nil {
			n += 1024
		}
	}
	return n
}

// estimateTextTokens estimates tokens for a text string: CJK runes count as one
// token each (a safe upper bound for common tokenizers), everything else as
// one token per 4 bytes.
func estimateTextTokens(s string) int {
	cjk := 0
	otherBytes := 0
	for _, r := range s {
		switch {
		case r >= 0x2E80 && r <= 0x9FFF, // CJK radicals..Han
			r >= 0xF900 && r <= 0xFAFF, // CJK compat ideographs
			r >= 0xFF00 && r <= 0xFFEF, // fullwidth forms
			r >= 0x3000 && r <= 0x303F: // CJK punctuation
			cjk++
		default:
			otherBytes += len(string(r))
		}
	}
	return cjk + otherBytes/4
}
