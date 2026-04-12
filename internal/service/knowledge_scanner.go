package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/singll/bellkeeper/internal/config"
)

// FileScanner 发现文件变更
type FileScanner struct {
	basePath string
	scanDirs []config.ScanDirConfig
}

// NewFileScanner 创建文件扫描器
func NewFileScanner(cfg config.KnowledgeConfig) *FileScanner {
	return &FileScanner{
		basePath: cfg.BasePath,
		scanDirs: cfg.ScanDirs,
	}
}

// FileInfo 文件信息
type FileInfo struct {
	AbsPath     string
	RelPath     string
	Layer       string
	Content     []byte
	ContentHash string
	Title       string
}

// ScanDirectory 扫描目录，返回所有文件
func (s *FileScanner) ScanDirectory() ([]FileInfo, error) {
	var files []FileInfo

	for _, dir := range s.scanDirs {
		dirPath := filepath.Join(s.basePath, dir.Path)

		entries, err := os.ReadDir(dirPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read directory %s: %w", dir.Path, err)
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if ext != ".md" && ext != ".txt" {
				continue
			}

			filePath := filepath.Join(dirPath, entry.Name())
			content, err := os.ReadFile(filePath)
			if err != nil {
				continue
			}

			hash := sha256.Sum256(content)

			relPath := filepath.Join(dir.Path, entry.Name())

			// 从文件名提取标题（去掉扩展名）
			title := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))

			files = append(files, FileInfo{
				AbsPath:     filePath,
				RelPath:     relPath,
				Layer:       dir.Layer,
				Content:     content,
				ContentHash: hex.EncodeToString(hash[:]),
				Title:       title,
			})
		}
	}

	return files, nil
}

// MarkdownParser 解析 Markdown 文件
type MarkdownParser struct{}

// NewMarkdownParser 创建 Markdown 解析器
func NewMarkdownParser() *MarkdownParser {
	return &MarkdownParser{}
}

// ParseResult 解析结果
type ParseResult struct {
	Title        string
	SourceURL    string
	SourceDomain string
	Category     string
	Tags         []string
	Body         string
	Headings     []Heading
}

// Heading 标题结构
type Heading struct {
	Level   int
	Text    string
	Content string
}

// ParseMarkdown 解析 Markdown
func (p *MarkdownParser) ParseMarkdown(fileInfo *FileInfo) (*ParseResult, error) {
	content := string(fileInfo.Content)

	// 解析 frontmatter
	fm, body, err := p.parseFrontmatter(content)
	if err != nil {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}

	result := &ParseResult{
		Title:    fileInfo.Title,
		Category: getStringFromMap(fm, "category"),
		Tags:     getTagsFromMap(fm),
		Body:     body,
	}

	// 提取 source URL
	if source := getStringFromMap(fm, "source"); source != "" {
		result.SourceURL = source
		// 提取域名
		if domain := extractDomain(source); domain != "" {
			result.SourceDomain = domain
		}
	} else if url := getStringFromMap(fm, "url"); url != "" {
		result.SourceURL = url
		if domain := extractDomain(url); domain != "" {
			result.SourceDomain = domain
		}
	}

	// 提取标题
	if title := getStringFromMap(fm, "title"); title != "" {
		result.Title = title
	}

	// 提取 headings
	result.Headings = p.extractHeadings(body)

	return result, nil
}

// parseFrontmatter 解析 YAML frontmatter
func (p *MarkdownParser) parseFrontmatter(content string) (map[string]string, string, error) {
	lines := strings.Split(content, "\n")

	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return nil, content, nil
	}

	var fmLines []string
	bodyStart := 1
	for i := 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "---" {
			bodyStart = i + 1
			break
		}
		fmLines = append(fmLines, lines[i])
	}

	// 解析 YAML 简单键值对
	fm := parseSimpleYAML(strings.Join(fmLines, "\n"))
	body := strings.Join(lines[bodyStart:], "\n")

	return fm, body, nil
}

// extractHeadings 提取标题
func (p *MarkdownParser) extractHeadings(content string) []Heading {
	var headings []Heading
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "### ") {
			headings = append(headings, Heading{
				Level: 3,
				Text:  strings.TrimPrefix(trimmed, "### "),
			})
		} else if strings.HasPrefix(trimmed, "## ") {
			headings = append(headings, Heading{
				Level: 2,
				Text:  strings.TrimPrefix(trimmed, "## "),
			})
		} else if strings.HasPrefix(trimmed, "# ") {
			headings = append(headings, Heading{
				Level: 1,
				Text:  strings.TrimPrefix(trimmed, "# "),
			})
		}
	}

	return headings
}

// ChunkSplitter 文本分块
type ChunkSplitter struct {
	minSize int
	maxSize int
}

// NewChunkSplitter 创建分块器
func NewChunkSplitter(minSize, maxSize int) *ChunkSplitter {
	return &ChunkSplitter{minSize: minSize, maxSize: maxSize}
}

// Chunk 分块结构
type Chunk struct {
	Index    int
	Heading  string
	Content  string
	FilePath string
}

// SplitByHeadings 按标题分块
func (s *ChunkSplitter) SplitByHeadings(body string, headings []Heading, filePath string) []Chunk {
	if len(headings) == 0 {
		return s.splitByParagraphs(body, filePath)
	}

	var chunks []Chunk
	lines := strings.Split(body, "\n")
	var currentSection []string
	var currentHeading Heading
	inSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		isHeading := false
		var heading Heading

		if strings.HasPrefix(trimmed, "### ") {
			isHeading = true
			heading = Heading{Level: 3, Text: strings.TrimPrefix(trimmed, "### ")}
		} else if strings.HasPrefix(trimmed, "## ") {
			isHeading = true
			heading = Heading{Level: 2, Text: strings.TrimPrefix(trimmed, "## ")}
		} else if strings.HasPrefix(trimmed, "# ") {
			isHeading = true
			heading = Heading{Level: 1, Text: strings.TrimPrefix(trimmed, "# ")}
		}

		if isHeading {
			// 保存当前 section
			if inSection && len(currentSection) > 0 {
				chunk := s.createChunk(currentSection, currentHeading, filePath, len(chunks))
				chunks = append(chunks, chunk...)
			}
			currentHeading = heading
			currentSection = []string{line}
			inSection = true
		} else {
			currentSection = append(currentSection, line)
		}
	}

	// 保存最后一个 section
	if inSection && len(currentSection) > 0 {
		chunk := s.createChunk(currentSection, currentHeading, filePath, len(chunks))
		chunks = append(chunks, chunk...)
	}

	return chunks
}

// createChunk 创建分块
func (s *ChunkSplitter) createChunk(lines []string, heading Heading, filePath string, baseIndex int) []Chunk {
	content := strings.TrimSpace(strings.Join(lines, "\n"))
	if content == "" {
		return nil
	}

	// 检查是否需要进一步分块
	if len(content) <= s.maxSize {
		return []Chunk{{
			Index:    baseIndex,
			Heading:  heading.Text,
			Content:  content,
			FilePath: filePath,
		}}
	}

	// 超过最大大小，按段落分块
	return s.splitByParagraphs(content, filePath)
}

// splitByParagraphs 按段落分块
func (s *ChunkSplitter) splitByParagraphs(body, filePath string) []Chunk {
	paragraphs := strings.Split(body, "\n\n")
	var chunks []Chunk
	var current strings.Builder
	index := 0

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		// 如果当前段落加上新段落会超过最大大小，先保存当前
		if current.Len()+len(para) > s.maxSize && current.Len() > 0 {
			chunkContent := current.String()
			if len(chunkContent) >= s.minSize {
				chunks = append(chunks, Chunk{
					Index:    index,
					Heading:  "",
					Content:  chunkContent,
					FilePath: filePath,
				})
				index++
			}
			current.Reset()
		}

		if current.Len() > 0 {
			current.WriteString("\n\n")
		}
		current.WriteString(para)
	}

	// 保存最后的段落
	if current.Len() > 0 {
		chunkContent := current.String()
		if len(chunkContent) >= s.minSize {
			chunks = append(chunks, Chunk{
				Index:    index,
				Heading:  "",
				Content:  chunkContent,
				FilePath: filePath,
			})
		}
	}

	return chunks
}

// parseSimpleYAML 解析简单 YAML（键值对）
func parseSimpleYAML(content string) map[string]string {
	result := make(map[string]string)
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// 匹配 key: value 或 key: "value" 或 key: 'value'
		kvPattern := regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_]*)\s*:\s*(.*)$`)
		matches := kvPattern.FindStringSubmatch(trimmed)
		if len(matches) >= 3 {
			key := matches[1]
			value := strings.TrimSpace(matches[2])
			// 去除引号
			value = strings.Trim(value, "\"")
			value = strings.Trim(value, "'")
			result[key] = value
		}
	}

	return result
}

// extractDomain 从 URL 提取域名
func extractDomain(url string) string {
	// 移除协议
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "www.")

	// 提取域名部分
	parts := strings.SplitN(url, "/", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// getStringFromMap 安全获取字符串
func getStringFromMap(m map[string]string, key string) string {
	if v, ok := m[key]; ok {
		return v
	}
	return ""
}

// getTagsFromMap 获取标签列表
func getTagsFromMap(m map[string]string) []string {
	tagsStr := m["tags"]
	if tagsStr == "" {
		return nil
	}

	// 支持逗号分隔或 YAML 列表格式
	if strings.Contains(tagsStr, ",") {
		parts := strings.Split(tagsStr, ",")
		var tags []string
		for _, p := range parts {
			t := strings.TrimSpace(p)
			if t != "" {
				tags = append(tags, t)
			}
		}
		return tags
	}

	return []string{tagsStr}
}

// getIntFromMap 安全获取整数
func getIntFromMap(m map[string]string, key string, defaultVal int) int {
	if v, ok := m[key]; ok {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultVal
}
