package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/pkg/response"
	"github.com/singll/bellkeeper/internal/service"
	"go.uber.org/zap"
)

// KnowledgeHandler 知识库 handler
type KnowledgeHandler struct {
	searchService *service.FileSearchService
	indexService  *service.KnowledgeIndexService
	askService    *service.AskService
}

// NewKnowledgeHandler 创建知识库 handler
func NewKnowledgeHandler(
	search *service.FileSearchService,
	index *service.KnowledgeIndexService,
	ask *service.AskService,
) *KnowledgeHandler {
	return &KnowledgeHandler{
		searchService: search,
		indexService:  index,
		askService:    ask,
	}
}

// Search 搜索
func (h *KnowledgeHandler) Search(c *gin.Context) {
	var req service.FileSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	result, err := h.searchService.Search(c.Request.Context(), req)
	if err != nil {
		response.InternalError(c, "search failed: "+err.Error())
		return
	}

	response.Success(c, result)
}

// Ask 问答
func (h *KnowledgeHandler) Ask(c *gin.Context) {
	var req service.AskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	result, err := h.askService.Ask(c.Request.Context(), req)
	if err != nil {
		response.InternalError(c, "ask failed: "+err.Error())
		return
	}

	response.Success(c, result)
}

// AskStream SSE 流式问答（1.0 §4 [fe] 知识问答 SSE 流式）。
// 客户端用 EventSource 消费，事件类型：references / delta / done / error。
func (h *KnowledgeHandler) AskStream(c *gin.Context) {
	var req service.AskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // Nginx 透传，禁缓冲

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		response.InternalError(c, "streaming not supported")
		return
	}

	ctx := c.Request.Context()
	ch := h.askService.AskStream(ctx, req)
	for chunk := range ch {
		data, err := json.Marshal(chunk.Data)
		if err != nil {
			middleware.GetLogger().Error("knowledge handler: marshal SSE data failed",
				zap.String("event", chunk.Type),
				zap.Error(err))
			continue
		}
		c.Writer.Write([]byte("event: " + chunk.Type + "\n"))
		c.Writer.Write([]byte("data: " + string(data) + "\n\n"))
		flusher.Flush()
	}
}

// Stats 获取索引统计
func (h *KnowledgeHandler) Stats(c *gin.Context) {
	stats, err := h.indexService.GetStats(c.Request.Context())
	if err != nil {
		response.InternalError(c, "get stats failed: "+err.Error())
		return
	}

	response.Success(c, stats)
}

// Rebuild 重建索引
func (h *KnowledgeHandler) Rebuild(c *gin.Context) {
	if err := h.indexService.FullScan(c.Request.Context()); err != nil {
		response.InternalError(c, "rebuild failed: "+err.Error())
		return
	}

	response.Message(c, "index rebuild completed")
}

// Health 健康检查
func (h *KnowledgeHandler) Health(c *gin.Context) {
	if err := h.indexService.GetHealth(c.Request.Context()); err != nil {
		response.InternalError(c, "meilisearch health check failed: "+err.Error())
		return
	}

	response.Success(c, gin.H{"status": "ok"})
}
