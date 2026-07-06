package balance

import (
	"context"
	"time"
)

// Info holds the result of a balance fetch.
type Info struct {
	ProviderType string  `json:"provider_type"` // e.g. "deepseek", "moonshot", "newapi", "aliyun"
	ChannelName  string  `json:"channel_name"`
	Balance      float64 `json:"balance"`       // in USD (or equivalent)
	Currency     string  `json:"currency"`
	TotalGranted float64 `json:"total_granted"` // total quota ever granted
	TotalUsed    float64 `json:"total_used"`    // total consumed
	ExpiresAt    *time.Time `json:"expires_at"`
	FetchedAt    time.Time  `json:"fetched_at"`
	Error        string     `json:"error,omitempty"`
}

// Provider defines how to fetch real balance from an upstream.
type Provider interface {
	// Fetch queries the upstream for current balance.
	Fetch(ctx context.Context) (*Info, error)
	// Name returns a human-readable provider name.
	Name() string
}

// Factory creates a Provider for a given channel configuration.
type Factory struct {
	// map of provider_type -> constructor
	constructors map[string]func(channelName, baseURL, apiKey, extraConfig string) (Provider, error)
}

// NewFactory creates the default factory with all built-in providers.
func NewFactory() *Factory {
	return &Factory{
		constructors: map[string]func(string, string, string, string) (Provider, error){
			"deepseek":  newDeepSeekProvider,
			"moonshot":  newMoonshotProvider,
			"newapi":    newNewAPIProvider,
			"aliyun":    newAliyunProvider,
		},
	}
}

// Create instantiates a Provider for the given channel configuration.
func (f *Factory) Create(providerType, channelName, baseURL, apiKey, extraConfig string) (Provider, error) {
	ctor, ok := f.constructors[providerType]
	if !ok {
		return nil, nil // unsupported provider type
	}
	return ctor(channelName, baseURL, apiKey, extraConfig)
}

// SupportedTypes returns all registered provider type names.
func (f *Factory) SupportedTypes() []string {
	types := make([]string, 0, len(f.constructors))
	for t := range f.constructors {
		types = append(types, t)
	}
	return types
}
