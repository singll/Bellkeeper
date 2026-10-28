package llmgateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	llmerrors "github.com/singll/bellkeeper/internal/llmgateway/errors"
	"github.com/singll/bellkeeper/internal/middleware"
	"go.uber.org/zap"
)

// --- Streaming Proxy ---

// StreamResult holds the result of a streaming proxy request.
type StreamResult struct {
	StatusCode     int
	RespHeaders    http.Header
	BodyReader     io.ReadCloser
	ProviderType   string
	ChannelName    string
	ModelName      string
	ConversationID string
	StartedAt      time.Time
	tracker        *streamUsageTracker
}

// IsStreamRequest checks if the request body has stream:true.
func (s *LLMProxyService) IsStreamRequest(body []byte) bool {
	var req struct {
		Stream bool `json:"stream"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return false
	}
	return req.Stream
}

// ProxyStreamRequest forwards an OpenAI-compatible streaming request.
// For virtual model groups and direct channels it implements the same
// (channel, model)-level failover as the non-streaming path: a non-2xx
// response received before the stream starts is treated as a retryable
// failure and the next member is attempted.
// Returns a StreamResult with an open BodyReader that the caller must close.
func (s *LLMProxyService) ProxyStreamRequest(
	method, path string,
	headers http.Header,
	body []byte,
	callerID string,
	tokenID uint,
) (*StreamResult, error) {
	modelName := extractModelFromBody(body)

	s.mu.RLock()
	modelGroups := s.modelGroups
	modelMap := s.modelMap
	s.mu.RUnlock()

	taskKey := headers.Get("X-Task-Key")
	if taskKey == "" && callerID != "" && callerID != "unknown" {
		taskKey = callerID + ":" + modelName
	}

	headerMap := headerToMap(headers)
	taskType := s.taskRouter.DetectTaskType(headerMap, modelName, body, estimateTokens(body))
	convID := s.convMgr.ImplicitConversationID(headerMap, body)
	allowSwitch := headers.Get("X-Allow-Channel-Switch") == "true"

	// --- Conversation binding (sticky session) ---
	// If the bound channel/model fails, clear the binding and fall through to
	// normal group/direct selection so we still get failover.
	if convID != "" && !allowSwitch {
		binding := s.convMgr.Get(convID)
		if binding != nil {
			boundOK := false
			if err := s.validateBinding(binding); err == nil {
				s.mu.RLock()
				ch := s.channels[binding.ChannelName]
				s.mu.RUnlock()
				if ch != nil {
					rewrittenBody := rewriteModel(body, binding.Model)
					result, err := s.tryChannelStream(ch, method, path, headers, rewrittenBody, callerID)
					if err == nil && result.StatusCode >= 200 && result.StatusCode < 300 {
						result.ModelName = binding.Model
						result.ConversationID = convID
						ch.Health.RecordSuccess()
						if s.rateLimitLearner != nil && ch.Config.ID != 0 {
							s.rateLimitLearner.RecordSuccess(ch.Config.ID, binding.Model, false)
						}
						return result, nil
					}
					if err == nil {
						s.recordStreamFailure(ch, binding.Model, binding.Model, result, path, callerID, tokenID)
					} else {
						s.recordStreamFailure(ch, binding.Model, binding.Model, nil, path, callerID, tokenID)
					}
				}
			}
			if !boundOK {
				s.convMgr.Remove(convID)
			}
		}
	}

	// Virtual model group with (channel, model)-level failover.
	if group, ok := modelGroups[modelName]; ok {
		return s.proxyStreamViaGroup(group, taskKey, method, path, headers, body, callerID, tokenID, taskType, convID)
	}

	// Direct channel matching with failover across healthy channels.
	return s.proxyStreamDirect(modelMap, modelName, method, path, headers, body, callerID, tokenID, convID)
}

// proxyStreamViaGroup routes a streaming request through a virtual model group,
// trying each (channel, model) member until one returns a <400 status.
func (s *LLMProxyService) proxyStreamViaGroup(
	group *ModelGroup,
	taskKey, method, path string,
	headers http.Header,
	body []byte,
	callerID string,
	tokenID uint,
	taskType TaskType,
	convID string,
) (*StreamResult, error) {
	modelName := extractModelFromBody(body)
	maxAttempts := len(group.Members)
	tried := make(map[string]bool, maxAttempts)

	// Context-aware pre-filter, same as the buffered path.
	ctxExclude := group.contextExcluded(estimateInputTokens(body), estimateOutputBudget(body))
	if len(ctxExclude) >= maxAttempts {
		ctxExclude = nil
	}

	codingPref := ""
	if taskType == TaskCoding {
		codingPref = s.codingPref(body)
	}
	balances := s.balanceSnapshot()

	var lastResult *StreamResult
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		ch, realModel := group.SelectChannel(taskKey, taskType, codingPref, balances, mergeExcludes(tried, ctxExclude))
		if ch == nil {
			break
		}
		tried[memberKey(ch.Config.Name, realModel)] = true

		rewrittenBody := rewriteModel(body, realModel)
		if member := group.findMember(ch.Config.Name, realModel); member != nil && member.Config.MaxOutputTokens > 0 {
			rewrittenBody = capOutputTokens(rewrittenBody, member.Config.MaxOutputTokens)
		}
		result, err := s.tryChannelStream(ch, method, path, headers, rewrittenBody, callerID)
		if err != nil {
			s.recordStreamFailure(ch, modelName, realModel, nil, path, callerID, tokenID)
			lastErr = err
			if taskKey != "" && group.Sticky != nil {
				group.Sticky.Remove(taskKey)
			}
			continue
		}

		if result.StatusCode >= 200 && result.StatusCode < 300 {
			result.ModelName = modelName + "→" + realModel
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
			if convID != "" {
				s.convMgr.Set(convID, ch.Config.ID, ch.Config.Name, realModel, "")
				result.ConversationID = convID
			}
			return result, nil
		}

		s.recordStreamFailureScoped(ch, group.findMember(ch.Config.Name, realModel), modelName, realModel, result, path, callerID, tokenID)
		lastResult = result
		if taskKey != "" && group.Sticky != nil {
			group.Sticky.Remove(taskKey)
		}
		middleware.GetLogger().Warn("stream group member failed, trying next",
			zap.String("group", group.Config.Name),
			zap.String("channel", ch.Config.Name),
			zap.String("model", realModel),
			zap.Int("status", result.StatusCode))
	}

	if lastResult != nil {
		return lastResult, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("all group members exhausted for: %s", modelName)
}

// proxyStreamDirect routes a streaming request through all healthy channels
// serving the requested model, in priority order.
func (s *LLMProxyService) proxyStreamDirect(
	modelMap map[string][]*Channel,
	modelName, method, path string,
	headers http.Header,
	body []byte,
	callerID string,
	tokenID uint,
	convID string,
) (*StreamResult, error) {
	channels := findChannelsInMap(modelMap, modelName)
	if len(channels) == 0 {
		return nil, fmt.Errorf("no channel available for model: %s", modelName)
	}

	healthy := s.filterHealthy(channels)
	if len(healthy) == 0 {
		return nil, fmt.Errorf("all channels circuit-broken for model: %s", modelName)
	}

	var lastResult *StreamResult
	var lastErr error
	for _, ch := range healthy {
		result, err := s.tryChannelStream(ch, method, path, headers, body, callerID)
		if err != nil {
			lastErr = err
			s.recordStreamFailure(ch, "", modelName, nil, path, callerID, tokenID)
			continue
		}
		if result.StatusCode >= 200 && result.StatusCode < 300 {
			result.ModelName = modelName
			ch.Health.RecordSuccess()
			if s.rateLimitLearner != nil && ch.Config.ID != 0 {
				s.rateLimitLearner.RecordSuccess(ch.Config.ID, modelName, false)
			}
			if convID != "" {
				s.convMgr.Set(convID, ch.Config.ID, ch.Config.Name, modelName, "")
				result.ConversationID = convID
			}
			return result, nil
		}
		s.recordStreamFailure(ch, "", modelName, result, path, callerID, tokenID)
		lastResult = result
	}

	// All channels failed — surface the last real upstream response (or the last
	// transport error) instead of masking it with a synthetic failure.
	if lastResult != nil {
		return lastResult, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("all channels exhausted for model: %s", modelName)
}

// recordStreamFailure logs a failed streaming attempt, updates channel health,
// records rate-limit learning data, and re-wraps the response body so the
// caller can still return it if this was the last attempt.
func (s *LLMProxyService) recordStreamFailure(
	ch *Channel,
	virtualModel, realModel string,
	result *StreamResult,
	path string,
	callerID string,
	tokenID uint,
) {
	s.recordStreamFailureScoped(ch, nil, virtualModel, realModel, result, path, callerID, tokenID)
}

// recordStreamFailureScoped is recordStreamFailure with member-scoped failure
// semantics: member != nil routes per-model quota/balance/subscription classes
// to a member-level cooldown (sibling models on the same channel stay in
// rotation) and skips health impact entirely for context_too_long.
func (s *LLMProxyService) recordStreamFailureScoped(
	ch *Channel,
	member *ModelGroupMemberRuntime,
	virtualModel, realModel string,
	result *StreamResult,
	path string,
	callerID string,
	tokenID uint,
) {
	statusCode := 0
	errBody := ""
	if result != nil {
		statusCode = result.StatusCode
		if result.BodyReader != nil {
			buf, _ := io.ReadAll(result.BodyReader)
			errBody = string(buf)
			result.BodyReader = io.NopCloser(bytes.NewReader(buf))
		}
	}

	cls := classifyChannelError(ch, statusCode, []byte(errBody))
	switch {
	case cls.Class == llmerrors.ContextTooLong || cls.Class == llmerrors.MaxOutputExceeded:
		// Request exceeds this member's window/output cap — a routing input, not a fault.
	case member != nil && isMemberScopedClass(cls.Class):
		member.RecordMemberBreakdown(string(cls.Class), llmerrors.BreakdownDuration(cls.BreakdownUntil))
		s.alertForClass(ch.Config.Name, cls.Class)
	default:
		duration := llmerrors.BreakdownDuration(cls.BreakdownUntil)
		ch.Health.RecordClassifiedFailure(classifyError(statusCode, nil), string(cls.Class), duration)
		s.alertForClass(ch.Config.Name, cls.Class)
		if !ch.Health.IsAvailable() {
			s.recordAlert("circuit_open", "warning", ch.Config.Name,
				fmt.Sprintf("channel %s circuit opened (%s)", ch.Config.Name, cls.Class))
		}
	}
	if statusCode == 429 && s.rateLimitLearner != nil && ch.Config.ID != 0 {
		s.rateLimitLearner.Record429(ch.Config.ID, realModel, ch.Config.RPM)
	}

	if virtualModel != "" {
		s.logGroupRequest(ch.Config.Name, virtualModel, realModel, path, statusCode, truncateForLog(errBody), callerID, tokenID)
	} else {
		s.logRequest(ch.Config.Name, realModel, path, statusCode, statusCode == 429,
			0, 0, 0, 0, 0, truncateForLog(errBody), callerID, tokenID)
	}
}

// truncateForLog clamps an upstream error body to a log-safe length (first 300 chars, single line).
func truncateForLog(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 300 {
		return s[:300]
	}
	return s
}
func (s *LLMProxyService) FinalizeStream(result *StreamResult, path, callerID string, tokenID uint) {
	if result == nil {
		return
	}
	promptTokens, compTokens, cachedTokens := 0, 0, 0
	errMsg := ""
	if result.tracker != nil {
		promptTokens, compTokens, cachedTokens = result.tracker.Usage()
	}
	if result.StatusCode < 400 && promptTokens+compTokens+cachedTokens == 0 {
		errMsg = "stream usage unavailable"
	}
	startedAt := result.StartedAt
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	s.logRequest(result.ChannelName, result.ModelName, path, result.StatusCode, result.StatusCode == 429,
		0, int(time.Since(startedAt).Milliseconds()), promptTokens, compTokens, cachedTokens, errMsg, callerID, tokenID)

	pricingModel := result.ModelName
	if i := strings.Index(pricingModel, "→"); i >= 0 {
		pricingModel = pricingModel[i+len("→"):]
	}
	s.touchConversationUsage(result.ConversationID, result.ChannelName, pricingModel, Usage{
		PromptTokens:     promptTokens,
		CompletionTokens: compTokens,
		CachedTokens:     cachedTokens,
	})
}
