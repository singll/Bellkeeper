package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"sync"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/defaults"
	"github.com/singll/bellkeeper/internal/pkg/httpclient"
	"github.com/singll/bellkeeper/internal/repository"
	"go.uber.org/zap"
)

// RagFlowService handles all RagFlow interactions
type RagFlowService struct {
	cfg         config.RagFlowConfig
	datasetRepo *repository.DatasetMappingRepository
	tagRepo     *repository.TagRepository
	client      *httpclient.Client
	activityLog *ActivityLogService
	parseTasks  sync.Map // taskID -> *ParseTask
}

// NewRagFlowService creates a new RagFlowService
func NewRagFlowService(cfg config.RagFlowConfig, datasetRepo *repository.DatasetMappingRepository, tagRepo *repository.TagRepository, activityLog *ActivityLogService) *RagFlowService {
	return &RagFlowService{
		cfg:         cfg,
		datasetRepo: datasetRepo,
		tagRepo:     tagRepo,
		client:      httpclient.NewClientWithTimeout(time.Duration(cfg.Timeout) * time.Second),
		activityLog: activityLog,
	}
}

type UploadRequest struct {
	Content        string   `json:"content"`
	Filename       string   `json:"filename"`
	Title         string   `json:"title"`
	URL           string   `json:"url"`
	Tags          []string `json:"tags"`
	Category      string   `json:"category"`
	DatasetID     string   `json:"dataset_id"`
	AutoCreateTags bool    `json:"auto_create_tags"`
}

type UploadResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// Upload uploads a document to RagFlow
func (s *RagFlowService) Upload(req *UploadRequest) (*UploadResponse, error) {
	start := time.Now()
	datasetID := req.DatasetID
	if datasetID == "" {
		defaultMapping, err := s.datasetRepo.GetDefault()
		if err != nil {
			return nil, fmt.Errorf("no dataset specified and no default found: %w", err)
		}
		datasetID = defaultMapping.DatasetID
	}

	resp, err := s.uploadToRagFlow(datasetID, req.Filename, req.Content)
	if s.activityLog != nil {
		status, summary := "success", fmt.Sprintf("上传 %s 到 %s", req.Filename, datasetID)
		if err != nil {
			status, summary = "error", fmt.Sprintf("上传 %s 失败: %v", req.Filename, err)
		}
		s.activityLog.LogActivity(LogActivityParams{
			Module: "ragflow_upload", Action: "upload", Status: status,
			Summary: summary, RefID: datasetID, DurationMs: int(time.Since(start).Milliseconds()),
		})
	}
	return resp, err
}

// UploadWithRouting uploads with intelligent dataset routing based on tags/category
func (s *RagFlowService) UploadWithRouting(req *UploadRequest) (*UploadResponse, string, error) {
	start := time.Now()
	var datasetID string

	// 1. Use LLM-recommended dataset name (lookup by name to get real RAGFlow ID)
	if req.DatasetID != "" {
		mapping, err := s.datasetRepo.GetByName(req.DatasetID)
		if err == nil && mapping.DatasetID != "" {
			datasetID = mapping.DatasetID
			middleware.GetLogger().Info("dataset routed by LLM recommendation",
			zap.String("dataset_name", req.DatasetID), zap.String("dataset_id", datasetID))
		}
	}

	// 2. Try to find dataset by tags
	if datasetID == "" && len(req.Tags) > 0 && req.AutoCreateTags {
		var tagIDs []uint
		for _, tagName := range req.Tags {
			tag, err := s.tagRepo.FindOrCreate(tagName, defaults.DefaultTagColor)
			if err != nil {
				return nil, "", fmt.Errorf("failed to get or create tag %q: %w", tagName, err)
			}
			tagIDs = append(tagIDs, tag.ID)
		}

		if len(tagIDs) > 0 {
			mappings, err := s.datasetRepo.GetByTagIDs(tagIDs)
			if err == nil && len(mappings) > 0 && mappings[0].DatasetID != "" {
				datasetID = mappings[0].DatasetID
			}
		}
	}

	// 3. Try to find dataset by category (by name, then by display_name for Chinese aliases)
	if datasetID == "" && req.Category != "" {
		mapping, err := s.datasetRepo.GetByName(req.Category)
		if err == nil && mapping.DatasetID != "" {
			datasetID = mapping.DatasetID
			middleware.GetLogger().Info("dataset routed by category name",
				zap.String("category", req.Category), zap.String("dataset_id", datasetID))
		} else {
			mapping, err = s.datasetRepo.GetByDisplayName(req.Category)
			if err == nil && mapping.DatasetID != "" {
				datasetID = mapping.DatasetID
				middleware.GetLogger().Info("dataset routed by category display_name",
					zap.String("category", req.Category), zap.String("dataset_id", datasetID))
			}
		}
	}

	// 4. Use default dataset
	if datasetID == "" {
		defaultMapping, err := s.datasetRepo.GetDefault()
		if err != nil {
			return nil, "", fmt.Errorf("no matching dataset found and no default configured")
		}
		if defaultMapping.DatasetID == "" {
			return nil, "", fmt.Errorf("default dataset mapping %q has empty DatasetID (run sync first)", defaultMapping.Name)
		}
		datasetID = defaultMapping.DatasetID
	}

	// Upload to RagFlow
	resp, err := s.uploadToRagFlow(datasetID, req.Filename, req.Content)
	if err != nil {
		s.logUploadWithRouting(req, datasetID, nil, err, start)
		return nil, datasetID, err
	}

	// Save article-tag associations (non-fatal errors are logged)
	if resp.Code == 0 && resp.Data != nil {
		// Extract document ID from response data (may be array or map)
		var docID string
		switch data := resp.Data.(type) {
		case map[string]interface{}:
			docID, _ = data["id"].(string)
		case []interface{}:
			if len(data) > 0 {
				if item, ok := data[0].(map[string]interface{}); ok {
					docID, _ = item["id"].(string)
				}
			}
		}
		if docID != "" {
			for _, tagName := range req.Tags {
				tag, err := s.tagRepo.GetByName(tagName)
				if err != nil {
					middleware.GetLogger().Warn("failed to find tag for article-tag association",
						zap.String("tag_name", tagName), zap.Error(err))
					continue
				}
				if tag != nil {
					if err := s.datasetRepo.CreateArticleTag(&model.ArticleTag{
						DocumentID:   docID,
						DatasetID:   datasetID,
						TagID:       tag.ID,
						ArticleTitle: req.Title,
						ArticleURL:  req.URL,
					}); err != nil {
						middleware.GetLogger().Warn("failed to create article-tag association",
							zap.String("document_id", docID), zap.String("tag_name", tagName), zap.Error(err))
					}
				}
			}
		}
	}

	s.logUploadWithRouting(req, datasetID, resp, nil, start)
	return resp, datasetID, nil
}

// logUploadWithRouting logs the upload result to activity log.
func (s *RagFlowService) logUploadWithRouting(req *UploadRequest, datasetID string, resp *UploadResponse, err error, start time.Time) {
	if s.activityLog == nil {
		return
	}
	status, summary := "success", fmt.Sprintf("路由上传 %s 到 %s", req.Filename, datasetID)
	if err != nil {
		status, summary = "error", fmt.Sprintf("路由上传 %s 失败: %v", req.Filename, err)
	}
	s.activityLog.LogActivity(LogActivityParams{
		Module: "ragflow_upload", Action: "upload_with_routing", Status: status,
		Summary: summary, RefID: datasetID, DurationMs: int(time.Since(start).Milliseconds()),
	})
}

func (s *RagFlowService) uploadToRagFlow(datasetID, filename, content string) (*UploadResponse, error) {
	url := fmt.Sprintf("%s/api/v1/datasets/%s/documents", s.cfg.BaseURL, datasetID)

	// RagFlow requires multipart file upload, not JSON body
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		return nil, fmt.Errorf("failed to write content: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close form writer: %w", err)
	}

	req, err := http.NewRequest("POST", url, &buf)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result UploadResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// BatchUpload uploads multiple documents to a dataset
func (s *RagFlowService) BatchUpload(datasetID string, documents []UploadRequest) ([]map[string]interface{}, []string) {
	var results []map[string]interface{}
	var errors []string

	for _, doc := range documents {
		resp, err := s.uploadToRagFlow(datasetID, doc.Filename, doc.Content)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", doc.Filename, err))
			continue
		}
		results = append(results, map[string]interface{}{
			"filename": doc.Filename,
			"response": resp,
		})
	}

	return results, errors
}
