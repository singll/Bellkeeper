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
	Name      string `json:"name"`
	Active    bool   `json:"active"`
	LastRun   string `json:"last_run,omitempty"`
	RunCount  int    `json:"run_count,omitempty"`
	ErrorRate string `json:"error_rate,omitempty"`
}

func (s *WorkflowService) Status() ([]WorkflowStatus, error) {
	// This would typically call n8n's API to get workflow status
	// For now, return a placeholder
	return []WorkflowStatus{
		{Name: "scheduled-fetch", Active: true},
		{Name: "core-crawler", Active: true},
		{Name: "manual-ingest", Active: true},
	}, nil
}

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
