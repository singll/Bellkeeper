package balance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// newAPIProvider fetches balance from new-api compatible sites (anyrouter, etc.).
// Requires session-based auth (not sk-key).
type newAPIProvider struct {
	channelName string
	baseURL     string
	userInfoPath string
	apiUserKey  string
	client      *http.Client
}

func newNewAPIProvider(channelName, baseURL, apiUserKey, extraConfig string) (Provider, error) {
	// extraConfig expected as JSON: {"user_info_path":"/api/user/self"}
	path := "/api/user/self"
	if extraConfig != "" {
		var cfg struct {
			UserInfoPath string `json:"user_info_path"`
		}
		if err := json.Unmarshal([]byte(extraConfig), &cfg); err == nil && cfg.UserInfoPath != "" {
			path = cfg.UserInfoPath
		}
	}
	return &newAPIProvider{
		channelName:  channelName,
		baseURL:      baseURL,
		userInfoPath: path,
		apiUserKey:   apiUserKey,
		client:       &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (p *newAPIProvider) Name() string {
	return "NewAPI"
}

func (p *newAPIProvider) Fetch(ctx context.Context) (*Info, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+p.userInfoPath, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("new-api-user", p.apiUserKey)
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch balance: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result struct {
		Data struct {
			Quota      int64   `json:"quota"`
			UsedQuota  int64   `json:"used_quota"`
			RemainQuota int64  `json:"remain_quota"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// new-api quota is in "points", typically 1 point = $0.000002 (1/500000 USD)
	const pointToUSD = 1.0 / 500000.0

	return &Info{
		ProviderType: "newapi",
		ChannelName:  p.channelName,
		Balance:      float64(result.Data.RemainQuota) * pointToUSD,
		Currency:     "USD",
		TotalGranted: float64(result.Data.Quota) * pointToUSD,
		TotalUsed:    float64(result.Data.UsedQuota) * pointToUSD,
		FetchedAt:    time.Now(),
	}, nil
}
