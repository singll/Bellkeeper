package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/pkg/response"
	"github.com/singll/bellkeeper/internal/service"
)

// SearchHandler handles search requests
type SearchHandler struct {
	svc *service.SearchService
}

// NewSearchHandler creates a new SearchHandler
func NewSearchHandler(svc *service.SearchService) *SearchHandler {
	return &SearchHandler{svc: svc}
}

// Search handles GET /api/search
func (h *SearchHandler) Search(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		response.BadRequest(c, "q parameter is required")
		return
	}

	scope := c.DefaultQuery("scope", "all")
	if scope != "all" && scope != "tags" && scope != "documents" && scope != "rss" {
		response.BadRequest(c, "scope must be one of: all, tags, documents, rss")
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	result, err := h.svc.Search(query, scope, limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}
