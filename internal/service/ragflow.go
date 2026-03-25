package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/defaults"
	"github.com/singll/bellkeeper/internal/pkg/urlutil"
	"github.com/singll/bellkeeper/internal/repository"
)

type RagFlowService struct {
	cfg         config.RagFlowConfig
	datasetRepo *repository.DatasetMappingRepository
	tagRepo     *repository.TagRepository
	client      *http.Client
	activityLog *ActivityLogService
}

func NewRagFlowService(cfg config.RagFlowConfig, datasetRepo *repository.DatasetMappingRepository, tagRepo *repository.TagRepository) *RagFlowService {
	return &RagFlowService{
		cfg:         cfg,
		datasetRepo: datasetRepo,
		tagRepo:     tagRepo,
		client:      &http.Client{Timeout: time.Duration(cfg.Timeout) * time.Second},
	}
}

// SetActivityLog injects the activity log service for instrumentation.
func (s *RagFlowService) SetActivityLog(al *ActivityLogService) {
	s.activityLog = al
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
			log.Printf("info: dataset routed by LLM recommendation %q -> %s", req.DatasetID, datasetID)
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
			log.Printf("info: dataset routed by category name %q -> %s", req.Category, datasetID)
		} else {
			mapping, err = s.datasetRepo.GetByDisplayName(req.Category)
			if err == nil && mapping.DatasetID != "" {
				datasetID = mapping.DatasetID
				log.Printf("info: dataset routed by category display_name %q -> %s", req.Category, datasetID)
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
				tag, _ := s.tagRepo.GetByName(tagName)
				if tag != nil {
					if err := s.datasetRepo.CreateArticleTag(&model.ArticleTag{
						DocumentID:   docID,
						DatasetID:    datasetID,
						TagID:        tag.ID,
						ArticleTitle: req.Title,
						ArticleURL:   req.URL,
					}); err != nil {
						log.Printf("warn: failed to create article-tag association for doc %s tag %s: %v", docID, tagName, err)
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

// CheckURL checks if a URL has been uploaded before
func (s *RagFlowService) CheckURL(url string) (bool, error) {
	return s.datasetRepo.ArticleURLExists(url)
}

// CheckURLEnhanced checks a URL with optional normalization, returning detailed info.
// When a local match is found, it verifies the document still exists in RAGFlow.
// Stale records (document deleted from RAGFlow) are automatically cleaned up.
func (s *RagFlowService) CheckURLEnhanced(rawURL string, normalize bool) (map[string]interface{}, error) {
	// 1. Exact match in local ArticleTag table
	ats, err := s.datasetRepo.FindArticleTagsByURL(rawURL)
	if err != nil {
		return nil, err
	}
	if len(ats) > 0 {
		// Verify document still exists in RAGFlow
		if s.DocumentExistsInRagFlow(ats[0].DatasetID, ats[0].DocumentID) {
			return map[string]interface{}{
				"exists":      true,
				"document_id": ats[0].DocumentID,
				"dataset_id":  ats[0].DatasetID,
				"title":       ats[0].ArticleTitle,
				"stored_url":  ats[0].ArticleURL,
				"match_type":  "exact",
			}, nil
		}
		// Document no longer in RAGFlow, clean up stale records
		log.Printf("info: document %s no longer exists in RAGFlow, cleaning up stale article_tags for URL %s", ats[0].DocumentID, rawURL)
		if err := s.datasetRepo.DeleteArticleTagsByDocumentIDs([]string{ats[0].DocumentID}); err != nil {
			log.Printf("warn: failed to clean up stale article_tags for document %s: %v", ats[0].DocumentID, err)
		}
	}

	// 2. Normalized match
	if normalize {
		normalizedURL := urlutil.Normalize(rawURL)
		allArticles, err := s.datasetRepo.GetAllArticleURLs()
		if err != nil {
			return nil, err
		}
		for _, at := range allArticles {
			if urlutil.Normalize(at.ArticleURL) == normalizedURL {
				// Verify document still exists in RAGFlow
				if s.DocumentExistsInRagFlow(at.DatasetID, at.DocumentID) {
					return map[string]interface{}{
						"exists":      true,
						"document_id": at.DocumentID,
						"dataset_id":  at.DatasetID,
						"title":       at.ArticleTitle,
						"stored_url":  at.ArticleURL,
						"match_type":  "normalized",
					}, nil
				}
				// Stale record, clean up
				log.Printf("info: document %s no longer exists in RAGFlow, cleaning up stale article_tags", at.DocumentID)
				if err := s.datasetRepo.DeleteArticleTagsByDocumentIDs([]string{at.DocumentID}); err != nil {
					log.Printf("warn: failed to clean up stale article_tags for document %s: %v", at.DocumentID, err)
				}
			}
		}
	}

	return map[string]interface{}{"exists": false}, nil
}

func (s *RagFlowService) uploadToRagFlow(datasetID, filename, content string) (*UploadResponse, error) {
	url := fmt.Sprintf("%s/api/v1/datasets/%s/documents", s.cfg.BaseURL, datasetID)

	// RagFlow 要求 multipart file upload，不接受 JSON body
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

// ListDocuments lists documents in a dataset
func (s *RagFlowService) ListDocuments(datasetID string, page, limit int) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/datasets/%s/documents?page=%d&page_size=%d", s.cfg.BaseURL, datasetID, page, limit)
	return s.doGet(url)
}

// DeleteDocument deletes a document from RagFlow and cleans up local article_tags
func (s *RagFlowService) DeleteDocument(datasetID, documentID string) error {
	url := fmt.Sprintf("%s/api/v1/datasets/%s/documents", s.cfg.BaseURL, datasetID)
	payload := map[string]interface{}{"ids": []string{documentID}}
	_, err := s.doRequestJSON("DELETE", url, payload)
	if err != nil {
		return err
	}

	// Clean up local article_tags so the URL can be re-uploaded
	if err := s.datasetRepo.DeleteArticleTagsByDocumentIDs([]string{documentID}); err != nil {
		log.Printf("warn: failed to clean up article_tags for document %s: %v", documentID, err)
	}

	return nil
}

// DocumentExistsInRagFlow checks if a document still exists in RAGFlow.
// Returns true on errors (conservative: assume exists if we can't verify).
func (s *RagFlowService) DocumentExistsInRagFlow(datasetID, documentID string) bool {
	if datasetID == "" || documentID == "" {
		return true
	}

	result, err := s.GetParsingStatus(datasetID, documentID)
	if err != nil {
		log.Printf("warn: failed to verify document %s in RAGFlow: %v", documentID, err)
		return true // assume exists on network error
	}

	// RAGFlow returns code != 0 on error
	if code, ok := result["code"].(float64); ok && code != 0 {
		return false
	}

	// Check data field for document presence
	data, ok := result["data"]
	if !ok || data == nil {
		return false
	}

	switch d := data.(type) {
	case map[string]interface{}:
		if total, ok := d["total"].(float64); ok {
			return total > 0
		}
		if docs, ok := d["docs"].([]interface{}); ok {
			return len(docs) > 0
		}
	case []interface{}:
		return len(d) > 0
	}

	return true // unclear format, assume exists
}

// --- Dataset Sync ---

// SyncDatasetMappings synchronizes local dataset_mappings with RAGFlow.
// For mappings with empty DatasetID, it tries to match by DisplayName against
// existing RAGFlow datasets. If no match is found, it creates a new dataset
// in RAGFlow. This eliminates the need to manually fill in RAGFlow UUIDs.
func (s *RagFlowService) SyncDatasetMappings() (*SyncResult, error) {
	result := &SyncResult{}

	// 1. Get all active local mappings
	mappings, err := s.datasetRepo.GetAll()
	if err != nil {
		return nil, fmt.Errorf("failed to list local mappings: %w", err)
	}

	// 2. List all RAGFlow datasets, build name→id lookup
	ragflowMap, err := s.buildRagFlowDatasetMap()
	if err != nil {
		return nil, fmt.Errorf("failed to list RAGFlow datasets: %w", err)
	}
	log.Printf("info: sync found %d RAGFlow datasets, %d local mappings", len(ragflowMap), len(mappings))

	// 3. For each mapping with empty DatasetID, try to match or create
	for _, m := range mappings {
		if m.DatasetID != "" {
			result.Skipped++
			continue
		}

		displayName := m.DisplayName
		if displayName == "" {
			displayName = m.Name
		}

		// Try to match by name in RAGFlow
		if rfID, ok := ragflowMap[displayName]; ok {
			if err := s.datasetRepo.UpdateDatasetID(m.ID, rfID); err != nil {
				log.Printf("warn: sync failed to update mapping %q: %v", m.Name, err)
				result.Failed++
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", m.Name, err))
				continue
			}
			log.Printf("info: sync matched %q -> RAGFlow dataset %s", displayName, rfID)
			result.Matched++
			continue
		}

		// No match — create dataset in RAGFlow
		params := map[string]interface{}{}
		if m.ParserID != "" {
			params["chunk_method"] = m.ParserID
		}
		createResp, err := s.CreateDataset(displayName, params)
		if err != nil {
			log.Printf("warn: sync failed to create RAGFlow dataset %q: %v", displayName, err)
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: create failed: %v", m.Name, err))
			continue
		}

		// Extract dataset ID from create response
		newID := extractDatasetID(createResp)
		if newID == "" {
			log.Printf("warn: sync created dataset %q but could not extract ID from response", displayName)
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: no ID in create response", m.Name))
			continue
		}

		if err := s.datasetRepo.UpdateDatasetID(m.ID, newID); err != nil {
			log.Printf("warn: sync created dataset %q (%s) but failed to update local mapping: %v", displayName, newID, err)
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: update failed: %v", m.Name, err))
			continue
		}
		log.Printf("info: sync created RAGFlow dataset %q -> %s", displayName, newID)
		result.Created++
	}

	log.Printf("info: sync complete — matched:%d created:%d skipped:%d failed:%d",
		result.Matched, result.Created, result.Skipped, result.Failed)
	return result, nil
}

// SyncResult holds the outcome of a dataset sync operation.
type SyncResult struct {
	Matched int      `json:"matched"`
	Created int      `json:"created"`
	Skipped int      `json:"skipped"`
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors,omitempty"`
}

// buildRagFlowDatasetMap fetches all RAGFlow datasets and returns a name→id map.
func (s *RagFlowService) buildRagFlowDatasetMap() (map[string]string, error) {
	resp, err := s.ListDatasets(1, 1000)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)

	dataRaw, ok := resp["data"]
	if !ok {
		return result, nil
	}

	datasets, ok := dataRaw.([]interface{})
	if !ok {
		return result, nil
	}

	for _, d := range datasets {
		ds, ok := d.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := ds["name"].(string)
		id, _ := ds["id"].(string)
		if name != "" && id != "" {
			result[name] = id
		}
	}
	return result, nil
}

// extractDatasetID extracts the dataset ID from a RAGFlow create response.
func extractDatasetID(resp map[string]interface{}) string {
	// RAGFlow returns {"code":0,"data":{"id":"...","name":"...",...}}
	dataRaw, ok := resp["data"]
	if !ok {
		return ""
	}
	switch data := dataRaw.(type) {
	case map[string]interface{}:
		id, _ := data["id"].(string)
		return id
	case []interface{}:
		if len(data) > 0 {
			if item, ok := data[0].(map[string]interface{}); ok {
				id, _ := item["id"].(string)
				return id
			}
		}
	}
	return ""
}

// --- Batch B: RagFlow 高级操作 ---

// ListDatasets lists all RagFlow datasets (knowledge bases)
func (s *RagFlowService) ListDatasets(page, limit int) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/datasets?page=%d&page_size=%d", s.cfg.BaseURL, page, limit)
	return s.doGet(url)
}

// GetDataset gets a single dataset's details
func (s *RagFlowService) GetDataset(datasetID string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/datasets/%s", s.cfg.BaseURL, datasetID)
	return s.doGet(url)
}

// CreateDataset creates a new RagFlow dataset
func (s *RagFlowService) CreateDataset(name string, params map[string]interface{}) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/datasets", s.cfg.BaseURL)
	if params == nil {
		params = make(map[string]interface{})
	}
	params["name"] = name
	return s.doPost(url, params)
}

// UpdateDataset updates a RagFlow dataset
func (s *RagFlowService) UpdateDataset(datasetID string, params map[string]interface{}) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/datasets/%s", s.cfg.BaseURL, datasetID)
	return s.doPut(url, params)
}

// DeleteDataset deletes a RagFlow dataset
func (s *RagFlowService) DeleteDataset(datasetID string) error {
	url := fmt.Sprintf("%s/api/v1/datasets", s.cfg.BaseURL)
	payload := map[string]interface{}{"ids": []string{datasetID}}
	_, err := s.doRequestJSON("DELETE", url, payload)
	return err
}

// RunParsing triggers document parsing (RAGFlow uses POST /chunks for this operation)
func (s *RagFlowService) RunParsing(datasetID string, documentIDs []string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/datasets/%s/chunks", s.cfg.BaseURL, datasetID)
	payload := map[string]interface{}{"document_ids": documentIDs}
	return s.doPost(url, payload)
}

// RunParsingThrottled submits documents for parsing in small batches with delays
// to avoid triggering upstream Embedding rate limits. Runs in background.
func (s *RagFlowService) RunParsingThrottled(datasetID string, documentIDs []string, batchSize, intervalSeconds int) {
	if batchSize <= 0 {
		batchSize = 3
	}
	if intervalSeconds <= 0 {
		intervalSeconds = 30
	}

	go func() {
		total := len(documentIDs)
		for i := 0; i < total; i += batchSize {
			end := i + batchSize
			if end > total {
				end = total
			}
			batch := documentIDs[i:end]
			batchNum := i/batchSize + 1

			_, err := s.RunParsing(datasetID, batch)
			if err != nil {
				log.Printf("warn: throttled parsing batch %d failed for dataset %s: %v", batchNum, datasetID, err)
			} else {
				log.Printf("info: throttled parsing batch %d (%d docs) submitted for dataset %s", batchNum, len(batch), datasetID)
			}

			if end < total {
				time.Sleep(time.Duration(intervalSeconds) * time.Second)
			}
		}
		log.Printf("info: throttled parsing completed for dataset %s (%d docs in batches of %d)", datasetID, total, batchSize)
	}()
}

// StopParsing stops document parsing (RAGFlow uses DELETE /chunks for this operation)
func (s *RagFlowService) StopParsing(datasetID string, documentIDs []string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/datasets/%s/chunks", s.cfg.BaseURL, datasetID)
	payload := map[string]interface{}{"document_ids": documentIDs}
	return s.doRequestJSON("DELETE", url, payload)
}

// GetParsingStatus gets document parsing status
func (s *RagFlowService) GetParsingStatus(datasetID, documentID string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/datasets/%s/documents?id=%s", s.cfg.BaseURL, datasetID, documentID)
	return s.doGet(url)
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

// BatchDeleteDocuments deletes multiple documents from a dataset in a single API call
func (s *RagFlowService) BatchDeleteDocuments(datasetID string, documentIDs []string) error {
	url := fmt.Sprintf("%s/api/v1/datasets/%s/documents", s.cfg.BaseURL, datasetID)
	payload := map[string]interface{}{"ids": documentIDs}
	_, err := s.doRequestJSON("DELETE", url, payload)
	if err != nil {
		return err
	}

	// Clean up local article_tags
	if err := s.datasetRepo.DeleteArticleTagsByDocumentIDs(documentIDs); err != nil {
		log.Printf("warn: failed to clean up article_tags for %d documents: %v", len(documentIDs), err)
	}

	return nil
}

// TransferDocument transfers a document from one dataset to another
func (s *RagFlowService) TransferDocument(sourceDatasetID, targetDatasetID, documentID string) (map[string]interface{}, error) {
	downloadURL := fmt.Sprintf("%s/api/v1/datasets/%s/documents/%s/download", s.cfg.BaseURL, sourceDatasetID, documentID)
	content, filename, err := s.downloadDocument(downloadURL)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}

	resp, err := s.uploadToRagFlow(targetDatasetID, filename, content)
	if err != nil {
		return nil, fmt.Errorf("upload to target failed: %w", err)
	}

	if err := s.DeleteDocument(sourceDatasetID, documentID); err != nil {
		return map[string]interface{}{
			"upload":        resp,
			"delete_failed": true,
			"error":         err.Error(),
		}, nil
	}

	return map[string]interface{}{
		"upload":  resp,
		"deleted": true,
	}, nil
}

// BatchTransferDocuments transfers multiple documents between datasets
func (s *RagFlowService) BatchTransferDocuments(sourceDatasetID, targetDatasetID string, documentIDs []string) (map[string]interface{}, error) {
	var results []map[string]interface{}
	successCount := 0
	failedCount := 0

	for _, docID := range documentIDs {
		result, err := s.TransferDocument(sourceDatasetID, targetDatasetID, docID)
		entry := map[string]interface{}{
			"document_id": docID,
		}
		if err != nil {
			entry["success"] = false
			entry["error"] = err.Error()
			failedCount++
		} else {
			entry["success"] = true
			entry["result"] = result
			successCount++
		}
		results = append(results, entry)
	}

	return map[string]interface{}{
		"total":   len(documentIDs),
		"success": successCount,
		"failed":  failedCount,
		"results": results,
	}, nil
}

// GetDocument gets a single document's details from RagFlow.
func (s *RagFlowService) GetDocument(datasetID, documentID string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/datasets/%s/documents?id=%s", s.cfg.BaseURL, datasetID, documentID)
	return s.doGet(url)
}

// UpdateDocumentMetadata updates document metadata
func (s *RagFlowService) UpdateDocumentMetadata(datasetID, documentID string, metadata map[string]interface{}) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/datasets/%s/documents/%s", s.cfg.BaseURL, datasetID, documentID)
	return s.doPut(url, metadata)
}

// UpdateDocumentParserConfig updates a document's parser method and parser_config.
func (s *RagFlowService) UpdateDocumentParserConfig(datasetID, documentID, parserID string, parserConfig map[string]interface{}) (map[string]interface{}, error) {
	payload := map[string]interface{}{}
	if parserID != "" {
		payload["chunk_method"] = parserID
	}
	if len(parserConfig) > 0 {
		payload["parser_config"] = parserConfig
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("parser_id or parser_config required")
	}
	return s.UpdateDocumentMetadata(datasetID, documentID, payload)
}

func (s *RagFlowService) buildSafeParserProfile(filename string) (string, map[string]interface{}, bool) {
	lower := strings.ToLower(strings.TrimSpace(filename))
	if lower != "" && !strings.HasSuffix(lower, ".md") && !strings.HasSuffix(lower, ".markdown") && !strings.HasSuffix(lower, ".txt") {
		return "", nil, false
	}

	return defaults.DefaultParserID, map[string]interface{}{
		"layout_recognize":   "DeepDOC",
		"chunk_token_num":    256,
		"delimiter":          "\n",
		"auto_keywords":      0,
		"auto_questions":     0,
		"html4excel":         false,
		"topn_tags":          3,
		"table_context_size": 0,
		"image_context_size": 0,
		"raptor": map[string]interface{}{
			"use_raptor": false,
		},
		"graphrag": map[string]interface{}{
			"use_graphrag": false,
		},
	}, true
}

func (s *RagFlowService) getDocumentFilename(datasetID, documentID string) (string, error) {
	result, err := s.GetDocument(datasetID, documentID)
	if err != nil {
		return "", err
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected document response format")
	}
	docs, ok := data["docs"].([]interface{})
	if !ok || len(docs) == 0 {
		return "", fmt.Errorf("document not found")
	}
	doc, ok := docs[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected document item format")
	}
	for _, key := range []string{"name", "filename", "file_name", "title"} {
		if value, ok := doc[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value), nil
		}
	}
	return "", fmt.Errorf("document filename not found")
}

// ListChunks lists chunks for a document
func (s *RagFlowService) ListChunks(datasetID, documentID string, page, limit int) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/datasets/%s/documents/%s/chunks?page=%d&limit=%d",
		s.cfg.BaseURL, datasetID, documentID, page, limit)
	return s.doGet(url)
}

// DeleteChunks deletes specific chunks
func (s *RagFlowService) DeleteChunks(datasetID, documentID string, chunkIDs []string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/datasets/%s/documents/%s/chunks", s.cfg.BaseURL, datasetID, documentID)
	payload := map[string]interface{}{"chunk_ids": chunkIDs}
	return s.doRequestJSON("DELETE", url, payload)
}

// --- HTTP helper methods ---

func (s *RagFlowService) doGet(url string) (map[string]interface{}, error) {
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return result, nil
}

func (s *RagFlowService) doPost(url string, payload map[string]interface{}) (map[string]interface{}, error) {
	return s.doRequestJSON("POST", url, payload)
}

func (s *RagFlowService) doPut(url string, payload map[string]interface{}) (map[string]interface{}, error) {
	return s.doRequestJSON("PUT", url, payload)
}

func (s *RagFlowService) doDelete(url string) error {
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
		return fmt.Errorf("request failed: %s", string(body))
	}
	return nil
}

func (s *RagFlowService) doRequestJSON(method, url string, payload map[string]interface{}) (map[string]interface{}, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest(method, url, bytes.NewBuffer(body))
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

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return result, nil
}

func (s *RagFlowService) downloadDocument(url string) (string, string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("download failed: %s", string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("failed to read download response: %w", err)
	}

	// Extract filename from Content-Disposition header
	filename := "document.txt"
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if idx := bytes.Index([]byte(cd), []byte("filename=")); idx != -1 {
			filename = cd[idx+9:]
			filename = strings.Trim(filename, "\"")
		}
	}

	return string(body), filename, nil
}
