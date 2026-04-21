package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/singll/bellkeeper/internal/pkg/defaults"
)

// HTTP helper methods for RagFlowService

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

func (s *RagFlowService) buildSafeParserProfile(filename string) (string, map[string]interface{}, bool) {
	return defaults.DefaultParserID, map[string]interface{}{
		"layout_recognize":    "naive",
		"chunk_token_num":      defaults.ParserDefaultChunkTokenNum,
		"delimiter":            defaults.ParserDefaultDelimiter,
		"auto_keywords":        0,
		"auto_questions":       0,
		"html4excel":           false,
		"topn_tags":            defaults.ParserDefaultTopNTags,
		"table_context_size":  0,
		"image_context_size":   0,
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
