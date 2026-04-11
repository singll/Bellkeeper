package service

import (
	"fmt"
	"log"
	"time"
)

// --- RagFlow API Proxy Methods ---

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
