package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/singll/bellkeeper/internal/pkg/httpclient"
	"github.com/singll/bellkeeper/internal/repository"
	"github.com/singll/bellkeeper/internal/service"
)

type healthCheckerAdapter struct {
	svc *service.HealthService
}

func (a healthCheckerAdapter) Detailed() interface{} {
	return a.svc.Detailed()
}

type dashboardStatserAdapter struct {
	svc *service.DashboardService
}

func (a dashboardStatserAdapter) Stats() (interface{}, error) {
	return a.svc.Stats()
}

type knowledgeSearcherAdapter struct {
	svc *service.FileSearchService
}

func (a knowledgeSearcherAdapter) SearchKnowledge(ctx context.Context, query string, layers []string, limit int) (interface{}, error) {
	return a.svc.Search(ctx, service.FileSearchRequest{
		Query:  query,
		Layers: layers,
		Limit:  limit,
	})
}

type knowledgeAskerAdapter struct {
	svc *service.AskService
}

func (a knowledgeAskerAdapter) AskKnowledge(ctx context.Context, question string, layers []string) (interface{}, error) {
	return a.svc.Ask(ctx, service.AskRequest{
		Question: question,
		Layers:   layers,
	})
}

type llmUsageQuerierAdapter struct {
	repo *repository.LLMProxyRepository
}

func (a llmUsageQuerierAdapter) LLMUsageSince(since time.Time) (interface{}, error) {
	return a.repo.SummarySince(since)
}

type crawlStatserAdapter struct {
	svc *service.CrawlQueueService
}

func (a crawlStatserAdapter) CrawlStats() (interface{}, error) {
	return a.svc.Stats()
}

func BuildToolDependencies(
	healthSvc *service.HealthService,
	dashboardSvc *service.DashboardService,
	searchSvc *service.FileSearchService,
	askSvc *service.AskService,
	llmProxyRepo *repository.LLMProxyRepository,
	crawlQueueSvc *service.CrawlQueueService,
) ToolDependencies {
	deps := ToolDependencies{
		HealthChecker: healthCheckerAdapter{svc: healthSvc},
		DashboardSvc:  dashboardStatserAdapter{svc: dashboardSvc},
		SearchSvc:     knowledgeSearcherAdapter{svc: searchSvc},
		AskSvc:        knowledgeAskerAdapter{svc: askSvc},
		LLMUsageSvc:   llmUsageQuerierAdapter{repo: llmProxyRepo},
	}
	if crawlQueueSvc != nil {
		deps.CrawlSvc = crawlStatserAdapter{svc: crawlQueueSvc}
	}
	return deps
}

type memosAdapter struct {
	baseURL    string
	apiToken   string
	httpClient *httpclient.Client
}

func (m memosAdapter) ListTodos(ctx context.Context) (interface{}, error) {
	url := m.baseURL + "/api/v1/memos"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+m.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("memos list: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("memos API error (HTTP %d): %s", resp.StatusCode, string(body))
	}
	var result interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode memos response: %w", err)
	}
	return result, nil
}

func (m memosAdapter) AddTodo(ctx context.Context, content string) (interface{}, error) {
	url := m.baseURL + "/api/v1/memos"
	payload := map[string]interface{}{
		"content":    content,
		"visibility": "PRIVATE",
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+m.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("memos add: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("memos API error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	var result interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode memos response: %w", err)
	}
	return result, nil
}

func (m memosAdapter) CompleteTodo(ctx context.Context, id int) error {
	url := fmt.Sprintf("%s/api/v1/memos/%d", m.baseURL, id)
	payload := map[string]interface{}{"done": true}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "PATCH", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("memos complete: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("memos API error (HTTP %d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}

type workflowAdapter struct {
	svc *service.WorkflowService
}

func (w workflowAdapter) TriggerWorkflow(ctx context.Context, name string, payload map[string]interface{}) (interface{}, error) {
	return w.svc.Trigger(name, payload)
}

func BuildWriteToolDependencies(
	memosBaseURL string,
	memosAPIToken string,
	workflowSvc *service.WorkflowService,
) WriteToolDependencies {
	deps := WriteToolDependencies{}
	if memosBaseURL != "" && memosAPIToken != "" {
		deps.TodoMgr = memosAdapter{
			baseURL:    memosBaseURL,
			apiToken:   memosAPIToken,
			httpClient: httpclient.NewClientWithTimeout(30 * time.Second),
		}
	}
	if workflowSvc != nil {
		deps.Workflow = workflowAdapter{svc: workflowSvc}
	}
	return deps
}
