package llmgateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/httpclient"
)

// ModelCatalogEntry is one (channel, model) pair exposed to the management UI.
// It contains no credential material: only routing metadata and the member
// capability limits declared in model groups.
type ModelCatalogEntry struct {
	ChannelID        uint     `json:"channel_id"`
	ChannelName      string   `json:"channel_name"`
	Model            string   `json:"model"`
	ProviderType     string   `json:"provider_type"`
	Vendor           string   `json:"vendor"`
	IsFree           bool     `json:"is_free"`
	IsEnabled        bool     `json:"is_enabled"`
	Tier             string   `json:"tier"`
	RPM              int      `json:"rpm"`
	RPD              int      `json:"rpd"`
	MaxContextTokens int      `json:"max_context_tokens"`
	MaxOutputTokens  int      `json:"max_output_tokens"`
	Groups           []string `json:"groups"`
}

// ModelCatalog is the response payload of GET /api/llm/config/models.
type ModelCatalog struct {
	Models []ModelCatalogEntry `json:"models"`
}

// ChannelConnectionTestResult is a diagnostic response. It never contains the
// API key or request headers, and the test path deliberately bypasses billing,
// token usage, rate-limit buckets and channel health.
type ChannelConnectionTestResult struct {
	ChannelID   uint      `json:"channel_id"`
	ChannelName string    `json:"channel_name"`
	Protocol    string    `json:"protocol"`
	Vendor      string    `json:"vendor"`
	Model       string    `json:"model"`
	OK          bool      `json:"ok"`
	StatusCode  int       `json:"status_code"`
	LatencyMs   int64     `json:"latency_ms"`
	Class       string    `json:"class,omitempty"`
	Message     string    `json:"message"`
	TestedAt    time.Time `json:"tested_at"`
}

// ChannelModelDiscoveryResult lists upstream models plus the channel's
// currently registered models. Source=configured means discovery was not
// possible; Warning always carries the explicit reason (no silent downgrade).
type ChannelModelDiscoveryResult struct {
	ChannelID   uint      `json:"channel_id"`
	ChannelName string    `json:"channel_name"`
	Protocol    string    `json:"protocol"`
	Source      string    `json:"source"`
	Models      []string  `json:"models"`
	Configured  []string  `json:"configured"`
	Warning     string    `json:"warning,omitempty"`
	FetchedAt   time.Time `json:"fetched_at"`
}

type memberCapability struct {
	maxContext int
	maxOutput  int
	groups     map[string]bool
}

// ListModelCatalog aggregates registered channel models with the capability
// limits declared by model-group members.
func (s *LLMConfigService) ListModelCatalog() (*ModelCatalog, error) {
	channels, err := s.channelRepo.List()
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	groups, err := s.groupRepo.List()
	if err != nil {
		return nil, fmt.Errorf("list model groups: %w", err)
	}
	return buildModelCatalog(channels, groups), nil
}

func buildModelCatalog(channels []model.LLMChannel, groups []model.LLMModelGroup) *ModelCatalog {
	caps := make(map[string]*memberCapability)
	for _, g := range groups {
		for _, m := range g.Members {
			key := memberKey(m.ChannelName, m.Model)
			c := caps[key]
			if c == nil {
				c = &memberCapability{groups: make(map[string]bool)}
				caps[key] = c
			}
			c.groups[g.Name] = true
			if m.MaxContextTokens > c.maxContext {
				c.maxContext = m.MaxContextTokens
			}
			if m.MaxOutputTokens > c.maxOutput {
				c.maxOutput = m.MaxOutputTokens
			}
		}
	}

	catalog := &ModelCatalog{Models: make([]ModelCatalogEntry, 0)}
	for _, ch := range channels {
		protocol := ch.ProviderType
		if protocol == "" {
			protocol = "openai"
		}
		vendor := ch.Vendor
		if vendor == "" {
			vendor = InferVendor(ch.BaseURL)
		}
		tier := ch.Tier
		if tier == "" {
			if ch.IsFree {
				tier = "free"
			} else {
				tier = "standard"
			}
		}
		for _, m := range ch.GetModels() {
			entry := ModelCatalogEntry{
				ChannelID:    ch.ID,
				ChannelName:  ch.Name,
				Model:        m,
				ProviderType: protocol,
				Vendor:       vendor,
				IsFree:       ch.IsFree,
				IsEnabled:    ch.IsEnabled,
				Tier:         tier,
				RPM:          ch.RPM,
				RPD:          ch.RPD,
				Groups:       []string{},
			}
			if c := caps[memberKey(ch.Name, m)]; c != nil {
				entry.MaxContextTokens = c.maxContext
				entry.MaxOutputTokens = c.maxOutput
				entry.Groups = make([]string, 0, len(c.groups))
				for name := range c.groups {
					entry.Groups = append(entry.Groups, name)
				}
				sort.Strings(entry.Groups)
			}
			catalog.Models = append(catalog.Models, entry)
		}
	}
	sort.Slice(catalog.Models, func(i, j int) bool {
		if catalog.Models[i].ChannelName != catalog.Models[j].ChannelName {
			return catalog.Models[i].ChannelName < catalog.Models[j].ChannelName
		}
		return catalog.Models[i].Model < catalog.Models[j].Model
	})
	return catalog
}

// TestChannelConnection probes one saved channel/model pair without touching
// billing, token usage, rate-limit buckets or circuit-breaker health.
func (s *LLMProxyService) TestChannelConnection(channelID uint, modelName string) (*ChannelConnectionTestResult, error) {
	ch, err := s.loadTestChannel(channelID)
	if err != nil {
		return nil, err
	}
	adapter, err := adapterFor(ch)
	if err != nil {
		return nil, err
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" && len(ch.Config.Models) > 0 {
		modelName = ch.Config.Models[0]
	}
	if modelName == "" {
		return nil, fmt.Errorf("channel %q has no model to test", ch.Config.Name)
	}

	result := &ChannelConnectionTestResult{
		ChannelID:   ch.Config.ID,
		ChannelName: ch.Config.Name,
		Protocol:    adapter.Protocol(),
		Vendor:      ch.Config.Vendor,
		Model:       modelName,
		TestedAt:    time.Now(),
	}
	if ch.Config.APIKey == "" {
		result.Message = "未配置可用密钥（purpose=api 凭证或环境变量）"
		return result, nil
	}
	return testChannelConnection(ch, adapter, modelName), nil
}

func (s *LLMProxyService) loadTestChannel(channelID uint) (*Channel, error) {
	if s.channelRepo == nil {
		return nil, fmt.Errorf("channel repository not initialized")
	}
	dbCh, err := s.channelRepo.Get(channelID)
	if err != nil {
		return nil, fmt.Errorf("channel %d not found: %w", channelID, err)
	}
	cfg := s.dbChannelToConfig(*dbCh)
	timeout := time.Duration(s.cfg.DefaultTimeout) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &Channel{
		Config: cfg,
		Client: httpclient.NewClientWithTimeout(timeout),
	}, nil
}

func testChannelConnection(ch *Channel, adapter UpstreamAdapter, modelName string) *ChannelConnectionTestResult {
	result := &ChannelConnectionTestResult{
		ChannelID:   ch.Config.ID,
		ChannelName: ch.Config.Name,
		Protocol:    adapter.Protocol(),
		Vendor:      ch.Config.Vendor,
		Model:       modelName,
		TestedAt:    time.Now(),
	}
	body, path := adapter.ProbeRequest(modelName)
	req, _, _, err := prepareUpstreamRequest(ch, http.MethodPost, path, http.Header{}, body)
	if err != nil {
		result.Message = "构建探测请求失败：" + redactSecret(err.Error(), ch.Config.APIKey)
		return result
	}

	start := time.Now()
	resp, err := ch.Client.Do(req)
	result.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		result.Message = "连接失败：" + redactSecret(err.Error(), ch.Config.APIKey)
		return result
	}
	defer resp.Body.Close() //nolint:errcheck

	respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if readErr != nil {
		result.Message = "读取上游响应失败：" + redactSecret(readErr.Error(), ch.Config.APIKey)
		return result
	}
	result.StatusCode = resp.StatusCode
	result.OK = resp.StatusCode >= 200 && resp.StatusCode < 300
	if result.OK {
		result.Message = "连接成功"
		return result
	}
	class := adapter.ErrorClass(resp.StatusCode, respBody, ch)
	result.Class = string(class.Class)
	detail := redactSecret(string(respBody), ch.Config.APIKey)
	result.Message = fmt.Sprintf("%s: %s", class.Class, truncateForLog(detail))
	return result
}

// DiscoverChannelModels asks the upstream for its model list. It is read-only:
// no billing, token usage, health state or rate-limit bucket is touched. On
// failure it returns the configured list with an explicit warning and
// Source=configured (no silent degradation).
func (s *LLMProxyService) DiscoverChannelModels(channelID uint) (*ChannelModelDiscoveryResult, error) {
	ch, err := s.loadTestChannel(channelID)
	if err != nil {
		return nil, err
	}
	adapter, err := adapterFor(ch)
	if err != nil {
		return nil, err
	}
	configured := append([]string(nil), ch.Config.Models...)
	result := &ChannelModelDiscoveryResult{
		ChannelID:   ch.Config.ID,
		ChannelName: ch.Config.Name,
		Protocol:    adapter.Protocol(),
		Source:      "configured",
		Configured:  configured,
		Models:      append([]string(nil), configured...),
		FetchedAt:   time.Now(),
	}
	if adapter.Protocol() == "rerank" {
		result.Warning = "rerank 协议不支持上游模型发现，已返回渠道登记清单"
		return result, nil
	}
	if ch.Config.APIKey == "" {
		result.Warning = "未配置可用密钥，已返回渠道登记清单"
		return result, nil
	}

	discovered, warning := discoverUpstreamModels(ch, adapter)
	if warning != "" {
		result.Warning = warning
		return result, nil
	}
	result.Source = "upstream"
	result.Models = mergeStrings(configured, discovered)
	return result, nil
}

func discoverUpstreamModels(ch *Channel, adapter UpstreamAdapter) ([]string, string) {
	target := strings.TrimRight(ch.Config.BaseURL, "/") + "/v1/models"
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return nil, "构建模型发现请求失败：" + redactSecret(err.Error(), ch.Config.APIKey)
	}
	adapter.ApplyAuth(req, ch, PreparedRequest{})

	resp, err := ch.Client.Do(req)
	if err != nil {
		return nil, "上游模型发现失败：" + redactSecret(err.Error(), ch.Config.APIKey)
	}
	defer resp.Body.Close() //nolint:errcheck
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if readErr != nil {
		return nil, "读取上游模型列表失败：" + redactSecret(readErr.Error(), ch.Config.APIKey)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Sprintf("上游返回 %d：%s", resp.StatusCode,
			truncateForLog(redactSecret(string(body), ch.Config.APIKey)))
	}
	models := parseModelIDs(body)
	if len(models) == 0 {
		return nil, "上游未返回可识别的模型列表"
	}
	return models, ""
}

// parseModelIDs accepts the common OpenAI/Anthropic model-list shapes. It is
// intentionally permissive because compatible gateways differ in wrapping.
func parseModelIDs(body []byte) []string {
	var direct []string
	if err := json.Unmarshal(body, &direct); err == nil && len(direct) > 0 {
		return mergeStrings(nil, direct)
	}
	var raw interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil
	}
	ids := make(map[string]bool)
	collectModelIDs(raw, ids)
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func collectModelIDs(v interface{}, ids map[string]bool) {
	switch x := v.(type) {
	case []interface{}:
		for _, item := range x {
			collectModelIDs(item, ids)
		}
	case map[string]interface{}:
		if id, ok := x["id"].(string); ok && strings.TrimSpace(id) != "" {
			ids[strings.TrimSpace(id)] = true
		}
		if name, ok := x["name"].(string); ok && strings.TrimSpace(name) != "" &&
			looksLikeModelID(name) {
			ids[strings.TrimSpace(name)] = true
		}
		for key, child := range x {
			if key == "id" || key == "name" {
				continue
			}
			collectModelIDs(child, ids)
		}
	}
}

// looksLikeModelID avoids treating arbitrary "name" fields (provider display
// names) as model identifiers while still supporting gateways that only return
// a name.
func looksLikeModelID(s string) bool {
	if strings.ContainsAny(s, " \t\n") {
		return false
	}
	return strings.Contains(s, "-") || strings.Contains(s, "/") || strings.Contains(s, ".") ||
		strings.Contains(s, ":")
}

func mergeStrings(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, list := range [][]string{a, b} {
		for _, v := range list {
			v = strings.TrimSpace(v)
			if v == "" || seen[v] {
				continue
			}
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func redactSecret(s, secret string) string {
	if secret == "" {
		return s
	}
	return strings.ReplaceAll(s, secret, "***")
}
