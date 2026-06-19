package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/singll/bellkeeper/internal/pkg/meili"
)

// FileSearchRequest 搜索请求
type FileSearchRequest struct {
	Query        string   `json:"query"`
	Layers       []string `json:"layers,omitempty"`
	Categories   []string `json:"categories,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	SourceDomain string   `json:"source_domain,omitempty"`
	Limit        int      `json:"limit,omitempty"`
}

// FileHit 搜索命中结果
type FileHit struct {
	FileID        string   `json:"file_id"`
	FilePath      string   `json:"file_path"`
	Title         string   `json:"title"`
	AtomicConcept string   `json:"atomic_concept,omitempty"`
	Layer         string   `json:"layer"`
	Category      string   `json:"category"`
	Tags          []string `json:"tags"`
	SourceURL     string   `json:"source_url,omitempty"`
	SourceDomain  string   `json:"source_domain,omitempty"`
	UpdatedAt     int64    `json:"updated_at,omitempty"`
	Snippets      []string `json:"snippets"`
}

// FileSearchResult 搜索结果
type FileSearchResult struct {
	Files   []FileHit `json:"files"`
	Total   int64     `json:"total"`
	QueryMs int64     `json:"query_ms"`
}

// FileSearchService 搜索服务
type FileSearchService struct {
	meiliClient *meili.Client
}

// NewFileSearchService 创建搜索服务
func NewFileSearchService(meiliClient *meili.Client) *FileSearchService {
	return &FileSearchService{meiliClient: meiliClient}
}

// Search 搜索
func (s *FileSearchService) Search(ctx context.Context, req FileSearchRequest) (*FileSearchResult, error) {
	if req.Limit <= 0 {
		req.Limit = 10
	}

	// 构建 filter
	filters := s.buildFilters(req)

	// 构建 Meilisearch 请求
	meiliReq := &meili.SearchRequest{
		Limit:                  int64(req.Limit),
		Filter:                 strings.Join(filters, " AND "),
		AttributesToHighlight:  []string{"heading", "content"},
		HighlightPreTag:        "**",
		HighlightPostTag:       "**",
		AttributesToCrop:       []string{"content:150"},
		CropLength:             150,
	}

	resp, err := s.meiliClient.Search(ctx, req.Query, meiliReq)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	// 转换结果
	hits := make([]FileHit, 0, len(resp.Hits))
	seenFiles := make(map[string]bool)

	for _, hit := range resp.Hits {
		filePath := getStringValue(hit, "file_path")
		title := getStringValue(hit, "title")

		if seenFiles[filePath] {
			continue
		}
		seenFiles[filePath] = true

		// 提取高亮片段
		snippets := s.extractSnippets(hit)

		tags := []string{}
		if tagsVal, ok := hit["tags"].([]interface{}); ok {
			for _, t := range tagsVal {
				if ts, ok := t.(string); ok {
					tags = append(tags, ts)
				}
			}
		}

		h := FileHit{
			FileID:        filePath,
			FilePath:      filePath,
			Title:         title,
			AtomicConcept: getStringValue(hit, "atomic_concept"),
			Layer:         getStringValue(hit, "layer"),
			Category:      getStringValue(hit, "category"),
			Tags:          tags,
			SourceURL:     getStringValue(hit, "source_url"),
			SourceDomain:  getStringValue(hit, "source_domain"),
			UpdatedAt:     getInt64Value(hit, "updated_at"),
			Snippets:      snippets,
		}
		hits = append(hits, h)
	}

	return &FileSearchResult{
		Files:   hits,
		Total:   resp.EstimatedTotalHits,
		QueryMs: resp.ProcessingTimeMs,
	}, nil
}

// buildFilters 构建 filter 表达式
func (s *FileSearchService) buildFilters(req FileSearchRequest) []string {
	var filters []string

	if len(req.Layers) > 0 {
		quoted := make([]string, len(req.Layers))
		for i, l := range req.Layers {
			quoted[i] = fmt.Sprintf(`"%s"`, l)
		}
		filters = append(filters, fmt.Sprintf("layer IN [%s]", strings.Join(quoted, ",")))
	}

	if len(req.Categories) > 0 {
		quoted := make([]string, len(req.Categories))
		for i, c := range req.Categories {
			quoted[i] = fmt.Sprintf(`"%s"`, c)
		}
		filters = append(filters, fmt.Sprintf("category IN [%s]", strings.Join(quoted, ",")))
	}

	if req.SourceDomain != "" {
		filters = append(filters, fmt.Sprintf(`source_domain = "%s"`, req.SourceDomain))
	}

	return filters
}

// extractSnippets 提取高亮片段
func (s *FileSearchService) extractSnippets(m map[string]interface{}) []string {
	var snippets []string

	if formatted, ok := m["_formatted"].(map[string]interface{}); ok {
		if content, ok := formatted["content"].(string); ok && content != "" {
			snippets = append(snippets, content)
		}
		if heading, ok := formatted["heading"].(string); ok && heading != "" && heading != m["heading"] {
			snippets = append(snippets, heading)
		}
	}

	return snippets
}

// getInt64Value 安全获取整数值（处理 float64/json.Number/int 等 JSON 反序列化类型）
func getInt64Value(m map[string]interface{}, key string) int64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch typed := v.(type) {
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	case json.Number:
		if n, err := typed.Int64(); err == nil {
			return n
		}
	}
	return 0
}

// getStringValue 安全获取字符串值（处理 string、json.RawMessage 和 []byte 类型）
func getStringValue(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		switch typed := v.(type) {
		case string:
			return typed
		case json.RawMessage:
			return string(typed)
		case []byte:
			return string(typed)
		}
	}
	return ""
}
