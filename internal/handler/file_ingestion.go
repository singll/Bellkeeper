package handler

import (
	"strconv"

	"github.com/singll/bellkeeper/internal/pkg/response"
	"github.com/singll/bellkeeper/internal/repository"
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

	article, err := h.svc.GetMetadata(uint(id))
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, article)
}

// List handles GET /api/files/list
func (h *FileIngestionHandler) List(c *gin.Context) {
	// Parse query parameters
	layer := c.DefaultQuery("layer", "")
	status := c.DefaultQuery("status", "")
	keyword := c.DefaultQuery("keyword", "")
	pageStr := c.DefaultQuery("page", "1")
	perPageStr := c.DefaultQuery("per_page", "20")

	page, _ := strconv.Atoi(pageStr)
	perPage, _ := strconv.Atoi(perPageStr)
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 1000 {
		perPage = 20
	}

	opts := repository.ListArticleTagOpts{
		Layer:   layer,
		Status:  status,
		Keyword: keyword,
		Page:    page,
		PerPage: perPage,
	}

	articles, total, err := h.svc.ListFiles(opts)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Page(c, articles, total, page, perPage)
}
