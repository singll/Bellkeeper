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
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// Upload uploads a document to RagFlow
func (s *RagFlowService) Upload(req *UploadRequest) (*UploadResponse, error) {
	datasetID := req.DatasetID
	if datasetID == "" {
		defaultMapping, err := s.datasetRepo.GetDefault()
		if err != nil {
			return nil, fmt.Errorf("no dataset specified and no default found: %w", err)
		}
		datasetID = defaultMapping.DatasetID
	}

	return s.uploadToRagFlow(datasetID, req.Filename, req.Content)
}

// UploadWithRouting uploads with intelligent dataset routing based on tags/category
func (s *RagFlowService) UploadWithRouting(req *UploadRequest) (*UploadResponse, string, error) {
	var datasetID string

	// 1. Try to find dataset by tags
	if len(req.Tags) > 0 && req.AutoCreateTags {
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

	return resp, datasetID, nil
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
	url := fmt.Sprintf("%s/api/v1/datasets/%s/documents?page=%d&limit=%d", s.cfg.BaseURL, datasetID, page, limit)
	return s.doGet(url)
}

// DeleteDocument deletes a document from RagFlow and cleans up local article_tags
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

// --- Batch B: RagFlow 高级操作 ---

// ListDatasets lists all RagFlow datasets (knowledge bases)
func (s *RagFlowService) ListDatasets(page, limit int) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/datasets?page=%d&limit=%d", s.cfg.BaseURL, page, limit)
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
	url := fmt.Sprintf("%s/api/v1/datasets/%s", s.cfg.BaseURL, datasetID)
	return s.doDelete(url)
}

// RunParsing triggers document parsing
func (s *RagFlowService) RunParsing(datasetID string, documentIDs []string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/datasets/%s/documents/parse", s.cfg.BaseURL, datasetID)
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

// StopParsing stops document parsing
func (s *RagFlowService) StopParsing(datasetID string, documentIDs []string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/datasets/%s/documents/parse", s.cfg.BaseURL, datasetID)
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

// BatchDeleteDocuments deletes multiple documents from a dataset
func (s *RagFlowService) BatchDeleteDocuments(datasetID string, documentIDs []string) ([]string, []string) {
	var deleted []string
	var errors []string

	for _, docID := range documentIDs {
		if err := s.DeleteDocument(datasetID, docID); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", docID, err))
		} else {
			deleted = append(deleted, docID)
		}
	}

	return deleted, errors
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

// UpdateDocumentMetadata updates document metadata
func (s *RagFlowService) UpdateDocumentMetadata(datasetID, documentID string, metadata map[string]interface{}) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v1/datasets/%s/documents/%s", s.cfg.BaseURL, datasetID, documentID)
	return s.doPut(url, metadata)
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
