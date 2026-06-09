package service

import (
	"strings"
	"unicode"
)

var tagSynonyms = map[string]string{
	"large-language-model":  "llm",
	"large-language-models": "llm",
	"genai":                 "generative-ai",
	"gen-ai":                "generative-ai",
	"artificial-intelligence": "ai",
	"machine-learning":      "ml",
	"deep-learning":         "dl",
	"natural-language-processing": "nlp",
	"computer-vision":       "cv",
	"cybersecurity":         "security",
	"infosec":               "security",
	"devops":                "dev-ops",
	"web-dev":               "web-development",
	"frontend":              "web-frontend",
	"backend":               "web-backend",
}

var noisyAutoTags = map[string]struct{}{
	"article": {},
	"news":    {},
	"post":    {},
	"update":  {},
	"blog":    {},
	"report":  {},
	"content": {},
	"info":    {},
	"general": {},
}

func normalizeTagList(tags []string) []string {
	return normalizeTagListFiltered(tags, false)
}

func normalizeAutoTagList(tags []string) []string {
	return normalizeTagListFiltered(tags, true)
}

func normalizeTagListFiltered(tags []string, filterNoise bool) []string {
	normalized := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		cleaned := normalizeTagName(tag)
		if cleaned == "" {
			continue
		}
		if filterNoise {
			if _, noisy := noisyAutoTags[cleaned]; noisy {
				continue
			}
		}
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		normalized = append(normalized, cleaned)
		if len(normalized) >= 10 {
			break
		}
	}
	return normalized
}

func normalizeTagName(tag string) string {
	tag = strings.ToLower(strings.TrimSpace(tag))
	tag = strings.Trim(tag, " \t\r\n#[]\"'`")
	tag = strings.Trim(tag, ".,;:，。；：、!！?？")

	var b strings.Builder
	lastHyphen := false
	for _, r := range tag {
		if unicode.IsControl(r) {
			continue
		}
		if unicode.IsSpace(r) || r == '_' || r == '/' {
			if !lastHyphen {
				b.WriteByte('-')
				lastHyphen = true
			}
			continue
		}
		if r == '"' || r == '\'' || r == '`' || r == '[' || r == ']' {
			continue
		}
		b.WriteRune(r)
		lastHyphen = r == '-'
	}

	tag = strings.Trim(b.String(), "-")
	if canonical, ok := tagSynonyms[tag]; ok {
		tag = canonical
	}
	if runeCount := len([]rune(tag)); runeCount < 2 || runeCount > 48 {
		return ""
	}
	return tag
}
