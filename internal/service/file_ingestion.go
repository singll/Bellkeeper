package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/defaults"
	"github.com/singll/bellkeeper/internal/repository"

	"gorm.io/gorm"
)

// FileIngestionService handles file ingestion from URLs
type FileIngestionService struct {
	cfg         config.FileIngestionConfig
	extractor   *ExtractorService
	datasetSvc  *DatasetService
	classifySvc *ClassifyService
	tagRepo     *repository.TagRepository
	articleRepo *repository.ArticleTagRepository
	activityLog *ActivityLogService
}

// IngestURLRequest represents a request to ingest a URL
type IngestURLRequest struct {
	URL      string   `json:"url" binding:"required"`
	Title    string   `json:"title"`
	Tags     []string `json:"tags"`
	Category string   `json:"category"`
	Layer    string   `json:"layer"`   // "raw" or "working"
	Content  string   `json:"content"` // 可选：传入已提取的内容，跳过 Extract 步骤
}

// IngestURLResponse represents the response from URL ingestion
type IngestURLResponse struct {
	Success       bool     `json:"success"`
	Status        string   `json:"status"` // "success", "duplicate", "duplicate_content", "extract_failed"
	FilePath      string   `json:"file_path,omitempty"`
	DocumentID    string   `json:"document_id,omitempty"`
	DatasetID     string   `json:"dataset_id,omitempty"`
	Title         string   `json:"title,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	Extractor     string   `json:"extractor,omitempty"`
	ExistingTitle string   `json:"existing_title,omitempty"` // for duplicate_content: title of existing article
	ExistingURL   string   `json:"existing_url,omitempty"`   // for duplicate_content: URL of existing article
	ErrorMessage  string   `json:"error_message,omitempty"`
}

// NewFileIngestionService creates a new FileIngestionService
func NewFileIngestionService(
	cfg config.FileIngestionConfig,
	extractor *ExtractorService,
	datasetSvc *DatasetService,
	classifySvc *ClassifyService,
	tagRepo *repository.TagRepository,
	articleRepo *repository.ArticleTagRepository,
	activityLog *ActivityLogService,
) *FileIngestionService {
	return &FileIngestionService{
		cfg:         cfg,
		extractor:   extractor,
		datasetSvc:  datasetSvc,
		classifySvc: classifySvc,
		tagRepo:     tagRepo,
		articleRepo: articleRepo,
		activityLog: activityLog,
	}
}

// ExtractURL extracts a URL's main content via the configured extractors and returns
// it WITHOUT ingesting — no URL/content dedup, no file write, no DB row. It backs the
// bare POST /api/files/extract endpoint that pkb-curate's gap-fill (ADR-0004 Phase G,
// Q7 V2 核实) uses to fetch an LLM-proposed authoritative source and check whether the
// page actually supports a drafted card.
func (s *FileIngestionService) ExtractURL(rawURL string) (*ExtractionResult, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, fmt.Errorf("extract: url is required")
	}
	result, err := s.extractor.Extract(&ExtractionRequest{URL: rawURL})
	if err != nil {
		return nil, fmt.Errorf("extract %s: %w", rawURL, err)
	}
	return result, nil
}

// IngestURL ingests content from a URL and saves it as a file
func (s *FileIngestionService) IngestURL(req *IngestURLRequest) (*IngestURLResponse, error) {
	// 1. URL 去重
	checkResult, err := s.datasetSvc.CheckURL(req.URL, true, false)
	if err != nil {
		return nil, fmt.Errorf("failed to check URL: %w", err)
	}
	if checkResult != nil && checkResult.Exists {
		s.logIngestion(req.URL, "duplicate", "URL already exists")
		return &IngestURLResponse{
			Success: false,
			Status:  "duplicate",
		}, nil
	}

	// 2. 提取正文（如果调用方已传入 Content，跳过 Extract）
	var extraction *ExtractionResult
	if req.Content != "" {
		extraction = &ExtractionResult{
			Content:   req.Content,
			Title:     req.Title,
			Extractor: "provided",
			Success:   true,
		}
	} else {
		extraction, err = s.extractor.Extract(&ExtractionRequest{URL: req.URL})
		if err != nil || !extraction.Success {
			errMsg := "extraction failed"
			if extraction != nil && extraction.Error != "" {
				errMsg = extraction.Error
			}
			s.logIngestion(req.URL, "extract_failed", errMsg)
			return &IngestURLResponse{
				Success:      false,
				Status:       "extract_failed",
				ErrorMessage: errMsg,
			}, fmt.Errorf("extraction failed: %s", errMsg)
		}
	}

	// Use extracted title if not provided
	if req.Title == "" {
		req.Title = extraction.Title
	}

	// 3. 内容哈希去重 — 在写入文件之前检查是否已有相同内容
	contentHash := s.calculateHash(extraction.Content)
	existingArticle, hashErr := s.articleRepo.GetByContentHash(contentHash)
	if hashErr == nil && existingArticle != nil {
		// 哈希匹配：内容已存在，跳过写入
		s.logIngestion(req.URL, "duplicate_content", fmt.Sprintf("Content hash matches existing article: %s (%s)", existingArticle.ArticleTitle, existingArticle.ArticleURL))
		return &IngestURLResponse{
			Success:       false,
			Status:        "duplicate_content",
			ExistingTitle: existingArticle.ArticleTitle,
			ExistingURL:   existingArticle.ArticleURL,
		}, nil
	} else if hashErr != nil && !errors.Is(hashErr, gorm.ErrRecordNotFound) {
		// DB 查询失败，不阻止入库，仅记录日志后继续
		s.logIngestion(req.URL, "hash_check_failed", fmt.Sprintf("Content hash DB check failed: %v", hashErr))
	}

	// 4. 分类标签（可选）
	datasetID := ""
	tagSource := "user"
	if s.classifySvc != nil && len(req.Tags) == 0 {
		classifyResult, err := s.classifySvc.ClassifyArticle(&ClassifyRequest{
			Title:   req.Title,
			URL:     req.URL,
			Content: extraction.Content,
		})
		if err == nil && classifyResult != nil {
			req.Tags = classifyResult.Tags
			datasetID = classifyResult.DatasetID
			tagSource = "llm"
			if req.Category == "" {
				req.Category = classifyResult.PrimaryDomain
			}
		}
	}
	req.Tags = normalizeTagList(req.Tags)

	tagRecords, err := s.getOrCreateTagRecords(req.Tags)
	if err != nil {
		s.logIngestion(req.URL, "tag_create_failed", fmt.Sprintf("tag creation failed: %v", err))
		tagRecords = nil
	}
	if datasetID == "" {
		datasetID = s.resolveDatasetID(req.Tags, req.Category)
	}

	// 5. 生成 frontmatter
	frontmatter := s.generateFrontmatter(req, extraction, tagSource)

	// 6. 生成文件名
	filename := s.generateFilename(req.Title)

	// 7. 确定路径
	layer := req.Layer
	if layer == "" {
		layer = s.cfg.DefaultLayer
	}
	if err := s.validateLayer(layer); err != nil {
		return nil, err
	}
	req.Layer = layer

	// Create layer directory if not exists
	layerDir, err := s.resolveLayerDir(layer)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(layerDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create layer directory: %w", err)
	}

	filePath := filepath.Join(layerDir, filename)

	// 8. 写入文件
	content := frontmatter + "\n\n" + extraction.Content
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	// 9. 内容哈希已在步骤 3 计算，直接使用

	// 10. 提取域名
	sourceDomain := s.extractDomain(req.URL)
	documentID := s.documentID(contentHash)
	primaryTagID := uint(0)
	if len(tagRecords) > 0 {
		primaryTagID = tagRecords[0].ID
	}
	metadata := map[string]interface{}{
		"tag_source": tagSource,
	}
	if len(req.Tags) > 0 {
		metadata["tags"] = req.Tags
	}
	metadataJSON, _ := json.Marshal(metadata)

	// 11. 记录元数据
	article := &model.ArticleTag{
		DocumentID:   documentID,
		DatasetID:    datasetID,
		TagID:        primaryTagID,
		ArticleURL:   req.URL,
		ArticleTitle: req.Title,
		FilePath:     filePath,
		Layer:        layer,
		Extractor:    extraction.Extractor,
		IngestStatus: "ingested",
		IndexStatus:  "pending",
		ContentHash:  contentHash,
		SourceDomain: sourceDomain,
		Metadata:     metadataJSON,
	}

	if err := s.articleRepo.Create(article); err != nil {
		// File created but DB record failed - log warning but don't fail
		s.logIngestion(req.URL, "db_failed", fmt.Sprintf("file created but DB record failed: %v", err))
	} else if len(tagRecords) > 1 {
		// 仅当主标签(tagRecords[0])之外还有附加标签时才建附加行；
		// tagRecords 为空(文章无标签)时 tagRecords[1:] 会触发 slice bounds [1:0] panic，导致进程崩溃重启。
		s.createAdditionalArticleTagRows(article, tagRecords[1:])
	}

	// 12. 记录日志
	s.logIngestion(req.URL, "success", fmt.Sprintf("入库成功: %s (提取器: %s)", req.Title, extraction.Extractor))

	return &IngestURLResponse{
		Success:   true,
		Status:    "success",
		FilePath:  filePath,
		DatasetID: datasetID,
		Title:     req.Title,
		Tags:      req.Tags,
		Extractor: extraction.Extractor,
	}, nil
}

func (s *FileIngestionService) getOrCreateTagRecords(names []string) ([]model.Tag, error) {
	if s.tagRepo == nil || len(names) == 0 {
		return nil, nil
	}
	tags := make([]model.Tag, 0, len(names))
	for _, name := range names {
		tag, err := s.tagRepo.FindOrCreate(name, defaults.DefaultTagColor)
		if err != nil {
			return nil, err
		}
		tags = append(tags, *tag)
	}
	return tags, nil
}

func (s *FileIngestionService) resolveDatasetID(tags []string, category string) string {
	if s.datasetSvc != nil {
		if mapping, _, err := s.datasetSvc.RecommendByTags(tags, category); err == nil && mapping != nil && mapping.DatasetID != "" {
			return mapping.DatasetID
		}
	}
	if category != "" {
		return category
	}
	return "daily-digest"
}

func (s *FileIngestionService) documentID(contentHash string) string {
	if len(contentHash) >= 16 {
		return "doc_" + contentHash[:16]
	}
	if contentHash != "" {
		return "doc_" + contentHash
	}
	return fmt.Sprintf("doc_%d", time.Now().UnixNano())
}

func (s *FileIngestionService) createAdditionalArticleTagRows(base *model.ArticleTag, tags []model.Tag) {
	if s.articleRepo == nil || base == nil || len(tags) == 0 {
		return
	}
	for _, tag := range tags {
		row := &model.ArticleTag{
			DocumentID:   base.DocumentID,
			DatasetID:    base.DatasetID,
			TagID:        tag.ID,
			ArticleTitle: base.ArticleTitle,
			ArticleURL:   base.ArticleURL,
			CanonicalURL: base.CanonicalURL,
			ContentHash:  base.ContentHash,
			SourceDomain: base.SourceDomain,
			FilePath:     base.FilePath,
			Layer:        base.Layer,
			Extractor:    base.Extractor,
			IngestStatus: "tag_association",
			IndexStatus:  "skipped",
			Metadata:     base.Metadata,
		}
		if err := s.articleRepo.Create(row); err != nil {
			s.logIngestion(base.ArticleURL, "tag_link_failed", fmt.Sprintf("tag association failed for tag %d: %v", tag.ID, err))
		}
	}
}

func (s *FileIngestionService) validateLayer(layer string) error {
	if layer == "" {
		return fmt.Errorf("layer is required")
	}
	if filepath.IsAbs(layer) || filepath.Clean(layer) != layer || strings.Contains(layer, "/") || strings.Contains(layer, `\`) {
		return fmt.Errorf("invalid layer: %s", layer)
	}

	allowed := map[string]struct{}{
		"raw":     {},
		"archive": {},
		"vault":   {},
	}
	for _, configured := range []string{s.cfg.RawDir, s.cfg.WorkingDir, s.cfg.DefaultLayer} {
		if configured != "" {
			allowed[configured] = struct{}{}
		}
	}
	if _, ok := allowed[layer]; !ok {
		return fmt.Errorf("invalid layer: %s", layer)
	}
	return nil
}

func (s *FileIngestionService) resolveLayerDir(layer string) (string, error) {
	absBase, err := filepath.Abs(s.cfg.BasePath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve base path: %w", err)
	}
	absLayer, err := filepath.Abs(filepath.Join(absBase, layer))
	if err != nil {
		return "", fmt.Errorf("failed to resolve layer path: %w", err)
	}
	rel, err := filepath.Rel(absBase, absLayer)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("access denied: layer outside base directory")
	}

	if realBase, err := filepath.EvalSymlinks(absBase); err == nil {
		if realLayer, err := filepath.EvalSymlinks(absLayer); err == nil {
			realRel, relErr := filepath.Rel(realBase, realLayer)
			if relErr != nil || realRel == ".." || strings.HasPrefix(realRel, ".."+string(filepath.Separator)) || filepath.IsAbs(realRel) {
				return "", fmt.Errorf("access denied: layer outside base directory")
			}
		}
	}

	return absLayer, nil
}

// generateFrontmatter generates YAML frontmatter for the file
func (s *FileIngestionService) generateFrontmatter(req *IngestURLRequest, extraction *ExtractionResult, tagSource string) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("title: \"%s\"\n", escapeYAML(req.Title)))
	sb.WriteString(fmt.Sprintf("url: \"%s\"\n", escapeYAML(req.URL)))

	domain := s.extractDomain(req.URL)
	if domain != "" {
		sb.WriteString(fmt.Sprintf("source_domain: \"%s\"\n", domain))
	}

	if len(req.Tags) > 0 {
		sb.WriteString("tags: [")
		for i, tag := range req.Tags {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(tag)
		}
		sb.WriteString("]\n")
	}

	if tagSource != "" {
		sb.WriteString(fmt.Sprintf("tag_source: %s\n", tagSource))
	}

	if req.Category != "" {
		sb.WriteString(fmt.Sprintf("category: %s\n", req.Category))
	}

	sb.WriteString(fmt.Sprintf("extractor: %s\n", extraction.Extractor))
	sb.WriteString(fmt.Sprintf("ingested_at: %s\n", time.Now().Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("layer: %s\n", req.Layer))
	sb.WriteString("---")

	return sb.String()
}

// generateFilename generates a filename from the title
func (s *FileIngestionService) generateFilename(title string) string {
	// Time prefix
	timestamp := time.Now().Format("20060102")

	// Sanitize title
	sanitized := s.sanitizeTitle(title)
	if sanitized == "" {
		sanitized = "untitled"
	}

	return fmt.Sprintf("%s_%s.md", timestamp, sanitized)
}

// sanitizeTitle cleans a title for use in filename
func (s *FileIngestionService) sanitizeTitle(title string) string {
	// Remove special characters, keep alphanumeric, Chinese, spaces, and hyphens
	reg := regexp.MustCompile(`[^a-zA-Z0-9\p{Han}\s\-]`)
	cleaned := reg.ReplaceAllString(title, "")

	// Replace spaces with hyphens
	cleaned = strings.ReplaceAll(cleaned, " ", "-")

	// Remove consecutive hyphens
	reg2 := regexp.MustCompile(`-+`)
	cleaned = reg2.ReplaceAllString(cleaned, "-")

	// Trim hyphens from start and end
	cleaned = strings.Trim(cleaned, "-")

	// Limit length to 80 runes (not bytes, to avoid breaking UTF-8)
	if runeLen := len([]rune(cleaned)); runeLen > 80 {
		cleaned = string([]rune(cleaned)[:80])
	}

	return cleaned
}

// extractDomain extracts the domain from a URL
func (s *FileIngestionService) extractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// calculateHash calculates SHA256 hash of content
func (s *FileIngestionService) calculateHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

// escapeYAML escapes special characters in YAML strings
func escapeYAML(s string) string {
	// Escape double quotes
	s = strings.ReplaceAll(s, "\"", "\\\"")
	// Escape backslashes
	s = strings.ReplaceAll(s, "\\", "\\\\")
	return s
}

// logIngestion logs ingestion activity
func (s *FileIngestionService) logIngestion(url, status, message string) {
	if s.activityLog == nil {
		return
	}

	s.activityLog.LogActivity(LogActivityParams{
		Module:  "file_ingestion",
		Action:  "ingest_url",
		Status:  status,
		Summary: message,
		Detail: map[string]interface{}{
			"url": url,
		},
	})
}

// GetMetadata retrieves metadata for a single file by ID
func (s *FileIngestionService) GetMetadata(id uint) (*model.ArticleTag, error) {
	return s.articleRepo.GetByIDWithPreload(id)
}

// ListFiles retrieves files with optional filtering and pagination
func (s *FileIngestionService) ListFiles(opts repository.ListArticleTagOpts) ([]model.ArticleTag, int64, error) {
	return s.articleRepo.ListWithFilter(opts)
}
