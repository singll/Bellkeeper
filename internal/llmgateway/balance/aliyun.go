package balance

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/singll/bellkeeper/internal/middleware"
	"go.uber.org/zap"
)

// aliyunProvider fetches balance via Alibaba Cloud BSS OpenAPI.
// Uses AccessKey + AccessSecret for signature (not Bearer token).
// For simplicity, this implementation uses a direct HTTP call with pre-signed URL
// or basic auth if the user provides an access_token in extraConfig.
type aliyunProvider struct {
	channelName string
	accessKey   string
	secret      string
	client      *http.Client
}

func newAliyunProvider(channelName, _, accessKey, extraConfig string) (Provider, error) {
	// extraConfig expected as JSON: {"access_secret":"xxx"}
	var cfg struct {
		AccessSecret string `json:"access_secret"`
	}
	if extraConfig != "" {
		_ = json.Unmarshal([]byte(extraConfig), &cfg)
	}
	return &aliyunProvider{
		channelName: channelName,
		accessKey:   accessKey,
		secret:      cfg.AccessSecret,
		client:      &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func (p *aliyunProvider) Name() string {
	return "AliyunBSS"
}

func (p *aliyunProvider) Fetch(ctx context.Context) (*Info, error) {
	apiURL := "https://bssopenapi.aliyuncs.com"
	params := p.buildSignedParams()

	fullURL := apiURL + "/?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

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
			AvailableAmount     float64 `json:"AvailableAmount"`
			AvailableCashAmount float64 `json:"AvailableCashAmount"`
			CreditLimit         float64 `json:"CreditLimit"`
			Currency            string  `json:"Currency"`
		} `json:"Data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		middleware.GetLogger().Warn("aliyun balance parse failed", zap.Error(err))
		return &Info{
			ProviderType: "aliyun",
			ChannelName:  p.channelName,
			Balance:      0,
			Currency:     "CNY",
			FetchedAt:    time.Now(),
			Error:        "parse failed: " + err.Error(),
		}, nil
	}

	return &Info{
		ProviderType: "aliyun",
		ChannelName:  p.channelName,
		Balance:      result.Data.AvailableAmount,
		Currency:     result.Data.Currency,
		TotalGranted: result.Data.AvailableAmount + result.Data.CreditLimit,
		TotalUsed:    result.Data.CreditLimit,
		FetchedAt:    time.Now(),
	}, nil
}

// buildSignedParams constructs query parameters with HMAC-SHA1 signature
// as required by Alibaba Cloud OpenAPI specification.
func (p *aliyunProvider) buildSignedParams() url.Values {
	params := url.Values{}
	params.Set("Action", "QueryAccountBalance")
	params.Set("Format", "JSON")
	params.Set("Version", "2017-12-14")
	params.Set("AccessKeyId", p.accessKey)
	params.Set("Timestamp", time.Now().UTC().Format("2006-01-02T15:04:05Z"))
	params.Set("SignatureMethod", "HMAC-SHA1")
	params.Set("SignatureVersion", "1.0")
	params.Set("SignatureNonce", fmt.Sprintf("%d", time.Now().UnixNano()))

	// Build canonicalized query string (sorted alphabetically)
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var canonicalParts []string
	for _, k := range keys {
		canonicalParts = append(canonicalParts,
			percentEncode(k)+"="+percentEncode(params.Get(k)))
	}
	canonicalQuery := strings.Join(canonicalParts, "&")

	// String to sign: HTTPMethod + "&" + percentEncode("/") + "&" + percentEncode(canonicalQuery)
	stringToSign := "GET&" + percentEncode("/") + "&" + percentEncode(canonicalQuery)

	// HMAC-SHA1 with key = AccessSecret + "&"
	mac := hmac.New(sha1.New, []byte(p.secret+"&"))
	mac.Write([]byte(stringToSign))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	params.Set("Signature", signature)
	return params
}

// percentEncode performs the special URL encoding required by Alibaba Cloud signature.
// It differs from url.QueryEscape: spaces encode as %20 (not +), and ~ is kept unencoded.
func percentEncode(s string) string {
	encoded := url.QueryEscape(s)
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	encoded = strings.ReplaceAll(encoded, "*", "%2A")
	encoded = strings.ReplaceAll(encoded, "%7E", "~")
	return encoded
}
