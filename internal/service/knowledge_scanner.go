package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

// ScanDirectory 递归扫描各 scan_dir（含子目录），返回所有 .md/.txt 文件。
// 改用 filepath.WalkDir 递归，使 vault/<领域>/x.md 等深层文件也能被索引到
// （原 os.ReadDir 仅扫顶层、遇子目录直接 continue）。
func (s *FileScanner) ScanDirectory() ([]FileInfo, error) {
	var files []FileInfo

	for _, dir := range s.scanDirs {
		dirPath := filepath.Join(s.basePath, dir.Path)
		layer := dir.Layer

		walkErr := filepath.WalkDir(dirPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				// 顶层目录不存在或单条目访问出错：跳过，不中断整体扫描
				return nil
			}
			if d.IsDir() {
				return nil
			}

			ext := strings.ToLower(filepath.Ext(d.Name()))
			if ext != ".md" && ext != ".txt" {
				return nil
			}

			content, err := os.ReadFile(path)
			if err != nil {
				return nil
			}

			hash := sha256.Sum256(content)

			// RelPath 取相对 basePath 的完整路径，保留子目录层级（如 vault/security/x.md）
			relPath, relErr := filepath.Rel(s.basePath, path)
			if relErr != nil {
				relPath = filepath.Join(dir.Path, d.Name())
			}

			// 从文件名提取标题（去掉扩展名）
			title := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))

			files = append(files, FileInfo{
				AbsPath:     path,
				RelPath:     relPath,
				Layer:       layer,
				Content:     content,
				ContentHash: hex.EncodeToString(hash[:]),
				Title:       title,
			})
			return nil
		})
		if walkErr != nil {
			return nil, fmt.Errorf("walk directory %s: %w", dir.Path, walkErr)
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
	Title         string
	SourceURL     string
	SourceDomain  string
	Category      string
	Tags          []string
	AtomicConcept string
	Body          string
	Headings      []Heading
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
		Title:         fileInfo.Title,
		Category:      getStringFromMap(fm, "category"),
		Tags:          normalizeTagList(getTagsFromMap(fm)),
		AtomicConcept: getStringFromMap(fm, "atomic_concept"),
		Body:          body,
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
	currentListKey := ""

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") && currentListKey != "" {
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			value = strings.Trim(value, "\"'")
			if value != "" {
				if result[currentListKey] != "" {
					result[currentListKey] += ","
				}
				result[currentListKey] += value
			}
			continue
		}

		// 匹配 key: value 或 key: "value" 或 key: 'value'
		kvPattern := regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_]*)\s*:\s*(.*)$`)
		matches := kvPattern.FindStringSubmatch(trimmed)
		if len(matches) >= 3 {
			key := matches[1]
			value := strings.TrimSpace(matches[2])
			currentListKey = ""
			// 去除引号
			value = strings.Trim(value, "\"")
			value = strings.Trim(value, "'")
			result[key] = value
			if value == "" {
				currentListKey = key
			}
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
	tagsStr := strings.TrimSpace(m["tags"])
	if tagsStr == "" {
		return nil
	}

	tagsStr = strings.Trim(tagsStr, "[]")

	// 支持逗号分隔、内联数组或简单 YAML 列表格式
	if strings.Contains(tagsStr, ",") {
		parts := strings.Split(tagsStr, ",")
		var tags []string
		for _, p := range parts {
			t := cleanFrontmatterTag(p)
			if t != "" {
				tags = append(tags, t)
			}
		}
		return tags
	}

	if tag := cleanFrontmatterTag(tagsStr); tag != "" {
		return []string{tag}
	}
	return nil
}

func cleanFrontmatterTag(tag string) string {
	tag = strings.TrimSpace(tag)
	tag = strings.Trim(tag, "[]\"'`")
	tag = strings.TrimSpace(tag)
	return tag
}


