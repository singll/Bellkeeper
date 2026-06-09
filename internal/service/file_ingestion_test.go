package service

import (
	"testing"

	"github.com/singll/bellkeeper/internal/config"

	"github.com/stretchr/testify/assert"
)

func TestFileIngestionService_sanitizeTitle(t *testing.T) {
	svc := &FileIngestionService{}

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "normal title",
			input: "Hello World",
		},
		{
			name:  "Chinese title",
			input: "这是一个测试标题",
		},
		{
			name:  "special characters",
			input: "Test: Title (2023) - Review",
		},
		{
			name:  "empty title",
			input: "",
		},
		{
			name:  "long title",
			input: "This is a very long title that exceeds eighty characters and should be truncated to fit within the limit properly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.sanitizeTitle(tt.input)
			// Verify output is reasonable
			if tt.input != "" {
				assert.NotEmpty(t, got)
				assert.LessOrEqual(t, len(got), 80)
				// Should not contain special characters
				assert.NotContains(t, got, ":")
				assert.NotContains(t, got, "(")
				assert.NotContains(t, got, ")")
			}
		})
	}
}

func TestFileIngestionService_generateFilename(t *testing.T) {
	svc := &FileIngestionService{}

	tests := []struct {
		name  string
		title string
	}{
		{
			name:  "normal title",
			title: "Hello World",
		},
		{
			name:  "Chinese title",
			title: "测试文档",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filename := svc.generateFilename(tt.title)
			// Should end with .md
			assert.True(t, filename[len(filename)-3:] == ".md")
			// Should contain underscore separator
			assert.Contains(t, filename, "_")
			// Should start with date format YYYYMMDD (8 digits)
			datePart := filename[:8]
			assert.Regexp(t, `^\d{8}$`, datePart)
		})
	}
}

func TestFileIngestionService_extractDomain(t *testing.T) {
	svc := &FileIngestionService{}

	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "simple domain",
			url:  "https://example.com/path",
			want: "example.com",
		},
		{
			name: "subdomain",
			url:  "https://blog.example.com/post",
			want: "blog.example.com",
		},
		{
			name: "with port",
			url:  "https://example.com:8080/path",
			want: "example.com",
		},
		{
			name: "invalid url",
			url:  "not a url",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.extractDomain(tt.url)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFileIngestionService_calculateHash(t *testing.T) {
	svc := &FileIngestionService{}

	hash1 := svc.calculateHash("test content")
	hash2 := svc.calculateHash("test content")
	hash3 := svc.calculateHash("different content")

	// Same content should produce same hash
	assert.Equal(t, hash1, hash2)

	// Different content should produce different hash
	assert.NotEqual(t, hash1, hash3)

	// Hash should be 64 characters (SHA256 hex)
	assert.Len(t, hash1, 64)
}

func TestIngestURLRequest_Structure(t *testing.T) {
	req := &IngestURLRequest{
		URL:      "https://example.com/article",
		Title:    "Test Article",
		Tags:     []string{"tech", "golang"},
		Category: "development",
		Layer:    "raw",
	}

	assert.Equal(t, "https://example.com/article", req.URL)
	assert.Equal(t, "Test Article", req.Title)
	assert.Len(t, req.Tags, 2)
	assert.Contains(t, req.Tags, "tech")
	assert.Contains(t, req.Tags, "golang")
	assert.Equal(t, "development", req.Category)
	assert.Equal(t, "raw", req.Layer)
}

func TestIngestURLRequest_Defaults(t *testing.T) {
	req := &IngestURLRequest{
		URL: "https://example.com/article",
	}

	assert.Equal(t, "https://example.com/article", req.URL)
	assert.Empty(t, req.Title)
	assert.Nil(t, req.Tags)
	assert.Empty(t, req.Category)
	assert.Empty(t, req.Layer)
}

func TestIngestURLResponse_Structure(t *testing.T) {
	resp := &IngestURLResponse{
		Success:      true,
		Status:       "success",
		FilePath:     "/mnt/knowledge/raw/test_article.md",
		DocumentID:   "doc123",
		DatasetID:    "ds456",
		Title:        "Test Article",
		Tags:         []string{"tech"},
		Extractor:    "trafilatura",
		ErrorMessage: "",
	}

	assert.True(t, resp.Success)
	assert.Equal(t, "success", resp.Status)
	assert.Equal(t, "/mnt/knowledge/raw/test_article.md", resp.FilePath)
	assert.Equal(t, "trafilatura", resp.Extractor)
	assert.Empty(t, resp.ErrorMessage)
}

func TestIngestURLResponse_Duplicate(t *testing.T) {
	resp := &IngestURLResponse{
		Success: false,
		Status:  "duplicate",
	}

	assert.False(t, resp.Success)
	assert.Equal(t, "duplicate", resp.Status)
	assert.Empty(t, resp.FilePath)
}

func TestIngestURLResponse_ExtractFailed(t *testing.T) {
	resp := &IngestURLResponse{
		Success:      false,
		Status:       "extract_failed",
		ErrorMessage: "content too short",
	}

	assert.False(t, resp.Success)
	assert.Equal(t, "extract_failed", resp.Status)
	assert.Equal(t, "content too short", resp.ErrorMessage)
}

func TestFileIngestionService_escapeYAML(t *testing.T) {
	// Test escapeYAML indirectly through generateFrontmatter
	svc := &FileIngestionService{}
	req := &IngestURLRequest{
		Title: `Test "quoted" title with \backslash`,
		URL:   "https://example.com",
	}

	// generateFrontmatter calls escapeYAML internally
	frontmatter := svc.generateFrontmatter(req, &ExtractionResult{Extractor: "test"})

	// Verify quotes are escaped (should not appear as unescaped quotes in the value)
	// The frontmatter should have \" instead of raw "
	assert.Contains(t, frontmatter, `title: "Test \`)
	// Should not have unescaped quotes that would break YAML
	assert.NotContains(t, frontmatter, `title: "Test "quoted"`)
}

func TestFileIngestionService_validateLayer(t *testing.T) {
	svc := &FileIngestionService{cfg: config.FileIngestionConfig{
		BasePath:     t.TempDir(),
		RawDir:       "raw",
		WorkingDir:   "working",
		DefaultLayer: "raw",
	}}

	for _, layer := range []string{"raw", "working", "archive", "vault"} {
		if err := svc.validateLayer(layer); err != nil {
			t.Fatalf("validateLayer(%q) unexpected error: %v", layer, err)
		}
	}

	for _, layer := range []string{"../outside", "raw/../vault", "custom", "/tmp"} {
		if err := svc.validateLayer(layer); err == nil {
			t.Fatalf("validateLayer(%q) should reject unsafe or unknown layer", layer)
		}
	}
}
