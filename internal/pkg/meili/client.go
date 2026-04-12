package meili

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/meilisearch/meilisearch-go"
)

// Client Meilisearch 客户端封装
type Client struct {
	client meilisearch.ServiceManager
	index  string
}

// NewClient 创建 Meilisearch 客户端
func NewClient(url, apiKey, indexName string) (*Client, error) {
	client := meilisearch.New(url, meilisearch.WithAPIKey(apiKey))

	// 验证连接
	if !client.IsHealthy() {
		return nil, fmt.Errorf("meilisearch is not healthy")
	}

	// 尝试创建索引（如果不存在会返回错误，可以忽略）
	_, err := client.CreateIndex(&meilisearch.IndexConfig{
		Uid:        indexName,
		PrimaryKey: "id",
	})
	if err != nil {
		// 索引可能已存在，忽略错误
		log.Printf("[Meili] create index: %v", err)
	}

	return &Client{
		client: client,
		index:  indexName,
	}, nil
}

// Index 获取索引操作接口
func (c *Client) Index() meilisearch.IndexManager {
	return c.client.Index(c.index)
}

// ConfigureIndex 配置索引属性（同步等待完成）
func (c *Client) ConfigureIndex(ctx context.Context) error {
	index := c.Index()

	// 设置可搜索属性
	searchableAttrs := []string{"heading", "content", "title"}
	taskInfo, err := index.UpdateSearchableAttributes(&searchableAttrs)
	if err != nil {
		return fmt.Errorf("update searchable attributes: %w", err)
	}
	if err := c.waitForTask(ctx, taskInfo.TaskUID); err != nil {
		return fmt.Errorf("wait for searchable attributes: %w", err)
	}

	// 设置可过滤属性
	filterableAttrs := []interface{}{"layer", "category", "tags", "source_domain", "file_path"}
	taskInfo, err = index.UpdateFilterableAttributes(&filterableAttrs)
	if err != nil {
		return fmt.Errorf("update filterable attributes: %w", err)
	}
	if err := c.waitForTask(ctx, taskInfo.TaskUID); err != nil {
		return fmt.Errorf("wait for filterable attributes: %w", err)
	}

	// 设置可排序属性
	sortableAttrs := []string{"updated_at"}
	taskInfo, err = index.UpdateSortableAttributes(&sortableAttrs)
	if err != nil {
		return fmt.Errorf("update sortable attributes: %w", err)
	}
	if err := c.waitForTask(ctx, taskInfo.TaskUID); err != nil {
		return fmt.Errorf("wait for sortable attributes: %w", err)
	}

	// 设置排名规则
	rankingRules := []string{"words", "typo", "proximity", "attribute", "sort", "exactness"}
	taskInfo, err = index.UpdateRankingRules(&rankingRules)
	if err != nil {
		return fmt.Errorf("update ranking rules: %w", err)
	}
	if err := c.waitForTask(ctx, taskInfo.TaskUID); err != nil {
		return fmt.Errorf("wait for ranking rules: %w", err)
	}

	log.Printf("[Meili] index %s configured successfully", c.index)
	return nil
}

// waitForTask 等待任务完成
func (c *Client) waitForTask(ctx context.Context, taskUID int64) error {
	// 使用 30 秒超时
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("timeout waiting for task %d", taskUID)
		case <-ticker.C:
			task, err := c.client.GetTask(taskUID)
			if err != nil {
				return fmt.Errorf("get task %d: %w", taskUID, err)
			}
			switch task.Status {
			case meilisearch.TaskStatusSucceeded:
				log.Printf("[Meili] task %d succeeded", taskUID)
				return nil
			case meilisearch.TaskStatusFailed:
				log.Printf("[Meili] task %d failed: %+v", taskUID, task.Error)
				return fmt.Errorf("task %d failed: %+v", taskUID, task.Error)
			case meilisearch.TaskStatusProcessing:
				log.Printf("[Meili] task %d processing...", taskUID)
			}
		}
	}
}

// AddDocuments 添加文档
func (c *Client) AddDocuments(ctx context.Context, docs []map[string]interface{}) error {
	if len(docs) == 0 {
		return nil
	}

	log.Printf("[Meili] AddDocuments: %d docs, first ID: %v", len(docs), docs[0]["id"])

	taskInfo, err := c.Index().AddDocuments(docs, nil)
	if err != nil {
		return fmt.Errorf("add documents: %w", err)
	}

	log.Printf("[Meili] AddDocuments: task UID %d created, waiting...", taskInfo.TaskUID)

	// 等待文档添加完成
	if err := c.waitForTask(ctx, taskInfo.TaskUID); err != nil {
		return fmt.Errorf("wait for document addition: %w", err)
	}

	log.Printf("[Meili] AddDocuments: %d docs indexed successfully", len(docs))

	return nil
}

// SearchRequest 搜索请求参数
type SearchRequest struct {
	Limit                 int64
	Filter               string
	AttributesToHighlight []string
	HighlightPreTag       string
	HighlightPostTag     string
	AttributesToCrop     []string
	CropLength           int64
}

// SearchResponse 搜索响应
type SearchResponse struct {
	Hits               []map[string]interface{} `json:"hits"`
	EstimatedTotalHits int64                    `json:"estimatedTotalHits"`
	ProcessingTimeMs  int64                    `json:"processingTimeMs"`
}

// Search 搜索
func (c *Client) Search(ctx context.Context, query string, req *SearchRequest) (*SearchResponse, error) {
	searchReq := &meilisearch.SearchRequest{
		Limit:                 req.Limit,
		Filter:                req.Filter,
		AttributesToHighlight: req.AttributesToHighlight,
		HighlightPreTag:      req.HighlightPreTag,
		HighlightPostTag:     req.HighlightPostTag,
		AttributesToCrop:     req.AttributesToCrop,
		CropLength:           req.CropLength,
	}

	resp, err := c.Index().Search(query, searchReq)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	// 转换 hits - Meilisearch SDK 返回 json.RawMessage 类型，需要转换为 string
	hits := make([]map[string]interface{}, 0, len(resp.Hits))
	for _, hit := range resp.Hits {
		m := make(map[string]interface{}, len(hit))
		for k, v := range hit {
			m[k] = convertValue(v)
		}
		hits = append(hits, m)
	}

	log.Printf("[Meili] Search returned %d hits, total: %d", len(hits), resp.EstimatedTotalHits)

	return &SearchResponse{
		Hits:               hits,
		EstimatedTotalHits: resp.EstimatedTotalHits,
		ProcessingTimeMs:  resp.ProcessingTimeMs,
	}, nil
}

// getMapKeys 获取 map 的所有 key
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// DeleteDocument 删除单个文档
func (c *Client) DeleteDocument(ctx context.Context, id string) error {
	_, err := c.Index().DeleteDocument(id, nil)
	if err != nil {
		return fmt.Errorf("delete document %s: %w", id, err)
	}
	return nil
}

// DeleteDocuments 删除多个文档
func (c *Client) DeleteDocuments(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := c.Index().DeleteDocuments(ids, nil)
	if err != nil {
		return fmt.Errorf("delete documents: %w", err)
	}
	return nil
}

// DeleteAllDocuments 删除所有文档
func (c *Client) DeleteAllDocuments(ctx context.Context) error {
	_, err := c.Index().DeleteAllDocuments(nil)
	if err != nil {
		return fmt.Errorf("delete all documents: %w", err)
	}
	return nil
}

// StatsIndex 索引统计
type StatsIndex struct {
	NumberOfDocuments int64              `json:"numberOfDocuments"`
	IsIndexing       bool               `json:"isIndexing"`
	FieldDistribution map[string]int64   `json:"fieldDistribution"`
}

// GetStats 获取索引统计
func (c *Client) GetStats(ctx context.Context) (*StatsIndex, error) {
	stats, err := c.Index().GetStats()
	if err != nil {
		return nil, fmt.Errorf("get stats: %w", err)
	}

	return &StatsIndex{
		NumberOfDocuments: stats.NumberOfDocuments,
		IsIndexing:       stats.IsIndexing,
		FieldDistribution: stats.FieldDistribution,
	}, nil
}

// Health 检查健康状态
func (c *Client) Health(ctx context.Context) error {
	if !c.client.IsHealthy() {
		return fmt.Errorf("meilisearch is not healthy")
	}
	return nil
}

// convertValue 转换 Meilisearch SDK 返回的值类型
// Meilisearch SDK 返回 json.RawMessage 和 []byte 类型，需要转换为 string
func convertValue(v interface{}) interface{} {
	switch val := v.(type) {
	case string:
		return val
	case json.RawMessage:
		return string(val)
	case []byte:
		return string(val)
	default:
		return val
	}
}
