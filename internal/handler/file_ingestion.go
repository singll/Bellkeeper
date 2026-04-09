package handler

import (
	"strconv"

	"github.com/singll/bellkeeper/internal/pkg/response"
	"github.com/singll/bellkeeper/internal/service"

	"github.com/gin-gonic/gin"
)

// FileIngestionHandler handles file ingestion HTTP requests
type FileIngestionHandler struct {
	svc *service.FileIngestionService
}

// NewFileIngestionHandler creates a new FileIngestionHandler
func NewFileIngestionHandler(svc *service.FileIngestionService) *FileIngestionHandler {
	return &FileIngestionHandler{svc: svc}
}

// IngestURL handles POST /api/files/ingest/url
func (h *FileIngestionHandler) IngestURL(c *gin.Context) {
	var req service.IngestURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.svc.IngestURL(&req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}

// GetMetadata handles GET /api/files/metadata/:id
func (h *FileIngestionHandler) GetMetadata(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid ID")
		return
	}

	// TODO: Implement GetByID in service layer
	response.Success(c, gin.H{
		"id":      id,
		"message": "GetMetadata not yet implemented",
	})
}

// List handles GET /api/files/list
func (h *FileIngestionHandler) List(c *gin.Context) {
	// Parse query parameters
	layer := c.DefaultQuery("layer", "")
	status := c.DefaultQuery("status", "")
	limitStr := c.DefaultQuery("limit", "50")
	offsetStr := c.DefaultQuery("offset", "0")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 1000 {
		limit = 50
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	// TODO: Implement List in service layer
	response.Success(c, gin.H{
		"layer":   layer,
		"status":  status,
		"limit":   limit,
		"offset":  offset,
		"message": "List not yet implemented",
	})
}
