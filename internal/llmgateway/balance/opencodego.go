package balance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// openCodeGoProvider fetches real quota usage from OpenCode Go's official
// usage API (GET {base}/v1/usage): rolling 5h / weekly / monthly windows with
// percent-used and reset timestamps. Unlike dollar-balance providers this is
// window-quota telemetry: Balance carries the *remaining fraction* (0..1) of
// the most constraining window so balance_aware routing and downstream supply
// watchers can throttle on real quota instead of Bellkeeper's local bucket.
type openCodeGoProvider struct {
	channelName string
	baseURL     string
	apiKey      string
	client      *http.Client
}

func newOpenCodeGoProvider(channelName, baseURL, apiKey, _ string) (Provider, error) {
	if baseURL == "" {
		baseURL = "https://opencode.ai/zen/go"
	}
	return &openCodeGoProvider{
		channelName: channelName,
		baseURL:     baseURL,
		apiKey:      apiKey,
		client:      &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (p *openCodeGoProvider) Name() string {
	return "OpenCode Go"
}

type ocGoWindow struct {
	Status   string  `json:"status"`
	Percent  float64 `json:"percent"` // 已用百分比
	ResetsAt string  `json:"resetsAt"`
}

func (p *openCodeGoProvider) Fetch(ctx context.Context) (*Info, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/v1/usage", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch usage: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var result struct {
		Usage struct {
			Rolling ocGoWindow `json:"rolling"`
			Weekly  ocGoWindow `json:"weekly"`
			Monthly ocGoWindow `json:"monthly"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode usage: %w", err)
	}

	// 最紧窗口决定可用性：任一窗口打满都不可用
	windows := []ocGoWindow{result.Usage.Rolling, result.Usage.Weekly, result.Usage.Monthly}
	worst := 0.0
	var soonestReset *time.Time
	for _, w := range windows {
		if w.Percent > worst {
			worst = w.Percent
		}
		if t, err := time.Parse(time.RFC3339, w.ResetsAt); err == nil {
			if soonestReset == nil || t.Before(*soonestReset) {
				soonestReset = &t
			}
		}
	}
	remaining := 1 - worst/100
	if remaining < 0 {
		remaining = 0
	}

	return &Info{
		ProviderType: "opencodego",
		ChannelName:  p.channelName,
		Balance:      remaining, // 窗口剩余比例（0..1），非美元
		Currency:     "window_ratio",
		TotalGranted: 1,
		TotalUsed:    worst / 100,
		ExpiresAt:    soonestReset,
		FetchedAt:    time.Now(),
	}, nil
}
