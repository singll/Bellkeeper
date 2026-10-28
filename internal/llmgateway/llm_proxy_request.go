package llmgateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	llmerrors "github.com/singll/bellkeeper/internal/llmgateway/errors"
	"github.com/singll/bellkeeper/internal/middleware"
	"go.uber.org/zap"
)

// ProxyRequest forwards an OpenAI-compatible request through rate-limited channels.
func (s *LLMProxyService) ProxyRequest(
	method, path string,
	headers http.Header,
	body []byte,
	callerID string,
	tokenID uint,
) (statusCode int, respBody []byte, respHeaders http.Header, err error) {
	modelName := extractModelFromBody(body)

	// Rerank requests (Tier 7) route only to provider_type=="rerank" channels and
	// bypass virtual groups, sticky binding, and chat conversion — they carry no
	// chat or conversation semantics. Reuses tryChannel (circuit + bucket + logging).
	if path == "/v1/rerank" {
		return s.proxyRerank(modelName, method, path, headers, body, callerID, tokenID)
	}

	// Snapshot current routing state for lock-free proxy operation
	s.mu.RLock()
	modelGroups := s.modelGroups
	modelMap := s.modelMap
	s.mu.RUnlock()

	// Derive task key for sticky routing
	taskKey := headers.Get("X-Task-Key")
	if taskKey == "" && callerID != "" && callerID != "unknown" {
		taskKey = callerID + ":" + modelName
	}

	// --- Task type detection (drives task-aware tiered routing in proxyViaGroup) ---
	headerMap := headerToMap(headers)
	taskType := s.taskRouter.DetectTaskType(headerMap, modelName, body, estimateTokens(body))

	// --- Conversation binding (sticky session) ---
	convID := s.convMgr.ImplicitConversationID(headerMap, body)
	allowSwitch := headers.Get("X-Allow-Channel-Switch") == "true"

	if convID != "" && !allowSwitch {
		binding := s.convMgr.Get(convID)
		if binding != nil {
			if err := s.validateBinding(binding); err == nil {
				// Bound channel is healthy — route directly to protect prompt cache
				s.mu.RLock()
				ch := s.channels[binding.ChannelName]
				s.mu.RUnlock()
				if ch != nil {
					rewrittenBody := rewriteModel(body, binding.Model)
					statusCode, respBody, respHeaders, err := s.tryChannel(ch, method, path, headers, rewrittenBody, callerID, tokenID)
					if err == nil && statusCode >= 200 && statusCode < 300 {
						ch.Health.RecordSuccess()
						if s.rateLimitLearner != nil && ch.Config.ID != 0 {
							s.rateLimitLearner.RecordSuccess(ch.Config.ID, modelName, false)
						}
						s.touchConversation(convID, ch, binding.Model, respBody)
						return statusCode, respBody, respHeaders, nil
					}
					// Bound channel failed: record it for health/circuit, drop
					// the binding, and fall through to group/direct selection
					// so the request still gets served (don't fail the task).
					s.recordChannelFailure(ch, statusCode, respBody, err)
					s.convMgr.Remove(convID)
				}
			}
		}
	}

	// Check if model matches a virtual model group
	if group, ok := modelGroups[modelName]; ok {
		statusCode, respBody, respHeaders, err = s.proxyViaGroup(group, taskKey, method, path, headers, body, callerID, tokenID, taskType)
		if err == nil && statusCode >= 200 && statusCode < 300 {
			// Record conversation binding on success
			if convID != "" {
				// Find which channel was actually used (from sticky binding)
				if taskKey != "" && group.Sticky != nil {
					if stickyCh, stickyModel := group.GetStickyBinding(taskKey); stickyCh != nil {
						s.convMgr.Set(convID, stickyCh.Config.ID, stickyCh.Config.Name, stickyModel, string(taskType))
						s.touchConversation(convID, stickyCh, stickyModel, respBody)
					}
				}
			}
		}
		return statusCode, respBody, respHeaders, err
	}

	// Direct channel matching (existing logic)
	channels := findChannelsInMap(modelMap, modelName)
	if len(channels) == 0 {
		errMsg := fmt.Sprintf("no channel available for model: %s", modelName)
		return 400, []byte(`{"error":"` + errMsg + `"}`), nil, fmt.Errorf("%s", errMsg)
	}

	// Filter out circuit-broken channels
	healthy := s.filterHealthy(channels)
	if len(healthy) == 0 {
		return 503, []byte(`{"error":"all channels circuit-broken for model: ` + modelName + `"}`), nil, nil
	}

	var lastErr error
	for _, ch := range healthy {
		statusCode, respBody, respHeaders, err = s.tryChannel(ch, method, path, headers, body, callerID, tokenID)
		// Only a true 2xx is a success — 4xx from one channel is often
		// channel-specific (model unsupported there, per-channel quota), so try
		// the next channel instead of failing the caller's task outright.
		if err == nil && statusCode >= 200 && statusCode < 300 {
			ch.Health.RecordSuccess()
			if s.rateLimitLearner != nil && ch.Config.ID != 0 {
				s.rateLimitLearner.RecordSuccess(ch.Config.ID, modelName, false)
			}
			if convID != "" {
				s.convMgr.Set(convID, ch.Config.ID, ch.Config.Name, modelName, string(taskType))
				s.touchConversation(convID, ch, modelName, respBody)
			}
			return statusCode, respBody, respHeaders, nil
		}
		result := s.recordChannelFailure(ch, statusCode, respBody, err)
		lastErr = err
		middleware.GetLogger().Warn("channel returned error, trying next",
			zap.String("channel", ch.Config.Name), zap.Int("status", statusCode),
			zap.String("model", modelName), zap.String("class", string(result.Class)))
	}

	// All channels failed — return the last real upstream response.
	return statusCode, respBody, respHeaders, lastErr
}

// recordChannelFailure classifies an upstream failure, applies the semantic breakdown
// to the channel's circuit breaker, and emits the matching aggregated alert (plus a
// circuit-open alert if the breaker tripped). It returns the classification so callers
// can branch on the class / log it. Shared by direct-match routing and rerank routing.
func (s *LLMProxyService) recordChannelFailure(ch *Channel, statusCode int, respBody []byte, err error) llmerrors.Result {
	errBody := ""
	if statusCode != 0 {
		errBody = string(respBody)
	}
	result := classifyChannelError(ch, statusCode, []byte(errBody))
	duration := llmerrors.BreakdownDuration(result.BreakdownUntil)
	ch.Health.RecordClassifiedFailure(classifyError(statusCode, err), string(result.Class), duration)
	s.alertForClass(ch.Config.Name, result.Class)
	if !ch.Health.IsAvailable() {
		s.recordAlert("circuit_open", "warning", ch.Config.Name,
			fmt.Sprintf("channel %s circuit opened (%s)", ch.Config.Name, result.Class))
	}
	return result
}

// proxyRerank routes a /v1/rerank request to a channel whose provider_type=="rerank"
// (Tier 7). It reuses tryChannel's circuit-breaker + token-bucket + logging path but
// skips chat conversion, virtual groups, and sticky conversation binding — rerank has
// no conversation semantics. Body is forwarded/returned verbatim (Cohere/Jina schema).
func (s *LLMProxyService) proxyRerank(
	modelName, method, path string,
	headers http.Header,
	body []byte,
	callerID string,
	tokenID uint,
) (statusCode int, respBody []byte, respHeaders http.Header, err error) {
	s.mu.RLock()
	modelMap := s.modelMap
	s.mu.RUnlock()

	candidates := findChannelsInMap(modelMap, modelName)
	rerank := make([]*Channel, 0, len(candidates))
	for _, ch := range candidates {
		if ch.Config.ProviderType == "rerank" {
			rerank = append(rerank, ch)
		}
	}
	if len(rerank) == 0 {
		errMsg := fmt.Sprintf("no rerank channel available for model: %s", modelName)
		return 400, []byte(`{"error":"` + errMsg + `"}`), nil, fmt.Errorf("%s", errMsg)
	}
	healthy := s.filterHealthy(rerank)
	if len(healthy) == 0 {
		return 503, []byte(`{"error":"all rerank channels circuit-broken for model: ` + modelName + `"}`), nil, nil
	}

	var lastErr error
	for _, ch := range healthy {
		statusCode, respBody, respHeaders, err = s.tryChannel(ch, method, path, headers, body, callerID, tokenID)
		if err == nil && statusCode < 500 && statusCode != 429 {
			ch.Health.RecordSuccess()
			if s.rateLimitLearner != nil && ch.Config.ID != 0 {
				s.rateLimitLearner.RecordSuccess(ch.Config.ID, modelName, false)
			}
			return statusCode, respBody, respHeaders, nil
		}
		result := s.recordChannelFailure(ch, statusCode, respBody, err)
		lastErr = err
		middleware.GetLogger().Warn("rerank channel returned error, trying next",
			zap.String("channel", ch.Config.Name), zap.Int("status", statusCode),
			zap.String("model", modelName), zap.String("class", string(result.Class)))
	}
	return statusCode, respBody, respHeaders, lastErr
}

// codingPref resolves the coding routing preference for tier ordering. For the
// complexity_aware strategy it derives simple/medium/complex from the prompt;
// otherwise it returns the configured free_first / quality_first directly.
func (s *LLMProxyService) codingPref(body []byte) string {
	if s.taskRouter == nil {
		return "complexity_aware"
	}
	strategy := s.taskRouter.GetCodingStrategy()
	if strategy == "complexity_aware" {
		return string(s.taskRouter.DetectComplexity(nil, body, estimateTokens(body)))
	}
	return strategy
}

// balanceSnapshot returns a channel→headroom map from the balance manager, used
// by the balance_aware routing strategy. Kind is carried alongside the value so
// USD amounts and 0..1 quota fractions are normalised separately (never mixed
// raw). Channels with fetch errors are omitted.
func (s *LLMProxyService) balanceSnapshot() map[string]channelBalance {
	if s.balanceMgr == nil {
		return nil
	}
	all := s.balanceMgr.GetAll()
	if len(all) == 0 {
		return nil
	}
	m := make(map[string]channelBalance, len(all))
	for name, info := range all {
		if info != nil && info.Error == "" {
			m[name] = channelBalance{Remaining: info.Balance, Kind: info.Kind}
		}
	}
	return m
}

// proxyViaGroup routes a request through a virtual model group with sticky binding.
func (s *LLMProxyService) proxyViaGroup(
	group *ModelGroup,
	taskKey, method, path string,
	headers http.Header,
	body []byte,
	callerID string,
	tokenID uint,
	taskType TaskType,
) (int, []byte, http.Header, error) {
	modelName := extractModelFromBody(body) // original virtual model name for logging
	maxAttempts := len(group.Members)
	tried := map[string]bool{}

	// Context-aware pre-filter: skip members whose declared window is smaller
	// than the estimated request budget (prompt + output). Output budget is
	// clamped per member to its max_output_tokens so a lower output cap doesn't
	// wrongly exclude the member. Dropped entirely if it would exclude every
	// member — never fail a request purely on estimation; a misestimate is
	// caught reactively by context_too_long failover.
	ctxExclude := group.contextExcluded(estimateInputTokens(body), estimateOutputBudget(body))
	if len(ctxExclude) >= maxAttempts {
		ctxExclude = nil
	}

	// Resolve coding sub-strategy + balance snapshot once (task-aware tiered routing).
	codingPref := ""
	if taskType == TaskCoding {
		codingPref = s.codingPref(body)
	}
	balances := s.balanceSnapshot()

	var lastStatus int
	var lastBody []byte
	var lastHeaders http.Header

	for attempt := 0; attempt < maxAttempts; attempt++ {
		ch, realModel := group.SelectChannel(taskKey, taskType, codingPref, balances, mergeExcludes(tried, ctxExclude))
		if ch == nil {
			break
		}
		tried[memberKey(ch.Config.Name, realModel)] = true

		// Rewrite the model name in request body to the real model name
		rewrittenBody := rewriteModel(body, realModel)
		// Clamp an over-large output budget to the member model's maximum so a
		// low-cap model (e.g. flash-lite 65536) can still serve a caller that
		// requested 131072 instead of rejecting the request outright.
		if member := group.findMember(ch.Config.Name, realModel); member != nil && member.Config.MaxOutputTokens > 0 {
			rewrittenBody = capOutputTokens(rewrittenBody, member.Config.MaxOutputTokens)
		}

		statusCode, respBody, respHeaders, err := s.tryChannel(
			ch, method, path, headers, rewrittenBody, callerID, tokenID,
		)

		// Log with original virtual model name for traceability
		logErr := ""
		if err != nil {
			logErr = err.Error()
		} else if statusCode >= 400 {
			logErr = truncateForLog(string(respBody))
		}
		s.logGroupRequest(ch.Config.Name, modelName, realModel, path, statusCode, logErr, callerID, tokenID)

		// Only a true 2xx is a success. Anything else — including 4xx, which
		// used to be passed straight through to the caller — advances to the
		// next member so the task survives per-member/per-channel quirks
		// (context too long, per-model quota, provider-specific rejections).
		if err == nil && statusCode >= 200 && statusCode < 300 {
			ch.Health.RecordSuccess()
			if member := group.findMember(ch.Config.Name, realModel); member != nil {
				member.RecordMemberSuccess()
			}
			if s.rateLimitLearner != nil && ch.Config.ID != 0 {
				s.rateLimitLearner.RecordSuccess(ch.Config.ID, realModel, false)
			}
			if taskKey != "" && group.Sticky != nil {
				group.Sticky.Renew(taskKey, group.Config.StickyTTLSeconds)
			}
			return statusCode, respBody, respHeaders, nil
		}

		// Failure: classify at the right scope (member vs channel), clear
		// sticky binding, try next member.
		result := s.recordRoutedFailure(ch, group.findMember(ch.Config.Name, realModel), statusCode, respBody, err)
		if taskKey != "" && group.Sticky != nil {
			group.Sticky.Remove(taskKey)
		}
		if statusCode != 0 || respBody != nil {
			lastStatus, lastBody, lastHeaders = statusCode, respBody, respHeaders
		}
		middleware.GetLogger().Warn("group member failed, trying next",
			zap.String("group", group.Config.Name), zap.String("channel", ch.Config.Name),
			zap.String("model", realModel), zap.Int("status", statusCode),
			zap.String("class", string(result.Class)), zap.NamedError("err", err))
	}

	// All members failed: pass through the last real upstream response so the
	// caller still sees a genuine error (not a synthetic 503 that masks it).
	if lastBody != nil || lastStatus != 0 {
		return lastStatus, lastBody, lastHeaders, nil
	}
	return 503, []byte(`{"error":"all group members exhausted for: ` + modelName + `"}`), nil, nil
}

// filterHealthy filters out channels whose circuit breaker is open.
func (s *LLMProxyService) filterHealthy(channels []*Channel) []*Channel {
	var healthy []*Channel
	for _, ch := range channels {
		if ch.Health.IsAvailable() {
			healthy = append(healthy, ch)
		}
	}
	return healthy
}
func findChannelsInMap(modelMap map[string][]*Channel, modelName string) []*Channel {
	// Exact match
	if chs, ok := modelMap[modelName]; ok {
		return chs
	}
	// Case-insensitive match
	lower := strings.ToLower(modelName)
	for m, chs := range modelMap {
		if strings.ToLower(m) == lower {
			return chs
		}
	}
	// Substring match
	for m, chs := range modelMap {
		if strings.Contains(strings.ToLower(m), lower) || strings.Contains(lower, strings.ToLower(m)) {
			return chs
		}
	}
	return nil
}

func extractModelFromBody(body []byte) string {
	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		middleware.GetLogger().Warn("failed to extract model from body", zap.Error(err))
		return ""
	}
	return req.Model
}

// capOutputTokens clamps max_tokens / max_completion_tokens in an
// OpenAI-compatible request body down to the member model's maximum output. No-op
// when maxOutput <= 0 or the request's budget is already within the limit.
func capOutputTokens(body []byte, maxOutput int) []byte {
	if maxOutput <= 0 {
		return body
	}
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return body
	}
	changed := false
	if v, ok := m["max_tokens"].(float64); ok && v > float64(maxOutput) {
		m["max_tokens"] = maxOutput
		changed = true
	}
	if v, ok := m["max_completion_tokens"].(float64); ok && v > float64(maxOutput) {
		m["max_completion_tokens"] = maxOutput
		changed = true
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

// isMemberScopedClass reports whether an error class describes a specific
// (channel, model) member rather than the whole channel. Per-model credit
// pools (e.g. SenseNova: glm-5.2 and flash-lite have independent quotas) must
// only take their own member out of rotation, not sibling models.
func isMemberScopedClass(c llmerrors.Class) bool {
	switch c {
	case llmerrors.QuotaExhausted, llmerrors.BalanceZero, llmerrors.SubscriptionInvalid:
		return true
	}
	return false
}

// recordRoutedFailure records a failed group-routing attempt with the right
// scope:
//   - context_too_long / max_output_exceeded: not a fault at all — no health
//     impact (a (request, model) property the router handles by capping/skipping);
//   - member-scoped classes (per-model quota/balance/subscription): member-level
//     cooldown, so sibling models on the same channel stay in rotation;
//   - everything else: channel-level circuit-breaker failure (existing behavior).
func (s *LLMProxyService) recordRoutedFailure(ch *Channel, member *ModelGroupMemberRuntime, statusCode int, respBody []byte, err error) llmerrors.Result {
	errBody := ""
	if statusCode != 0 {
		errBody = string(respBody)
	}
	result := classifyChannelError(ch, statusCode, []byte(errBody))
	switch {
	case result.Class == llmerrors.ContextTooLong || result.Class == llmerrors.MaxOutputExceeded:
		// Request exceeds this member's window/output cap — a routing input, not a fault.
	case member != nil && isMemberScopedClass(result.Class):
		member.RecordMemberBreakdown(string(result.Class), llmerrors.BreakdownDuration(result.BreakdownUntil))
		s.alertForClass(ch.Config.Name, result.Class)
	default:
		duration := llmerrors.BreakdownDuration(result.BreakdownUntil)
		ch.Health.RecordClassifiedFailure(classifyError(statusCode, err), string(result.Class), duration)
		s.alertForClass(ch.Config.Name, result.Class)
		if !ch.Health.IsAvailable() {
			s.recordAlert("circuit_open", "warning", ch.Config.Name,
				fmt.Sprintf("channel %s circuit opened (%s)", ch.Config.Name, result.Class))
		}
	}
	return result
}

// mergeExcludes returns a fresh exclude set combining tried members and
// context-excluded members for this request.
func mergeExcludes(tried, ctxExclude map[string]bool) map[string]bool {
	if len(ctxExclude) == 0 {
		return tried
	}
	merged := make(map[string]bool, len(tried)+len(ctxExclude))
	for k := range tried {
		merged[k] = true
	}
	for k := range ctxExclude {
		merged[k] = true
	}
	return merged
}
