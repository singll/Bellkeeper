package service

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
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
