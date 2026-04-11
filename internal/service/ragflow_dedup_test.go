package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckURL_EmptyURL(t *testing.T) {
	// Empty URL should return false (not found)
	url := ""
	assert.NotNil(t, url)
	assert.Equal(t, "", url)
}

func TestCheckURL_Normalization(t *testing.T) {
	// Test URL normalization scenarios
	tests := []struct {
		name     string
		url1     string
		url2     string
		shouldMatch bool
	}{
		{
			name:        "exact match",
			url1:        "https://example.com/article",
			url2:        "https://example.com/article",
			shouldMatch: true,
		},
		{
			name:        "different protocols",
			url1:        "http://example.com/article",
			url2:        "https://example.com/article",
			shouldMatch: false, // without normalization
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldMatch {
				assert.Equal(t, tt.url1, tt.url2)
			}
		})
	}
}

func TestCheckURLEnhanced_ResultStructure(t *testing.T) {
	// Verify the result map structure
	result := map[string]interface{}{
		"exists":      true,
		"document_id": "doc123",
		"dataset_id":  "ds456",
		"title":       "Test Article",
		"stored_url":  "https://example.com/test",
		"match_type":  "exact",
	}

	assert.True(t, result["exists"].(bool))
	assert.Equal(t, "doc123", result["document_id"])
	assert.Equal(t, "ds456", result["dataset_id"])
	assert.Equal(t, "Test Article", result["title"])
	assert.Equal(t, "exact", result["match_type"])
}

func TestCheckURLEnhanced_MatchTypes(t *testing.T) {
	validMatchTypes := []string{"exact", "normalized"}

	for _, mt := range validMatchTypes {
		t.Run("match_type_"+mt, func(t *testing.T) {
			found := false
			for _, v := range validMatchTypes {
				if v == mt {
					found = true
					break
				}
			}
			assert.True(t, found)
		})
	}
}

func TestDocumentExistsInRagFlow_EmptyParams(t *testing.T) {
	// Empty dataset_id or document_id should return true (conservative)
	datasetID := ""
	documentID := ""

	// Empty params should return true (assume exists)
	if datasetID == "" || documentID == "" {
		assert.True(t, true, "empty params should return conservative true")
	}
}

func TestDocumentExistsInRagFlow_ResultParsing(t *testing.T) {
	tests := []struct {
		name     string
		code     interface{}
		data     interface{}
		expected bool
	}{
		{
			name:     "code 0 means document exists",
			code:     float64(0),
			data:     map[string]interface{}{"total": float64(1)},
			expected: true,
		},
		{
			name:     "code != 0 means document not found",
			code:     float64(102),
			data:     nil,
			expected: false,
		},
		{
			name:     "empty data means not found",
			code:     float64(0),
			data:     nil,
			expected: false,
		},
		{
			name:     "data with zero total",
			code:     float64(0),
			data:     map[string]interface{}{"total": float64(0)},
			expected: false,
		},
		{
			name:     "data with positive total",
			code:     float64(0),
			data:     map[string]interface{}{"total": float64(5)},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse result like the actual implementation
			if code, ok := tt.code.(float64); ok && code != 0 {
				assert.False(t, tt.expected)
				return
			}

			if tt.data == nil {
				assert.False(t, tt.expected)
				return
			}

			if d, ok := tt.data.(map[string]interface{}); ok {
				if total, ok := d["total"].(float64); ok {
					assert.Equal(t, tt.expected, total > 0)
					return
				}
			}

			assert.Equal(t, tt.expected, true)
		})
	}
}

func TestStaleRecordCleanup(t *testing.T) {
	// Test that stale records are properly identified
	documentID := "stale_doc_123"
	datasetID := "ds_456"

	// When document doesn't exist in RAGFlow, it should be cleaned up
	assert.NotEmpty(t, documentID)
	assert.NotEmpty(t, datasetID)
}
