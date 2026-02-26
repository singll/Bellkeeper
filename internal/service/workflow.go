package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/singll/bellkeeper/internal/config"
)

type WorkflowService struct {
	cfg    config.N8NConfig
	client *http.Client
}

func NewWorkflowService(cfg config.N8NConfig) *WorkflowService {
	return &WorkflowService{
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

type WorkflowStatus struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Active    bool                   `json:"active"`
	CreatedAt string                 `json:"created_at,omitempty"`
	UpdatedAt string                 `json:"updated_at,omitempty"`
	Tags      []WorkflowTag          `json:"tags,omitempty"`
	Meta      map[string]interface{} `json:"meta,omitempty"`
}

type WorkflowTag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type WorkflowExecution struct {
	ID         string `json:"id"`
	WorkflowID string `json:"workflow_id"`
	Finished   bool   `json:"finished"`
	Status     string `json:"status"` // success, error, waiting
	StartedAt  string `json:"started_at"`
	StoppedAt  string `json:"stopped_at,omitempty"`
}

// n8nWorkflow represents the workflow response from n8n API
type n8nWorkflow struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Active    bool                   `json:"active"`
	CreatedAt string                 `json:"createdAt"`
	UpdatedAt string                 `json:"updatedAt"`
	Tags      []n8nTag               `json:"tags"`
	Meta      map[string]interface{} `json:"meta"`
}

type n8nTag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type n8nWorkflowsResponse struct {
	Data       []n8nWorkflow `json:"data"`
	NextCursor string        `json:"nextCursor"`
}

type n8nExecutionsResponse struct {
	Data       []n8nExecution `json:"data"`
	NextCursor string         `json:"nextCursor"`
}

type n8nExecution struct {
	ID         string `json:"id"`
	WorkflowID string `json:"workflowId"`
	Finished   bool   `json:"finished"`
	Status     string `json:"status"`
	StartedAt  string `json:"startedAt"`
	StoppedAt  string `json:"stoppedAt"`
}

// Status retrieves the list of workflows from n8n
func (s *WorkflowService) Status() ([]WorkflowStatus, error) {
	if s.cfg.APIKey == "" {
		return []WorkflowStatus{}, nil
	}

	req, err := http.NewRequest("GET", s.cfg.APIBaseURL+"/workflows", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-N8N-API-KEY", s.cfg.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to n8n: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("n8n returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var n8nResp n8nWorkflowsResponse
	if err := json.Unmarshal(body, &n8nResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	workflows := make([]WorkflowStatus, len(n8nResp.Data))
	for i, wf := range n8nResp.Data {
		tags := make([]WorkflowTag, len(wf.Tags))
		for j, t := range wf.Tags {
			tags[j] = WorkflowTag{ID: t.ID, Name: t.Name}
		}
		workflows[i] = WorkflowStatus{
			ID:        wf.ID,
			Name:      wf.Name,
			Active:    wf.Active,
			CreatedAt: wf.CreatedAt,
			UpdatedAt: wf.UpdatedAt,
			Tags:      tags,
			Meta:      wf.Meta,
		}
	}

	return workflows, nil
}

// GetWorkflow retrieves a single workflow by ID
func (s *WorkflowService) GetWorkflow(id string) (*WorkflowStatus, error) {
	if s.cfg.APIKey == "" {
		return nil, fmt.Errorf("n8n API key not configured")
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/workflows/%s", s.cfg.APIBaseURL, id), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-N8N-API-KEY", s.cfg.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get workflow: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var wf n8nWorkflow
	if err := json.Unmarshal(body, &wf); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	tags := make([]WorkflowTag, len(wf.Tags))
	for i, t := range wf.Tags {
		tags[i] = WorkflowTag{ID: t.ID, Name: t.Name}
	}

	return &WorkflowStatus{
		ID:        wf.ID,
		Name:      wf.Name,
		Active:    wf.Active,
		CreatedAt: wf.CreatedAt,
		UpdatedAt: wf.UpdatedAt,
		Tags:      tags,
		Meta:      wf.Meta,
	}, nil
}

// ActivateWorkflow activates a workflow
func (s *WorkflowService) ActivateWorkflow(id string) error {
	if s.cfg.APIKey == "" {
		return fmt.Errorf("n8n API key not configured")
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/workflows/%s/activate", s.cfg.APIBaseURL, id), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-N8N-API-KEY", s.cfg.APIKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to activate workflow: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to activate workflow: HTTP %d", resp.StatusCode)
	}

	return nil
}

// DeactivateWorkflow deactivates a workflow
func (s *WorkflowService) DeactivateWorkflow(id string) error {
	if s.cfg.APIKey == "" {
		return fmt.Errorf("n8n API key not configured")
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/workflows/%s/deactivate", s.cfg.APIBaseURL, id), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-N8N-API-KEY", s.cfg.APIKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to deactivate workflow: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to deactivate workflow: HTTP %d", resp.StatusCode)
	}

	return nil
}

// GetExecutions retrieves workflow executions
func (s *WorkflowService) GetExecutions(workflowID string, limit int) ([]WorkflowExecution, error) {
	if s.cfg.APIKey == "" {
		return nil, fmt.Errorf("n8n API key not configured")
	}

	url := fmt.Sprintf("%s/executions?limit=%d", s.cfg.APIBaseURL, limit)
	if workflowID != "" {
		url += fmt.Sprintf("&workflowId=%s", workflowID)
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-N8N-API-KEY", s.cfg.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get executions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get executions: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var n8nResp n8nExecutionsResponse
	if err := json.Unmarshal(body, &n8nResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	executions := make([]WorkflowExecution, len(n8nResp.Data))
	for i, ex := range n8nResp.Data {
		executions[i] = WorkflowExecution{
			ID:         ex.ID,
			WorkflowID: ex.WorkflowID,
			Finished:   ex.Finished,
			Status:     ex.Status,
			StartedAt:  ex.StartedAt,
			StoppedAt:  ex.StoppedAt,
		}
	}

	return executions, nil
}

// Trigger triggers a workflow via webhook
func (s *WorkflowService) Trigger(name string, payload map[string]interface{}) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/webhook/%s", s.cfg.WebhookBaseURL, name)

	var body []byte
	if payload != nil {
		body, _ = json.Marshal(payload)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result map[string]interface{}
	json.Unmarshal(respBody, &result)

	if resp.StatusCode >= 400 {
		return result, fmt.Errorf("trigger failed with status %d", resp.StatusCode)
	}

	return result, nil
}

