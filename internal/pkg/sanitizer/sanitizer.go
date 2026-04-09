package sanitizer

import (
	"github.com/microcosm-cc/bluemonday"
)

// StrictPolicy creates a policy that only allows safe formatting tags
// (bold, italic, etc.) but strips all scripts and potentially dangerous content.
var StrictPolicy = bluemonday.StrictPolicy()

// RelaxedPolicy creates a policy that allows more formatting but still
// strips scripts and dangerous content. Suitable for user-generated content.
var RelaxedPolicy = bluemonday.UGCPolicy()

// Sanitize removes potentially dangerous HTML content and returns safe HTML.
// Use StrictPolicy for untrusted content.
func Sanitize(html string) string {
	return StrictPolicy.Sanitize(html)
}

// SanitizeRelaxed removes dangerous HTML but allows safe formatting.
// Use for content that should preserve some formatting.
func SanitizeRelaxed(html string) string {
	return RelaxedPolicy.Sanitize(html)
}

// StripAllHTML removes all HTML tags, returning plain text.
func StripAllHTML(html string) string {
	return StrictPolicy.Sanitize(html)
}
