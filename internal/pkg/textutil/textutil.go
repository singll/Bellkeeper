package textutil

import "strings"

func StripJSONFence(content string) string {
	return StripFence(content)
}

func StripFence(content string) string {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json\n")
		content = strings.TrimSuffix(content, "```")
	} else if strings.HasPrefix(content, "```markdown") {
		content = strings.TrimPrefix(content, "```markdown\n")
		content = strings.TrimSuffix(content, "```")
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```\n")
		content = strings.TrimSuffix(content, "```")
	}
	return strings.TrimSpace(content)
}
