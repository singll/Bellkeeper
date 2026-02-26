package service

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
)

type WebhookService struct {
	repo *repository.WebhookRepository
}

func NewWebhookService(repo *repository.WebhookRepository) *WebhookService {
	return &WebhookService{repo: repo}
}

func (s *WebhookService) List(page, perPage int) ([]model.WebhookConfig, int64, error) {
	return s.repo.List(page, perPage)
}

func (s *WebhookService) GetByID(id uint) (*model.WebhookConfig, error) {
	return s.repo.GetByID(id)
}

func (s *WebhookService) Create(webhook *model.WebhookConfig) error {
	return s.repo.Create(webhook)
}

func (s *WebhookService) Update(webhook *model.WebhookConfig) error {
	return s.repo.Update(webhook)
}

func (s *WebhookService) Delete(id uint) error {
	return s.repo.Delete(id)
}

func (s *WebhookService) GetHistory(webhookID uint, limit int) ([]model.WebhookHistory, error) {
	return s.repo.GetHistory(webhookID, limit)
}

func (s *WebhookService) Trigger(id uint, payload map[string]interface{}) (*model.WebhookHistory, error) {
	webhook, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	// Create history record
	history := &model.WebhookHistory{
		WebhookID:     id,
		RequestURL:    webhook.URL,
		RequestMethod: webhook.Method,
		Status:        "pending",
	}

	// Prepare request body
	var bodyBytes []byte
	if payload != nil {
		bodyBytes, _ = json.Marshal(payload)
		history.RequestBody = string(bodyBytes)
	} else if webhook.BodyTemplate != "" {
		bodyBytes = []byte(webhook.BodyTemplate)
		history.RequestBody = webhook.BodyTemplate
	}

	// Create history first
	if err := s.repo.CreateHistory(history); err != nil {
		return nil, err
	}

	// Make HTTP request
	start := time.Now()
	client := &http.Client{Timeout: time.Duration(webhook.TimeoutSeconds) * time.Second}

	req, err := http.NewRequest(webhook.Method, webhook.URL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		history.Status = "failed"
		history.ErrorMessage = err.Error()
		s.repo.UpdateHistory(history)
		return history, err
	}

	req.Header.Set("Content-Type", webhook.ContentType)

	// Add custom headers
	if webhook.Headers != nil {
		var headers map[string]string
		json.Unmarshal(webhook.Headers, &headers)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
	}

	resp, err := client.Do(req)
	history.DurationMs = int(time.Since(start).Milliseconds())

	if err != nil {
		history.Status = "failed"
		history.ErrorMessage = err.Error()
		s.repo.UpdateHistory(history)
		return history, err
	}
	defer resp.Body.Close()

	history.ResponseCode = resp.StatusCode
	respBody, _ := io.ReadAll(resp.Body)
	history.ResponseBody = string(respBody)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		history.Status = "success"
	} else {
		history.Status = "failed"
	}

	s.repo.UpdateHistory(history)
	return history, nil
}

// --- Batch D: 模板变量系统 ---

var templateVarRegex = regexp.MustCompile(`\{\{\s*(\w+)\s*\}\}`)

// processTemplate replaces {{variable}} placeholders in a string
func processTemplate(template string, variables map[string]string) string {
	return templateVarRegex.ReplaceAllStringFunc(template, func(match string) string {
		varName := strings.TrimSpace(match[2 : len(match)-2])
		if val, ok := variables[varName]; ok {
			return val
		}
		return match
	})
}

// processTemplateMap recursively processes template variables in a map
func processTemplateMap(data map[string]interface{}, variables map[string]string) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range data {
		switch val := v.(type) {
		case string:
			result[k] = processTemplate(val, variables)
		case map[string]interface{}:
			result[k] = processTemplateMap(val, variables)
		case []interface{}:
			result[k] = processTemplateSlice(val, variables)
		default:
			result[k] = v
		}
	}
	return result
}

func processTemplateSlice(data []interface{}, variables map[string]string) []interface{} {
	result := make([]interface{}, len(data))
	for i, v := range data {
		switch val := v.(type) {
		case string:
			result[i] = processTemplate(val, variables)
		case map[string]interface{}:
			result[i] = processTemplateMap(val, variables)
		case []interface{}:
			result[i] = processTemplateSlice(val, variables)
		default:
			result[i] = v
		}
	}
	return result
}

// buildVariables constructs the template variables map with built-in + custom variables
func buildVariables(webhook *model.WebhookConfig, payload map[string]interface{}, customVars map[string]string) map[string]string {
	now := time.Now()
	vars := map[string]string{
		"timestamp":    now.Format(time.RFC3339),
		"date":         now.Format("2006-01-02"),
		"webhook_name": webhook.Name,
	}

	// Extract article_url from payload if present
	if payload != nil {
		if url, ok := payload["article_url"].(string); ok {
			vars["article_url"] = url
		}
	}

	// Merge custom variables (override built-in ones)
	for k, v := range customVars {
		vars[k] = v
	}

	return vars
}

// TriggerWithVariables triggers a webhook with template variable support
func (s *WebhookService) TriggerWithVariables(id uint, payload map[string]interface{}, customVars map[string]string) (*model.WebhookHistory, error) {
	webhook, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	variables := buildVariables(webhook, payload, customVars)

	// Create history record
	history := &model.WebhookHistory{
		WebhookID:     id,
		RequestURL:    processTemplate(webhook.URL, variables),
		RequestMethod: webhook.Method,
		Status:        "pending",
	}

	// Prepare request body with template processing
	var bodyBytes []byte
	if payload != nil {
		processed := processTemplateMap(payload, variables)
		bodyBytes, _ = json.Marshal(processed)
		history.RequestBody = string(bodyBytes)
	} else if webhook.BodyTemplate != "" {
		processed := processTemplate(webhook.BodyTemplate, variables)
		bodyBytes = []byte(processed)
		history.RequestBody = processed
	}

	if err := s.repo.CreateHistory(history); err != nil {
		return nil, err
	}

	start := time.Now()
	client := &http.Client{Timeout: time.Duration(webhook.TimeoutSeconds) * time.Second}

	req, err := http.NewRequest(webhook.Method, history.RequestURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		history.Status = "failed"
		history.ErrorMessage = err.Error()
		s.repo.UpdateHistory(history)
		return history, err
	}

	req.Header.Set("Content-Type", webhook.ContentType)

	// Add custom headers with template processing
	if webhook.Headers != nil {
		var headers map[string]string
		json.Unmarshal(webhook.Headers, &headers)
		for k, v := range headers {
			req.Header.Set(k, processTemplate(v, variables))
		}
	}

	resp, err := client.Do(req)
	history.DurationMs = int(time.Since(start).Milliseconds())

	if err != nil {
		history.Status = "failed"
		history.ErrorMessage = err.Error()
		s.repo.UpdateHistory(history)
		return history, err
	}
	defer resp.Body.Close()

	history.ResponseCode = resp.StatusCode
	respBody, _ := io.ReadAll(resp.Body)
	history.ResponseBody = string(respBody)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		history.Status = "success"
	} else {
		history.Status = "failed"
	}

	s.repo.UpdateHistory(history)
	return history, nil
}

// GetHistoryByStatus returns webhook history filtered by status
func (s *WebhookService) GetHistoryByStatus(webhookID uint, status string, limit int) ([]model.WebhookHistory, error) {
	return s.repo.GetHistoryByStatus(webhookID, status, limit)
}
