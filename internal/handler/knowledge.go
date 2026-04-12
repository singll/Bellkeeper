package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/pkg/response"
	"github.com/singll/bellkeeper/internal/service"
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
