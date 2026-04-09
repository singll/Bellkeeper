package service

import (
	"testing"

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
