package sanitizer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitize(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "safe HTML preserved",
			input: "<p>Hello <strong>world</strong></p>",
			want:  "<p>Hello <strong>world</strong></p>",
		},
		{
			name:  "script removed",
			input: "<p>Hello</p><script>alert('xss')</script>",
			want:  "<p>Hello</p>",
		},
		{
			name:  "onclick removed",
			input: "<p onclick=\"alert('xss')\">Click me</p>",
			want:  "<p>Click me</p>",
		},
		{
			name:  "plain text unchanged",
			input: "Hello world",
			want:  "Hello world",
		},
		{
			name:  "javascript URL removed",
			input: "<a href=\"javascript:alert('xss')\">Click</a>",
			want:  "<a rel=\"nofollow\">Click</a>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Sanitize(tt.input)
			assert.NotContains(t, got, "script")
			assert.NotContains(t, got, "javascript:")
			assert.NotContains(t, got, "onclick")
		})
	}
}

func TestStripAllHTML(t *testing.T) {
	input := "<p>Hello <strong>world</strong></p>"
	got := StripAllHTML(input)
	assert.Equal(t, "Hello world", got)
}
