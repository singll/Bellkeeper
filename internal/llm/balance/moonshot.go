package balance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// moonshotProvider fetches balance from Moonshot (Kimi Open Platform) native API.
// Note: this is for api.moonshot.cn, NOT api.kimi.com (Kimi Code subscription).
type moonshotProvider struct {
	channelName string
	baseURL     string
	apiKey      string
	client      *http.Client
}

func newMoonshotProvider(channelName, baseURL, apiKey, _ string) (Provider, error) {
	if baseURL == "" {
		baseURL = "https://api.moonshot.cn"
	}
	return &moonshotProvider{
		channelName: channelName,
		baseURL:     baseURL,
		apiKey:      apiKey,
		client:      &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (p *moonshotProvider) Name() string {
	return "Moonshot"
}

func (p *moonshotProvider) Fetch(ctx context.Context) (*Info, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/v1/users/me/balance", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch balance: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result struct {
		Data struct {
			AvailableBalance float64 `json:"available_balance"`
			VoucherBalance   float64 `json:"voucher_balance"`
			CashBalance      float64 `json:"cash_balance"`
			Currency         string  `json:"currency"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &Info{
		ProviderType: "moonshot",
		ChannelName:  p.channelName,
		Balance:      result.Data.AvailableBalance,
		Currency:     result.Data.Currency,
		TotalGranted: result.Data.CashBalance + result.Data.VoucherBalance,
		TotalUsed:    (result.Data.CashBalance + result.Data.VoucherBalance) - result.Data.AvailableBalance,
		FetchedAt:    time.Now(),
	}, nil
}
