package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/pkg/response"
	"github.com/singll/bellkeeper/internal/service"
)

// ClassifyHandler handles article classification requests
type ClassifyHandler struct {
	svc *service.ClassifyService
}

// NewClassifyHandler creates a new classify handler
func NewClassifyHandler(svc *service.ClassifyService) *ClassifyHandler {
	return &ClassifyHandler{svc: svc}
}

// ClassifyArticle handles POST /api/classify/article
func (h *ClassifyHandler) ClassifyArticle(c *gin.Context) {
	var req service.ClassifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	result, err := h.svc.ClassifyArticle(&req)
	if err != nil {
		// Return empty result on error (caller will use fallback)
		c.JSON(http.StatusOK, gin.H{
			"primary_domain": "",
			"tags":           []string{},
			"dataset_id":     "",
			"confidence":     0.0,
			"error":          err.Error(),
		})
		return
	}

	// Return flat JSON (not wrapped in "data") so n8n can read fields directly
	c.JSON(http.StatusOK, gin.H{
		"primary_domain": result.PrimaryDomain,
		"tags":           result.Tags,
		"dataset_id":     result.DatasetID,
		"confidence":     result.Confidence,
		"reasoning":      result.Reasoning,
	})
}
