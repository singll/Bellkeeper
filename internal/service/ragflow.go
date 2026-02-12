package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
)

type RagFlowService struct {
	cfg         config.RagFlowConfig
	datasetRepo *repository.DatasetMappingRepository
	tagRepo     *repository.TagRepository
	client      *http.Client
}

func NewRagFlowService(cfg config.RagFlowConfig, datasetRepo *repository.DatasetMappingRepository, tagRepo *repository.TagRepository) *RagFlowService {
	return &RagFlowService{
		cfg:         cfg,
		datasetRepo: datasetRepo,
		tagRepo:     tagRepo,
		client:      &http.Client{Timeout: time.Duration(cfg.Timeout) * time.Second},
	}
}

type UploadRequest struct {
	Content        string   `json:"content"`
	Filename       string   `json:"filename"`
	Title          string   `json:"title"`
	URL            string   `json:"url"`
	Tags           []string `json:"tags"`
	Category       string   `json:"category"`
	DatasetID      string   `json:"dataset_id"`
	AutoCreateTags bool     `json:"auto_create_tags"`
}

type UploadResponse struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data"`
}

// Upload uploads a document to RagFlow
func (s *RagFlowService) Upload(req *UploadRequest) (*UploadResponse, error) {
	datasetID := req.DatasetID
	if datasetID == "" {
		// Use default dataset
		defaultMapping, err := s.datasetRepo.GetDefault()
		if err != nil {
			return nil, fmt.Errorf("no dataset specified and no default found: %w", err)
		}
		datasetID = defaultMapping.DatasetID
	}

	// Upload to RagFlow
	return s.uploadToRagFlow(datasetID, req.Filename, req.Content)
}

// UploadWithRouting uploads with intelligent dataset routing based on tags/category
func (s *RagFlowService) UploadWithRouting(req *UploadRequest) (*UploadResponse, string, error) {
	var datasetID string

	// 1. Try to find dataset by tags
	if len(req.Tags) > 0 && req.AutoCreateTags {
		// Get or create tags
		var tagIDs []uint
		for _, tagName := range req.Tags {
			tag, err := s.tagRepo.GetByName(tagName)
			if err != nil {
				// Create new tag
				tag = &model.Tag{Name: tagName, Color: "#409EFF"}
				s.tagRepo.Create(tag)
			}
			tagIDs = append(tagIDs, tag.ID)
		}

		// Find matching dataset
		if len(tagIDs) > 0 {
			mappings, err := s.datasetRepo.GetByTagIDs(tagIDs)
			if err == nil && len(mappings) > 0 {
				datasetID = mappings[0].DatasetID
			}
		}
	}

	// 2. Try to find dataset by category
	if datasetID == "" && req.Category != "" {
		mapping, err := s.datasetRepo.GetByName(req.Category)
		if err == nil {
			datasetID = mapping.DatasetID
		}
	}

	// 3. Use default dataset
	if datasetID == "" {
		defaultMapping, err := s.datasetRepo.GetDefault()
		if err != nil {
			return nil, "", fmt.Errorf("no matching dataset found and no default configured")
		}
		datasetID = defaultMapping.DatasetID
	}

	// Upload to RagFlow
	resp, err := s.uploadToRagFlow(datasetID, req.Filename, req.Content)
	if err != nil {
		return nil, datasetID, err
	}

	// Save article-tag associations
	if resp.Code == 0 && resp.Data != nil {
		if docID, ok := resp.Data["id"].(string); ok {
			for _, tagName := range req.Tags {
				tag, _ := s.tagRepo.GetByName(tagName)
				if tag != nil {
					s.datasetRepo.CreateArticleTag(&model.ArticleTag{
						DocumentID:   docID,
						DatasetID:    datasetID,
						TagID:        tag.ID,
						ArticleTitle: req.Title,
						ArticleURL:   req.URL,
					})
				}
			}
		}
	}

	return resp, datasetID, nil
}

// CheckURL checks if a URL has been uploaded before
func (s *RagFlowService) CheckURL(url string) (bool, error) {
	return s.datasetRepo.ArticleURLExists(url)
}

func (s *RagFlowService) uploadToRagFlow(datasetID, filename, content string) (*UploadResponse, error) {
	url := fmt.Sprintf("%s/api/v1/datasets/%s/documents", s.cfg.BaseURL, datasetID)

	payload := map[string]interface{}{
		"name": filename,
		"text": content,
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result UploadResponse
	json.Unmarshal(respBody, &result)

	return &result, nil
}

// ListDocuments lists documents in a dataset
func (s *RagFlowService) ListDocuments(datasetID string, page, limit int) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/datasets/%s/documents?page=%d&limit=%d", s.cfg.BaseURL, datasetID, page, limit)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	json.Unmarshal(body, &result)

	return result, nil
}

// DeleteDocument deletes a document from RagFlow
func (s *RagFlowService) DeleteDocument(datasetID, documentID string) error {
	url := fmt.Sprintf("%s/api/v1/datasets/%s/documents/%s", s.cfg.BaseURL, datasetID, documentID)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete failed: %s", string(body))
	}

	return nil
}
