package service

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

	defaults := []model.LLMModelPricing{
		{ChannelName: "deepseek", Model: "deepseek-chat", InputPricePer1MCents: 14, OutputPricePer1MCents: 28, CachedInputPricePer1MCents: 1, Notes: "DeepSeek V3"},
		{ChannelName: "qwen", Model: "qwen3.5-plus", InputPricePer1MCents: 40, OutputPricePer1MCents: 120, Notes: "Qwen3.5 Plus"},
		{ChannelName: "claude", Model: "claude-sonnet-4-6", InputPricePer1MCents: 300, OutputPricePer1MCents: 1500, CachedInputPricePer1MCents: 30, Notes: "Claude Sonnet 4.6"},
		{ChannelName: "siliconflow", Model: "Qwen3-8B", InputPricePer1MCents: 0, OutputPricePer1MCents: 0, Notes: "SiliconFlow Free"},
		{ChannelName: "siliconflow", Model: "Qwen2.5-7B-Instruct", InputPricePer1MCents: 0, OutputPricePer1MCents: 0, Notes: "SiliconFlow Free"},
	}

	for _, pr := range defaults {
		if err := p.repo.Create(&pr); err != nil {
			return fmt.Errorf("seed pricing for %s/%s: %w", pr.ChannelName, pr.Model, err)
		}
	}
	return nil
}
