package llmgateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/httpclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildModelCatalog(t *testing.T) {
	channels := []model.LLMChannel{
		{
			ID: 1, Name: "siliconflow", BaseURL: "https://api.siliconflow.cn",
			ProviderType: "", Vendor: "", IsFree: true, IsEnabled: true,
			RPM: 500, RPD: 50000, Models: `["m1","m2"]`,
		},
		{
			ID: 2, Name: "claude", BaseURL: "https://api.anthropic.com",
			ProviderType: "anthropic", Vendor: "anthropic", Tier: "premium", IsEnabled: true,
			RPM: 10, RPD: 100, Models: `["claude-sonnet"]`,
		},
	}
	groups := []model.LLMModelGroup{
		{
			Name: "g1",
			Members: []model.LLMModelGroupMember{
				{ChannelName: "siliconflow", Model: "m1", MaxContextTokens: 128000, MaxOutputTokens: 4096},
			},
		},
		{
			Name: "g2",
			Members: []model.LLMModelGroupMember{
				{ChannelName: "siliconflow", Model: "m1", MaxContextTokens: 256000, MaxOutputTokens: 8192},
			},
		},
	}

	catalog := buildModelCatalog(channels, groups)
	require.Len(t, catalog.Models, 3)

	byKey := map[string]ModelCatalogEntry{}
	for _, e := range catalog.Models {
		byKey[e.ChannelName+"/"+e.Model] = e
	}

	m1 := byKey["siliconflow/m1"]
	assert.Equal(t, uint(1), m1.ChannelID)
	assert.Equal(t, "openai", m1.ProviderType, "empty protocol defaults to openai")
	assert.Equal(t, "siliconflow", m1.Vendor, "vendor inferred from base URL")
	assert.Equal(t, "free", m1.Tier, "tier derived from is_free")
	assert.True(t, m1.IsFree)
	assert.Equal(t, 256000, m1.MaxContextTokens, "max context across groups")
	assert.Equal(t, 8192, m1.MaxOutputTokens, "max output across groups")
	assert.Equal(t, []string{"g1", "g2"}, m1.Groups)

	m2 := byKey["siliconflow/m2"]
	assert.Zero(t, m2.MaxContextTokens)
	assert.Empty(t, m2.Groups)

	claude := byKey["claude/claude-sonnet"]
	assert.Equal(t, "anthropic", claude.ProviderType)
	assert.Equal(t, "premium", claude.Tier)
	assert.Equal(t, "anthropic", claude.Vendor)
}

func TestParseModelIDs(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "openai data ids",
			body: `{"object":"list","data":[{"id":"gpt-4.1","object":"model"},{"id":"gpt-4o","object":"model"}]}`,
			want: []string{"gpt-4.1", "gpt-4o"},
		},
		{
			name: "anthropic data ids",
			body: `{"data":[{"id":"claude-sonnet-4-6","display_name":"Claude Sonnet"}]}`,
			want: []string{"claude-sonnet-4-6"},
		},
		{
			name: "names without spaces",
			body: `{"models":[{"name":"vendor/model-x"},{"name":"Display Name"}]}`,
			want: []string{"vendor/model-x"},
		},
		{
			name: "plain string array",
			body: `["a","b"]`,
			want: []string{"a", "b"},
		},
		{
			name: "unrecognized",
			body: `{"error":"nope"}`,
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseModelIDs([]byte(tt.body)))
		})
	}
}

func TestTestChannelConnection(t *testing.T) {
	const key = "sk-secret-value"
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer "+key, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"ok","choices":[{"message":{"content":"pong"}}]}`))
	}))
	t.Cleanup(srv.Close)

	ch := &Channel{
		Config: config.ChannelConfig{
			ID: 7, Name: "test", ProviderType: "openai", Vendor: "deepseek",
			BaseURL: srv.URL, APIKey: key, Models: []string{"deepseek-v4-flash"},
		},
		Client: httpclient.NewClientWithTimeout(5 * time.Second),
	}
	adapter, err := adapterFor(ch)
	require.NoError(t, err)

	result := testChannelConnection(ch, adapter, "deepseek-v4-flash")
	assert.True(t, result.OK)
	assert.Equal(t, http.StatusOK, result.StatusCode)
	assert.Equal(t, "连接成功", result.Message)
	assert.Equal(t, "openai", result.Protocol)
	assert.Equal(t, "deepseek", result.Vendor)
	assert.Equal(t, 1, hits)
	assert.NotContains(t, result.Message, key)
}

func TestTestChannelConnectionRedactsSecretOnError(t *testing.T) {
	const key = "sk-secret-value"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid key ` + key + `"}`))
	}))
	t.Cleanup(srv.Close)

	ch := &Channel{
		Config: config.ChannelConfig{
			ID: 8, Name: "bad", ProviderType: "openai", BaseURL: srv.URL,
			APIKey: key, Models: []string{"m"},
		},
		Client: httpclient.NewClientWithTimeout(5 * time.Second),
	}
	adapter, err := adapterFor(ch)
	require.NoError(t, err)

	result := testChannelConnection(ch, adapter, "m")
	assert.False(t, result.OK)
	assert.Equal(t, http.StatusUnauthorized, result.StatusCode)
	assert.Equal(t, "auth_failed", result.Class)
	assert.NotContains(t, result.Message, key)
	assert.Contains(t, result.Message, "***")
}

func TestDiscoverUpstreamModels(t *testing.T) {
	const key = "sk-secret-value"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/models", r.URL.Path)
		assert.Equal(t, "Bearer "+key, r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"data":[{"id":"m2"},{"id":"m1"}]}`))
	}))
	t.Cleanup(srv.Close)

	ch := &Channel{
		Config: config.ChannelConfig{
			ID: 9, Name: "catalog", ProviderType: "openai",
			BaseURL: srv.URL, APIKey: key, Models: []string{"m1"},
		},
		Client: httpclient.NewClientWithTimeout(5 * time.Second),
	}
	adapter, err := adapterFor(ch)
	require.NoError(t, err)

	models, warning := discoverUpstreamModels(ch, adapter)
	assert.Empty(t, warning)
	assert.Equal(t, []string{"m1", "m2"}, models)

	merged := mergeStrings(ch.Config.Models, models)
	assert.Equal(t, []string{"m1", "m2"}, merged)
}

func TestDiscoverUpstreamModelsReportsWarning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not supported"}`))
	}))
	t.Cleanup(srv.Close)

	ch := &Channel{
		Config: config.ChannelConfig{
			ID: 10, Name: "catalog", ProviderType: "openai",
			BaseURL: srv.URL, APIKey: "sk-x",
		},
		Client: httpclient.NewClientWithTimeout(5 * time.Second),
	}
	adapter, err := adapterFor(ch)
	require.NoError(t, err)

	models, warning := discoverUpstreamModels(ch, adapter)
	assert.Nil(t, models)
	assert.Contains(t, warning, "404")
}
