package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/service"
)

type RagFlowHandler struct {
	svc *service.RagFlowService
}

func NewRagFlowHandler(svc *service.RagFlowService) *RagFlowHandler {
	return &RagFlowHandler{svc: svc}
}

func (h *RagFlowHandler) Upload(c *gin.Context) {
	var req service.UploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, err := h.svc.Upload(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": resp})
}

func (h *RagFlowHandler) UploadWithRouting(c *gin.Context) {
	var req service.UploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resp, datasetID, err := h.svc.UploadWithRouting(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":       resp,
		"dataset_id": datasetID,
	})
}

func (h *RagFlowHandler) CheckURL(c *gin.Context) {
	url := c.Query("url")
	if url == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url parameter required"})
		return
	}

	exists, err := h.svc.CheckURL(url)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"exists": exists})
}

func (h *RagFlowHandler) ListDocuments(c *gin.Context) {
	datasetID := c.Query("dataset_id")
	if datasetID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dataset_id parameter required"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	result, err := h.svc.ListDocuments(datasetID, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *RagFlowHandler) DeleteDocument(c *gin.Context) {
	documentID := c.Param("id")
	datasetID := c.Query("dataset_id")
	if datasetID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "dataset_id parameter required"})
		return
	}

	if err := h.svc.DeleteDocument(datasetID, documentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
