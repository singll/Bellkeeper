package llmgateway

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
)

// ValidationError marks a caller-supplied configuration problem. Handlers map it
// to HTTP 400 instead of 500 so the UI can show the actionable message.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

func invalidf(format string, args ...interface{}) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}

var supportedProtocols = map[string]bool{
	"openai":    true,
	"anthropic": true,
	"rerank":    true,
}

var supportedGroupStrategies = map[string]bool{
	"priority-health": true,
	"best-weight":     true,
	"least_latency":   true,
	"balance_aware":   true,
}

var supportedBalanceProviders = map[string]bool{
	"deepseek":   true,
	"moonshot":   true,
	"newapi":     true,
	"aliyun":     true,
	"opencodego": true,
}

var supportedTiers = map[string]bool{
	"":         true,
	"free":     true,
	"standard": true,
	"premium":  true,
}

// InferVendor derives the vendor from a known upstream base URL. It is only a
// convenience for legacy/YAML rows; an explicit vendor always wins.
func InferVendor(baseURL string) string {
	u := strings.ToLower(baseURL)
	switch {
	case strings.Contains(u, "deepseek.com"):
		return "deepseek"
	case strings.Contains(u, "api.kimi.com"):
		return "kimi-code"
	case strings.Contains(u, "moonshot.cn"):
		return "moonshot"
	case strings.Contains(u, "opencode.ai"):
		return "opencodego"
	case strings.Contains(u, "anthropic.com"):
		return "anthropic"
	case strings.Contains(u, "aliyuncs.com"):
		return "aliyun"
	case strings.Contains(u, "openai.com"):
		return "openai"
	case strings.Contains(u, "sensenova"):
		return "sensenova"
	case strings.Contains(u, "siliconflow"):
		return "siliconflow"
	default:
		return ""
	}
}

func normalizeVendor(vendor, baseURL string) string {
	v := strings.ToLower(strings.TrimSpace(vendor))
	if v != "" {
		return v
	}
	return InferVendor(baseURL)
}

// errorVendor returns the vendor identity used by the error classifier. The
// protocol (openai/anthropic/rerank) must never impersonate a vendor: when the
// explicit vendor is empty we only infer it from the base URL.
func errorVendor(ch *Channel) string {
	if ch.Config.Vendor != "" {
		return ch.Config.Vendor
	}
	return InferVendor(ch.Config.BaseURL)
}

func validateChannel(ch *model.LLMChannel) error {
	ch.Name = strings.TrimSpace(ch.Name)
	if ch.Name == "" {
		return invalidf("channel name is required")
	}
	if len(ch.Name) > 100 {
		return invalidf("channel name must be at most 100 characters")
	}

	ch.BaseURL = strings.TrimRight(strings.TrimSpace(ch.BaseURL), "/")
	if ch.BaseURL == "" {
		return invalidf("base_url is required")
	}
	u, err := url.Parse(ch.BaseURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return invalidf("base_url must be an absolute http(s) URL, got %q", ch.BaseURL)
	}
	if strings.HasSuffix(u.Path, "/v1") {
		return invalidf("base_url must not end with /v1 (the proxy appends the API path)")
	}

	ch.ProviderType = strings.ToLower(strings.TrimSpace(ch.ProviderType))
	if ch.ProviderType == "" {
		ch.ProviderType = "openai"
	}
	if !supportedProtocols[ch.ProviderType] {
		return invalidf("provider_type must be one of: openai, anthropic, rerank")
	}
	ch.Vendor = normalizeVendor(ch.Vendor, ch.BaseURL)

	if ch.RPM < 0 || ch.RPD < 0 || ch.Priority < 0 || ch.QuotaWindowSeconds < 0 {
		return invalidf("rpm, rpd, priority and quota_window_seconds must be >= 0")
	}

	models, err := normalizeStringList(ch.Models, "models")
	if err != nil {
		return err
	}
	if len(models) == 0 {
		return invalidf("models must contain at least one model")
	}
	encoded, err := json.Marshal(models)
	if err != nil {
		return invalidf("invalid models: %v", err)
	}
	ch.Models = string(encoded)

	taskTypes, err := normalizeStringList(ch.TaskTypes, "task_types")
	if err != nil {
		return err
	}
	encoded, err = json.Marshal(taskTypes)
	if err != nil {
		return invalidf("invalid task_types: %v", err)
	}
	ch.TaskTypes = string(encoded)

	ch.Tier = strings.ToLower(strings.TrimSpace(ch.Tier))
	if !supportedTiers[ch.Tier] {
		return invalidf("tier must be empty, free, standard or premium")
	}

	ch.BalanceProviderType = strings.ToLower(strings.TrimSpace(ch.BalanceProviderType))
	if ch.BalanceProviderType != "" && !supportedBalanceProviders[ch.BalanceProviderType] {
		return invalidf("balance_provider_type must be empty or one of: deepseek, moonshot, newapi, aliyun, opencodego")
	}
	if strings.TrimSpace(ch.BalanceConfigJSON) != "" {
		var cfg map[string]interface{}
		if err := json.Unmarshal([]byte(ch.BalanceConfigJSON), &cfg); err != nil {
			return invalidf("balance_config_json must be a JSON object: %v", err)
		}
	}

	ch.ModelRPMOverrides = strings.TrimSpace(ch.ModelRPMOverrides)
	if ch.ModelRPMOverrides != "" {
		var overrides map[string]int
		if err := json.Unmarshal([]byte(ch.ModelRPMOverrides), &overrides); err != nil {
			return invalidf("model_rpm_overrides must be a JSON object of model to rpm: %v", err)
		}
		for m, rpm := range overrides {
			if strings.TrimSpace(m) == "" || rpm <= 0 {
				return invalidf("model_rpm_overrides requires non-empty model keys and rpm > 0")
			}
		}
	}

	return nil
}

func normalizeStringList(raw, field string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return []string{}, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, invalidf("%s must be a JSON array of strings: %v", field, err)
	}
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out, nil
}

func validateGroup(g *model.LLMModelGroup, channelRepo *repository.LLMChannelRepository) error {
	g.Name = strings.TrimSpace(g.Name)
	if g.Name == "" {
		return invalidf("group name is required")
	}
	if len(g.Name) > 100 {
		return invalidf("group name must be at most 100 characters")
	}

	g.Strategy = strings.TrimSpace(g.Strategy)
	if g.Strategy == "" {
		g.Strategy = "priority-health"
	}
	if !supportedGroupStrategies[g.Strategy] {
		return invalidf("strategy must be one of: priority-health, best-weight, least_latency, balance_aware")
	}
	if g.StickyTTLSeconds < 0 {
		return invalidf("sticky_ttl_seconds must be >= 0")
	}
	if len(g.Members) == 0 {
		return invalidf("group must have at least one member")
	}

	seen := make(map[string]bool, len(g.Members))
	for i := range g.Members {
		m := &g.Members[i]
		m.ChannelName = strings.TrimSpace(m.ChannelName)
		m.Model = strings.TrimSpace(m.Model)
		if m.ChannelName == "" || m.Model == "" {
			return invalidf("member %d requires channel_name and model", i+1)
		}
		if m.Weight <= 0 {
			m.Weight = 1
		}
		if m.MaxContextTokens < 0 || m.MaxOutputTokens < 0 {
			return invalidf("member %s/%s context/output limits must be >= 0", m.ChannelName, m.Model)
		}
		key := memberKey(m.ChannelName, m.Model)
		if seen[key] {
			return invalidf("duplicate member %s", key)
		}
		seen[key] = true

		ch, err := channelRepo.GetByName(m.ChannelName)
		if err != nil {
			return invalidf("member channel %q does not exist; create it on the Channels page first", m.ChannelName)
		}
		if !containsString(ch.GetModels(), m.Model) {
			return invalidf("model %q is not in channel %q's model list; add it on the Channels page first", m.Model, m.ChannelName)
		}
	}
	return nil
}
