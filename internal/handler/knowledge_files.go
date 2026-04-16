package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/service"
)

// KnowledgeFilesHandler handles file browsing requests
type KnowledgeFilesHandler struct {
	filesSvc *service.KnowledgeFilesService
}

// NewKnowledgeFilesHandler creates a new knowledge files handler
func NewKnowledgeFilesHandler(filesSvc *service.KnowledgeFilesService) *KnowledgeFilesHandler {
	return &KnowledgeFilesHandler{filesSvc: filesSvc}
}

// GetTree handles GET /api/knowledge/files/tree
// Query params:
//   - path: relative path within knowledge base (default: "")
func (h *KnowledgeFilesHandler) GetTree(c *gin.Context) {
	path := c.DefaultQuery("path", "")

	tree, err := h.filesSvc.GetTree(path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": tree})
}

// ListFiles handles GET /api/knowledge/files/list
// Query params:
//   - path: relative path within knowledge base (default: "")
//   - layer: filter by layer (optional)
func (h *KnowledgeFilesHandler) ListFiles(c *gin.Context) {
	path := c.DefaultQuery("path", "")
	layer := c.Query("layer")

	files, err := h.filesSvc.ListFiles(path, layer)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": files})
}

// ReadFile handles GET /api/knowledge/files/read
// Query params:
//   - path: relative path to the file (required)
func (h *KnowledgeFilesHandler) ReadFile(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "path parameter is required"})
		return
	}

	content, err := h.filesSvc.ReadFile(path)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": content})
}

// GetStats handles GET /api/knowledge/files/stats
func (h *KnowledgeFilesHandler) GetStats(c *gin.Context) {
	stats, err := h.filesSvc.GetStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": stats})
}

// SearchFiles handles GET /api/knowledge/files/search
// Query params:
//   - q: search pattern (required)
func (h *KnowledgeFilesHandler) SearchFiles(c *gin.Context) {
	pattern := c.Query("q")
	if pattern == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "q parameter is required"})
		return
	}

	files, err := h.filesSvc.SearchFiles(pattern)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": files})
}
