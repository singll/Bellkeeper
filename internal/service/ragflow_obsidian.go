package service

import (
	"fmt"
	"strings"
)

// ObsidianIngestRequest is the input for IngestObsidianNote.
type ObsidianIngestRequest struct {
	DocID   string   `json:"doc_id"`  // CouchDB document ID
	Vault   string   `json:"vault"`   // vault name, e.g. "personal"
	Path    string   `json:"path"`    // note path within vault, e.g. "notes/foo.md"
	Content string   `json:"content"` // raw Markdown content
	Title   string   `json:"title"`
	Tags    []string `json:"tags"`
}

// IngestObsidianNote deduplicates by obsidian://vault/path, then uploads to RAGFlow via UploadWithRouting.
func (s *RagFlowService) IngestObsidianNote(req *ObsidianIngestRequest) (map[string]interface{}, error) {
	noteURL := "obsidian://" + req.Vault + "/" + strings.TrimPrefix(req.Path, "/")

	exists, err := s.datasetRepo.ArticleURLExists(noteURL)
	if err != nil {
		return nil, fmt.Errorf("dedup check failed: %w", err)
	}
	if exists {
		return map[string]interface{}{"skipped": true, "reason": "already_indexed", "note_url": noteURL}, nil
	}

	filename := req.Path
	if idx := strings.LastIndex(filename, "/"); idx >= 0 {
		filename = filename[idx+1:]
	}
	if filename == "" {
		filename = req.DocID + ".md"
	}

	title := req.Title
	if title == "" {
		title = strings.TrimSuffix(filename, ".md")
	}

	uploadReq := &UploadRequest{
		URL:            noteURL,
		Filename:       filename,
		Content:        req.Content,
		Title:          title,
		Tags:           req.Tags,
		Category:       "obsidian-notes",
		AutoCreateTags: true,
	}

	resp, datasetID, err := s.UploadWithRouting(uploadReq)
	if err != nil {
		return nil, fmt.Errorf("upload failed: %w", err)
	}

	return map[string]interface{}{
		"skipped":    false,
		"note_url":   noteURL,
		"dataset_id": datasetID,
		"response":   resp,
	}, nil
}
