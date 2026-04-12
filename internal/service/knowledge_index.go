package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/pkg/meili"
)

// KnowledgeIndexService 知识库索引服务
type KnowledgeIndexService struct {
	cfg         config.KnowledgeConfig
	meiliClient *meili.Client
	scanner     *FileScanner
	parser      *MarkdownParser
	splitter    *ChunkSplitter

	// Goroutine 生命周期管理
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewKnowledgeIndexService 创建索引服务
func NewKnowledgeIndexService(
	cfg config.KnowledgeConfig,
	meiliClient *meili.Client,
) *KnowledgeIndexService {
	return &KnowledgeIndexService{
		cfg:         cfg,
		meiliClient: meiliClient,
		scanner:     NewFileScanner(cfg),
		parser:      NewMarkdownParser(),
		splitter:    NewChunkSplitter(cfg.ChunkMinSize, cfg.ChunkMaxSize),
		stopCh:     make(chan struct{}),
	}
}

// StartFullScan 启动全量扫描（后台执行）
func (s *KnowledgeIndexService) StartFullScan(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		log.Println("[KnowledgeIndex] starting full scan...")

		if err := s.FullScan(ctx); err != nil {
			log.Printf("[KnowledgeIndex] full scan failed: %v", err)
		} else {
			log.Println("[KnowledgeIndex] full scan completed")
		}
	}()
}

// FullScan 全量索引
func (s *KnowledgeIndexService) FullScan(ctx context.Context) error {
	files, err := s.scanner.ScanDirectory()
	if err != nil {
		return fmt.Errorf("scan directory: %w", err)
	}

	log.Printf("[KnowledgeIndex] found %d files to index", len(files))

	if len(files) == 0 {
		return nil
	}

	// 配置索引
	if err := s.meiliClient.ConfigureIndex(ctx); err != nil {
		log.Printf("[KnowledgeIndex] configure index: %v", err)
	}

	// 批量索引
	return s.indexFiles(ctx, files)
}

// StartIncrementalScan 启动增量扫描
func (s *KnowledgeIndexService) StartIncrementalScan(ctx context.Context) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		// 首次等待 30 秒，让服务完全启动
		firstTick := time.NewTimer(30 * time.Second)
		defer firstTick.Stop()

		interval := time.Duration(s.cfg.ScanInterval) * time.Second
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-firstTick.C:
				if err := s.IncrementalScan(ctx); err != nil {
					log.Printf("[KnowledgeIndex] incremental scan failed: %v", err)
				}
			case <-ticker.C:
				if err := s.IncrementalScan(ctx); err != nil {
					log.Printf("[KnowledgeIndex] incremental scan failed: %v", err)
				}
			case <-s.stopCh:
				log.Println("[KnowledgeIndex] incremental scan stopped")
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// IncrementalScan 增量扫描
func (s *KnowledgeIndexService) IncrementalScan(ctx context.Context) error {
	files, err := s.scanner.ScanDirectory()
	if err != nil {
		return fmt.Errorf("scan directory: %w", err)
	}

	if len(files) == 0 {
		return nil
	}

	return s.indexFiles(ctx, files)
}

// Stop 停止后台任务
func (s *KnowledgeIndexService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

// indexFiles 索引文件列表（带并发控制）
func (s *KnowledgeIndexService) indexFiles(ctx context.Context, files []FileInfo) error {
	const batchSize = 10
	sem := make(chan struct{}, batchSize)
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var lastErr error

	for _, file := range files {
		wg.Add(1)
		sem <- struct{}{}

		go func(f FileInfo) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := s.indexFile(ctx, &f); err != nil {
				errMu.Lock()
				lastErr = err
				errMu.Unlock()
				log.Printf("[KnowledgeIndex] index file %s: %v", f.RelPath, err)
			}
		}(file)
	}

	wg.Wait()
	return lastErr
}

// indexFile 索引单个文件
func (s *KnowledgeIndexService) indexFile(ctx context.Context, file *FileInfo) error {
	parsed, err := s.parser.ParseMarkdown(file)
	if err != nil {
		return fmt.Errorf("parse markdown: %w", err)
	}

	chunks := s.splitter.SplitByHeadings(parsed.Body, parsed.Headings, file.RelPath)

	if len(chunks) == 0 {
		log.Printf("[KnowledgeIndex] skip file %s: no chunks", file.RelPath)
		return nil
	}

	// 生成安全的文档 ID（只包含字母、数字、连字符和下划线）
	safeID := sanitizeDocumentID(file.RelPath)

	// 构建 Meilisearch 文档
	docs := make([]map[string]interface{}, 0, len(chunks))
	for i, chunk := range chunks {
		doc := map[string]interface{}{
			"id":            fmt.Sprintf("%s_%d", safeID, i),
			"file_path":     file.RelPath,
			"file_id":       file.RelPath,
			"chunk_index":   i,
			"heading":       chunk.Heading,
			"content":       chunk.Content,
			"title":         parsed.Title,
			"layer":         file.Layer,
			"category":      parsed.Category,
			"tags":          parsed.Tags,
			"source_url":    parsed.SourceURL,
			"source_domain": parsed.SourceDomain,
			"content_hash":  file.ContentHash,
			"updated_at":    time.Now().Unix(),
		}
		docs = append(docs, doc)
	}

	log.Printf("[KnowledgeIndex] indexing file %s: %d chunks", file.RelPath, len(docs))

	return s.meiliClient.AddDocuments(ctx, docs)
}

// sanitizeDocumentID 生成安全的 Meilisearch 文档 ID
// 只允许: a-z, A-Z, 0-9, -, _
func sanitizeDocumentID(path string) string {
	// 去掉扩展名
	path = strings.TrimSuffix(path, ".md")
	path = strings.TrimSuffix(path, ".txt")

	var result strings.Builder
	for _, r := range path {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			result.WriteRune(r)
		} else if r == '/' || r == '\\' {
			result.WriteRune('_')
		} else {
			// 中文或其他字符，用其字节的十六进制表示
			result.WriteString(fmt.Sprintf("_%X", r))
		}
	}
	return result.String()
}

// IndexFile 索引单个文件（供外部调用）
func (s *KnowledgeIndexService) IndexFile(ctx context.Context, filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	relPath, err := filepath.Rel(s.cfg.BasePath, filePath)
	if err != nil {
		return fmt.Errorf("get relative path: %w", err)
	}

	// 确定 layer
	layer := s.getLayerFromPath(relPath)

	hash := sha256.Sum256(content)
	title := extractTitleFromPath(filePath)

	fileInfo := &FileInfo{
		AbsPath:     filePath,
		RelPath:     relPath,
		Layer:       layer,
		Content:     content,
		ContentHash: fmt.Sprintf("%x", hash),
		Title:       title,
	}

	return s.indexFile(ctx, fileInfo)
}

// getLayerFromPath 从路径推断 layer
func (s *KnowledgeIndexService) getLayerFromPath(relPath string) string {
	for _, dir := range s.cfg.ScanDirs {
		if dir.Path != "" && len(relPath) >= len(dir.Path) && relPath[:len(dir.Path)] == dir.Path {
			return dir.Layer
		}
	}
	return "unknown"
}

// GetStats 获取索引统计
func (s *KnowledgeIndexService) GetStats(ctx context.Context) (map[string]interface{}, error) {
	stats, err := s.meiliClient.GetStats(ctx)
	if err != nil {
		return nil, fmt.Errorf("get stats: %w", err)
	}

	return map[string]interface{}{
		"number_of_documents": stats.NumberOfDocuments,
		"is_indexing":        stats.IsIndexing,
		"field_distribution":  stats.FieldDistribution,
	}, nil
}

// GetHealth 检查服务健康状态
func (s *KnowledgeIndexService) GetHealth(ctx context.Context) error {
	return s.meiliClient.Health(ctx)
}

// extractTitleFromPath 从路径提取标题
func extractTitleFromPath(path string) string {
	name := filepath.Base(path)
	ext := filepath.Ext(name)
	return strings.TrimSuffix(name, ext)
}
