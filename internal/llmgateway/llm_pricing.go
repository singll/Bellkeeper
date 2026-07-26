package llmgateway

import (
	"fmt"

	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
)

// Pricer calculates request costs based on model pricing configuration.
type Pricer struct {
	repo *repository.LLMModelPricingRepository
}

// NewPricer creates a new Pricer instance.
func NewPricer(repo *repository.LLMModelPricingRepository) *Pricer {
	return &Pricer{repo: repo}
}

// Usage holds token consumption details.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	CachedTokens     int // cached/prompt_tokens_details.cached_tokens
}

// Calc computes the cost in cents (rounded, for display) for a given channel,
// model, and usage. Falls back to zero cost if no pricing is configured.
func (p *Pricer) Calc(channelName, model string, usage Usage) (int, error) {
	mc, err := p.CalcMicroCents(channelName, model, usage)
	if err != nil {
		return 0, err
	}
	return int(MicroCentsToCents(mc)), nil
}

// CalcMicroCents computes the cost in micro-cents (1 cent = 1000 micro-cents) for
// precise accounting. Cheap models (e.g. DeepSeek at 14¢/1M) produce sub-cent
// per-request costs that integer-cent math would truncate to zero; micro-cents
// preserves that precision so daily/monthly aggregation is accurate.
func (p *Pricer) CalcMicroCents(channelName, model string, usage Usage) (int64, error) {
	pricing, err := p.repo.GetByChannelAndModel(channelName, model)
	if err != nil {
		return 0, fmt.Errorf("lookup pricing: %w", err)
	}
	if pricing == nil {
		return 0, nil // No pricing configured → free
	}
	return computeMicroCents(pricing, usage), nil
}

// computeMicroCents is the pure pricing arithmetic (no DB), extracted for testing.
func computeMicroCents(pricing *model.LLMModelPricing, usage Usage) int64 {
	cachedTokens := usage.CachedTokens
	if cachedTokens > usage.PromptTokens {
		cachedTokens = usage.PromptTokens
	}
	uncachedPromptTokens := usage.PromptTokens - cachedTokens

	// Prices are cents per 1M tokens. micro-cents = tokens * pricePer1MCents / 1000.
	inputMC := int64(uncachedPromptTokens) * int64(pricing.InputPricePer1MCents) / 1000
	outputMC := int64(usage.CompletionTokens) * int64(pricing.OutputPricePer1MCents) / 1000

	var cachedMC int64
	if pricing.CachedInputPricePer1MCents > 0 {
		cachedMC = int64(cachedTokens) * int64(pricing.CachedInputPricePer1MCents) / 1000
	} else {
		// Default cached price = input × 0.1. Divide LAST (÷10 then ÷1000 = ÷10000)
		// so a small input price doesn't floor to zero before multiplying tokens.
		cachedMC = int64(cachedTokens) * int64(pricing.InputPricePer1MCents) / 10000
	}

	total := inputMC + cachedMC + outputMC
	if total < 0 {
		total = 0
	}
	return total
}

// MicroCentsToCents converts micro-cents to cents, rounding half up.
func MicroCentsToCents(mc int64) int64 {
	if mc < 0 {
		return 0
	}
	return (mc + 500) / 1000
}

// SeedDefaultPricing inserts default pricing for known models if the table is empty.
func (p *Pricer) SeedDefaultPricing() error {
	count, err := p.repo.Count()
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	// ChannelName MUST match the runtime channel name (llm_channels.name) exactly —
	// billing looks up pricing by (channel_name, model) with an exact match, so a
	// mismatch here silently zeroes all cost accounting (regression fixed 2026-07-26).
	// Prices are USD cents per 1M tokens. Values marked EST are estimates pending
	// confirmation against current official pricing.
	defaults := []model.LLMModelPricing{
		{ChannelName: "deepseek-direct", Model: "deepseek-v4-flash", InputPricePer1MCents: 28, OutputPricePer1MCents: 110, CachedInputPricePer1MCents: 7, Notes: "DeepSeek V4 Flash ⚠️EST"},
		{ChannelName: "deepseek-direct", Model: "deepseek-v4-pro", InputPricePer1MCents: 55, OutputPricePer1MCents: 219, CachedInputPricePer1MCents: 14, Notes: "DeepSeek V4 Pro ⚠️EST"},
		{ChannelName: "kimi-direct", Model: "kimi-k2.6", InputPricePer1MCents: 60, OutputPricePer1MCents: 250, CachedInputPricePer1MCents: 15, Notes: "Kimi K2.6 ⚠️EST"},
		{ChannelName: "kimi-code", Model: "kimi-for-coding", InputPricePer1MCents: 60, OutputPricePer1MCents: 250, CachedInputPricePer1MCents: 15, Notes: "Kimi coding ⚠️EST"},
		{ChannelName: "qwen-plus-direct", Model: "qwen3.5-plus", InputPricePer1MCents: 40, OutputPricePer1MCents: 120, Notes: "Qwen3.5 Plus"},
		{ChannelName: "qwen-flash-direct", Model: "qwen3.5-flash", InputPricePer1MCents: 5, OutputPricePer1MCents: 40, Notes: "Qwen3.5 Flash ⚠️EST"},
		{ChannelName: "glm-flash-via-newapi", Model: "GLM-4.7-Flash", InputPricePer1MCents: 0, OutputPricePer1MCents: 0, Notes: "GLM Flash free"},
		{ChannelName: "siliconflow-qwen3-8b", Model: "Qwen/Qwen3-8B", InputPricePer1MCents: 0, OutputPricePer1MCents: 0, Notes: "SiliconFlow Free"},
		{ChannelName: "siliconflow-qwen25-7b", Model: "Qwen/Qwen2.5-7B-Instruct", InputPricePer1MCents: 0, OutputPricePer1MCents: 0, Notes: "SiliconFlow Free"},
		{ChannelName: "claude", Model: "claude-sonnet-4-6", InputPricePer1MCents: 300, OutputPricePer1MCents: 1500, CachedInputPricePer1MCents: 30, Notes: "Claude Sonnet 4.6"},
	}

	for _, pr := range defaults {
		if err := p.repo.Create(&pr); err != nil {
			return fmt.Errorf("seed pricing for %s/%s: %w", pr.ChannelName, pr.Model, err)
		}
	}
	return nil
}
