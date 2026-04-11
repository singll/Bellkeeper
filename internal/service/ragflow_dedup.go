package service

import (
	"log"

	"github.com/singll/bellkeeper/internal/pkg/urlutil"
)

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
