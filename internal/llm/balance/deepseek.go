package balance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// deepSeekProvider fetches balance from DeepSeek's native API.
type deepSeekProvider struct {
	channelName string
	baseURL     string
	apiKey      string
	client      *http.Client
}

func newDeepSeekProvider(channelName, baseURL, apiKey, _ string) (Provider, error) {
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}
	return &deepSeekProvider{
		channelName: channelName,
		baseURL:     baseURL,
		apiKey:      apiKey,
		client:      &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (p *deepSeekProvider) Name() string {
	return "DeepSeek"
}

func (p *deepSeekProvider) Fetch(ctx context.Context) (*Info, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/user/balance", nil)
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
		IsAvailable  bool    `json:"is_available"`
		BalanceInfos []struct {
			Currency        string  `json:"currency"`
			TotalBalance    float64 `json:"total_balance"`
			GrantedBalance  float64 `json:"granted_balance"`
			ToppedUpBalance float64 `json:"topped_up_balance"`
		} `json:"balance_infos"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	info := &Info{
		ProviderType: "deepseek",
		ChannelName:  p.channelName,
		FetchedAt:    time.Now(),
	}

	if len(result.BalanceInfos) > 0 {
		bi := result.BalanceInfos[0]
		info.Currency = bi.Currency
		info.Balance = bi.TotalBalance
		info.TotalGranted = bi.GrantedBalance + bi.ToppedUpBalance
		info.TotalUsed = info.TotalGranted - info.Balance
	}
	if !result.IsAvailable {
		info.Error = "account balance unavailable"
	}

	return info, nil
}
