package service

import (
	"strings"
	"testing"
)

func TestStripFrontmatter(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "with frontmatter",
			input: "---\ncreated: 2026-05-03\n---\n\nHello",
			want:  "Hello",
		},
		{
			name:  "no frontmatter",
			input: "Hello world",
			want:  "Hello world",
		},
		{
			name: "unclosed frontmatter",
			input: "---\ncreated: 2026-05-03\nHello",
			want:  "---\ncreated: 2026-05-03\nHello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripFrontmatter(tt.input)
			if strings.TrimSpace(got) != strings.TrimSpace(tt.want) {
				t.Errorf("stripFrontmatter() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseSections(t *testing.T) {
	content := `### 📊 每日知识库摘要 - 2026-05-03

#### 服务状态

| 服务 | 状态 |
|------|------|
| Bellkeeper | ✅ |

#### 今日内容采集

*今日无采集记录*

#### 爬取队列概览

*队列空闲*`

	title, sections := parseSections(content)
	if title != "### 📊 每日知识库摘要 - 2026-05-03" {
		t.Errorf("title = %q", title)
	}
	if len(sections) != 3 {
		t.Fatalf("expected 3 sections, got %d", len(sections))
	}
	if sections[0].heading != "#### 服务状态" {
		t.Errorf("section[0].heading = %q", sections[0].heading)
	}
	if !strings.Contains(sections[0].body, "| Bellkeeper | ✅ |") {
		t.Errorf("section[0].body missing expected content: %q", sections[0].body)
	}
	if sections[1].heading != "#### 今日内容采集" {
		t.Errorf("section[1].heading = %q", sections[1].heading)
	}
}

func TestMergeMarkdown_NewSection(t *testing.T) {
	existing := `#### 服务状态

| 服务 | 状态 |
|------|------|
| Bellkeeper | ✅ |`

	new := `#### 服务状态

| 服务 | 状态 |
|------|------|
| Bellkeeper | ✅ |

#### 今日内容采集

*今日无采集记录*`

	merged, newSections, newLines := mergeMarkdown(existing, new)
	if newSections != 1 {
		t.Errorf("newSections = %d, want 1", newSections)
	}
	if !strings.Contains(merged, "#### 今日内容采集") {
		t.Error("merged should contain new section")
	}
	if !strings.Contains(merged, "#### 服务状态") {
		t.Error("merged should still contain existing section")
	}
	// Should not duplicate the Bellkeeper row
	count := strings.Count(merged, "| Bellkeeper | ✅ |")
	if count != 1 {
		t.Errorf("Bellkeeper row appears %d times, want 1", count)
	}
	_ = newLines
}

func TestMergeMarkdown_IncrementalLines(t *testing.T) {
	existing := `#### 今日内容采集

| 指标 | 数量 |
|------|------|
| 成功入库 | 5 篇 |`

	new := `#### 今日内容采集

| 指标 | 数量 |
|------|------|
| 成功入库 | 5 篇 |
| 失败 | 2 篇 |

**失败记录:**

- https://example.com/fail1`

	merged, newSections, newLines := mergeMarkdown(existing, new)
	if newSections != 0 {
		t.Errorf("newSections = %d, want 0", newSections)
	}
	if newLines != 3 { // "| 失败 | 2 篇 |", "**失败记录:**", and "- https://example.com/fail1"
		t.Errorf("newLines = %d, want 3", newLines)
	}
	// Should contain both the old and new rows
	if !strings.Contains(merged, "| 成功入库 | 5 篇 |") {
		t.Error("merged should contain existing row")
	}
	if !strings.Contains(merged, "| 失败 | 2 篇 |") {
		t.Error("merged should contain new row")
	}
	if !strings.Contains(merged, "- https://example.com/fail1") {
		t.Error("merged should contain new failure record")
	}
	// Should not duplicate the success row
	count := strings.Count(merged, "| 成功入库 | 5 篇 |")
	if count != 1 {
		t.Errorf("success row appears %d times, want 1", count)
	}
}

func TestMergeMarkdown_NoDuplicates(t *testing.T) {
	existing := `#### Worker 通道状态

| 通道 | 熔断器 | 连续失败 | 工人数 |
|------|--------|----------|--------|
| firecrawl | ✅ closed | 0 | 2 |`

	// New content is identical to existing
	new := `#### Worker 通道状态

| 通道 | 熔断器 | 连续失败 | 工人数 |
|------|--------|----------|--------|
| firecrawl | ✅ closed | 0 | 2 |`

	merged, newSections, newLines := mergeMarkdown(existing, new)
	if newSections != 0 {
		t.Errorf("newSections = %d, want 0", newSections)
	}
	if newLines != 0 {
		t.Errorf("newLines = %d, want 0 (no new content)", newLines)
	}
	// Should have exactly one firecrawl row
	count := strings.Count(merged, "| firecrawl |")
	if count != 1 {
		t.Errorf("firecrawl row appears %d times, want 1", count)
	}
}

func TestMergeMarkdown_PreservesExistingSections(t *testing.T) {
	existing := `#### 服务状态

| 服务 | 状态 |
|------|------|
| Bellkeeper | ✅ |

#### 今日内容采集

*今日无采集记录*`

	// New content only has 服务状态, missing 今日内容采集
	new := `#### 服务状态

| 服务 | 状态 |
|------|------|
| Bellkeeper | ✅ |
| n8n | ✅ |`

	merged, _, _ := mergeMarkdown(existing, new)
	// Should preserve the existing section that's missing from new
	if !strings.Contains(merged, "#### 今日内容采集") {
		t.Error("merged should preserve existing section not in new content")
	}
	if !strings.Contains(merged, "| n8n | ✅ |") {
		t.Error("merged should contain new n8n row")
	}
}
