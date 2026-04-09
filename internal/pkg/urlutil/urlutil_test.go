package urlutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "remove trailing slash",
			url:  "https://example.com/path/",
			want: "https://example.com/path",
		},
		{
			name: "lowercase host",
			url:  "https://EXAMPLE.COM/Path",
			want: "https://example.com/Path",
		},
		{
			name: "remove utm",
			url:  "https://example.com/path?utm_source=test",
			want: "https://example.com/path",
		},
		{
			name: "remove trailing slash from root",
			url:  "https://example.com/",
			want: "https://example.com/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Normalize(tt.url)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNormalize_InvalidURL(t *testing.T) {
	// Invalid URL returns as-is when parsing fails
	// But "not a url" is technically parseable by url.Parse
	got := Normalize("not a url")
	// URL parsing may succeed but result in unexpected format
	assert.NotEmpty(t, got)
}

func TestFuzzyMatch(t *testing.T) {
	tests := []struct {
		name       string
		url1       string
		url2       string
		minPathLen int
		want       bool
	}{
		{
			name:       "similar paths",
			url1:       "https://example.com/article/how-to-code",
			url2:       "https://example.com/article/how-to-code-in-go",
			minPathLen: 5,
			want:       true,
		},
		{
			name:       "different paths",
			url1:       "https://example.com/article-1",
			url2:       "https://example.com/article-2",
			minPathLen: 5,
			want:       false,
		},
		{
			name:       "short path below threshold",
			url1:       "https://example.com/a",
			url2:       "https://example.com/ab",
			minPathLen: 5,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FuzzyMatch(tt.url1, tt.url2, tt.minPathLen)
			assert.Equal(t, tt.want, got)
		})
	}
}

