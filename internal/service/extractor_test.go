package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/singll/bellkeeper/internal/config"
)

// newFirecrawlMock 返回一个记录最后一次请求体的 firecrawl mock server。
func newFirecrawlMock(t *testing.T, captured *FirecrawlRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, captured); err != nil {
			t.Errorf("mock firecrawl: bad request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"markdown":"` +
			"# hello world this is sufficiently long body content for the extractor to accept it" +
			`","metadata":{"title":"T"}}}`))
	}))
}

func newFirecrawlExtractor(apiURL string, supportsActions bool) *ExtractorService {
	cfg := config.FileIngestionConfig{
		Firecrawl: config.FirecrawlConfig{
			Enabled:         true,
			APIURL:          apiURL,
			Timeout:         5,
			SupportsActions: supportsActions,
		},
	}
	return NewExtractorService(cfg, nil)
}

// TestFirecrawlDropsActionsWhenUnsupported 坐实 ① 修复：supports_actions=false 时，
// 即便 overrides 带 actions，发给 firecrawl 的请求体也不含 actions（避免 400
// SCRAPE_ACTIONS_NOT_SUPPORTED）。
func TestFirecrawlDropsActionsWhenUnsupported(t *testing.T) {
	var captured FirecrawlRequest
	srv := newFirecrawlMock(t, &captured)
	defer srv.Close()

	svc := newFirecrawlExtractor(srv.URL, false)
	_, err := svc.extractWithFirecrawl(&ExtractionRequest{
		URL: "http://example.com/a",
		Overrides: &RequestOverrides{
			FirecrawlActions: []FirecrawlAction{{Type: "click", Selector: ".consent-btn"}},
			FirecrawlWaitFor: 3000,
		},
	})
	if err != nil {
		t.Fatalf("extractWithFirecrawl error: %v", err)
	}
	if len(captured.Actions) != 0 {
		t.Fatalf("actions must be dropped when unsupported, got %d", len(captured.Actions))
	}
	// waitFor 等其它 override 仍应保留。
	if captured.WaitFor != 3000 {
		t.Fatalf("waitFor = %d, want 3000 (non-action overrides must survive)", captured.WaitFor)
	}
}

// TestFirecrawlKeepsActionsWhenSupported 断言 supports_actions=true 时 actions 正常下发，
// 保证「将来接 Fire Engine」的能力未被误删。
func TestFirecrawlKeepsActionsWhenSupported(t *testing.T) {
	var captured FirecrawlRequest
	srv := newFirecrawlMock(t, &captured)
	defer srv.Close()

	svc := newFirecrawlExtractor(srv.URL, true)
	_, err := svc.extractWithFirecrawl(&ExtractionRequest{
		URL: "http://example.com/a",
		Overrides: &RequestOverrides{
			FirecrawlActions: []FirecrawlAction{{Type: "click", Selector: ".consent-btn"}},
		},
	})
	if err != nil {
		t.Fatalf("extractWithFirecrawl error: %v", err)
	}
	if len(captured.Actions) != 1 || captured.Actions[0].Selector != ".consent-btn" {
		t.Fatalf("actions must be forwarded when supported, got %+v", captured.Actions)
	}
}
