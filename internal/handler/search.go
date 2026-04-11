package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

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
		c.JSON(http.StatusBadRequest, gin.H{"error": "q parameter is required"})
		return
	}

	scope := c.DefaultQuery("scope", "all")
	if scope != "all" && scope != "tags" && scope != "documents" && scope != "rss" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scope must be one of: all, tags, documents, rss"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if limit <= 0 || limit > 100 {
		limit = 10
	}

	result, err := h.svc.Search(query, scope, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}
