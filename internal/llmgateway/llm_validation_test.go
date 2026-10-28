package llmgateway

import (
	"testing"

	"github.com/singll/bellkeeper/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInferVendor(t *testing.T) {
	tests := []struct {
		baseURL string
		want    string
	}{
		{"https://api.deepseek.com", "deepseek"},
		{"https://api.moonshot.cn/v1", "moonshot"},
		{"https://api.kimi.com/coding", "kimi-code"},
		{"https://opencode.ai/zen/go", "opencodego"},
		{"https://api.anthropic.com", "anthropic"},
		{"https://dashscope.aliyuncs.com/compatible-mode", "aliyun"},
		{"https://api.siliconflow.cn", "siliconflow"},
		{"https://example.com", ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, InferVendor(tt.baseURL), tt.baseURL)
	}
}

func TestValidateChannel(t *testing.T) {
	valid := func() *model.LLMChannel {
		return &model.LLMChannel{
			Name:      "deepseek-direct",
			BaseURL:   "https://api.deepseek.com",
			Models:    `["deepseek-v4-flash"]`,
			TaskTypes: `["summary"]`,
			Tier:      "standard",
			RPM:       10,
			RPD:       500,
			Priority:  1,
		}
	}

	ch := valid()
	require.NoError(t, validateChannel(ch))
	assert.Equal(t, "openai", ch.ProviderType)
	assert.Equal(t, "deepseek", ch.Vendor)
	assert.JSONEq(t, `["deepseek-v4-flash"]`, ch.Models)

	t.Run("gemini removed", func(t *testing.T) {
		ch := valid()
		ch.ProviderType = "gemini"
		assert.Error(t, validateChannel(ch))
	})

	t.Run("base url must not include v1", func(t *testing.T) {
		ch := valid()
		ch.BaseURL = "https://api.example.com/v1"
		assert.Error(t, validateChannel(ch))
	})

	t.Run("models required", func(t *testing.T) {
		ch := valid()
		ch.Models = "[]"
		assert.Error(t, validateChannel(ch))
	})

	t.Run("models must be json array", func(t *testing.T) {
		ch := valid()
		ch.Models = "deepseek-v4-flash"
		assert.Error(t, validateChannel(ch))
	})

	t.Run("balance provider enum", func(t *testing.T) {
		ch := valid()
		ch.BalanceProviderType = "siliconflow"
		assert.Error(t, validateChannel(ch))
	})

	t.Run("tier enum", func(t *testing.T) {
		ch := valid()
		ch.Tier = "gold"
		assert.Error(t, validateChannel(ch))
	})

	t.Run("rerank allowed", func(t *testing.T) {
		ch := valid()
		ch.ProviderType = "rerank"
		assert.NoError(t, validateChannel(ch))
	})
}
