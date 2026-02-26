package urlutil

import (
	"net/url"
	"strings"
)

// trackingParams are common URL tracking parameters to strip during normalization
var trackingParams = map[string]bool{
	"utm_source":   true,
	"utm_medium":   true,
	"utm_campaign": true,
	"utm_term":     true,
	"utm_content":  true,
	"fbclid":       true,
	"gclid":        true,
	"ref":          true,
	"source":       true,
}

// Normalize normalizes a URL by:
// 1. Lowercasing the domain
// 2. Removing trailing slashes from the path
// 3. Removing tracking parameters (utm_*, fbclid, gclid, ref, source)
// 4. Removing URL fragments
func Normalize(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	// Lowercase the host
	parsed.Host = strings.ToLower(parsed.Host)

	// Remove trailing slash from path (but keep "/" for root)
	if len(parsed.Path) > 1 {
		parsed.Path = strings.TrimRight(parsed.Path, "/")
	}

	// Remove tracking parameters
	q := parsed.Query()
	for param := range trackingParams {
		q.Del(param)
	}
	parsed.RawQuery = q.Encode()

	// Remove fragment
	parsed.Fragment = ""

	return parsed.String()
}

// FuzzyMatch checks if two URLs are likely the same page by comparing
// their normalized paths. Returns true if one path contains the other
// and the path length exceeds minPathLen.
func FuzzyMatch(url1, url2 string, minPathLen int) bool {
	p1, err1 := url.Parse(url1)
	p2, err2 := url.Parse(url2)
	if err1 != nil || err2 != nil {
		return false
	}

	path1 := strings.TrimRight(p1.Path, "/")
	path2 := strings.TrimRight(p2.Path, "/")

	if len(path1) < minPathLen || len(path2) < minPathLen {
		return false
	}

	return strings.Contains(path1, path2) || strings.Contains(path2, path1)
}
