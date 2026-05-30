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

// Calc computes the cost in cents for a given channel, model, and usage.
// Falls back to zero cost if no pricing is configured.
func (p *Pricer) Calc(channelName, model string, usage Usage) (int, error) {
	pricing, err := p.repo.GetByChannelAndModel(channelName, model)
	if err != nil {
		return 0, fmt.Errorf("lookup pricing: %w", err)
	}
	if pricing == nil {
		return 0, nil // No pricing configured → free
	}

	// Prices are per 1M tokens in cents
	per1M := float64(1_000_000)

	cachedTokens := usage.CachedTokens
	if cachedTokens > usage.PromptTokens {
		cachedTokens = usage.PromptTokens
	}
	uncachedPromptTokens := usage.PromptTokens - cachedTokens

	inputCost := float64(uncachedPromptTokens) * float64(pricing.InputPricePer1MCents) / per1M
	cachedCost := float64(cachedTokens) * float64(pricing.GetCachedInputPrice()) / per1M
	outputCost := float64(usage.CompletionTokens) * float64(pricing.OutputPricePer1MCents) / per1M

	totalCents := int(inputCost + cachedCost + outputCost)
	if totalCents < 0 {
		totalCents = 0
	}
	return totalCents, nil
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
