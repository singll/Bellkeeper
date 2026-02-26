package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/service"
)

type DatasetHandler struct {
	svc *service.DatasetService
}

func NewDatasetHandler(svc *service.DatasetService) *DatasetHandler {
	return &DatasetHandler{svc: svc}
}

type DatasetRequest struct {
	Name        string `json:"name" binding:"required"`
	DisplayName string `json:"display_name"`
	DatasetID   string `json:"dataset_id" binding:"required"`
	Description string `json:"description"`
	IsDefault   *bool  `json:"is_default"`
	IsActive    *bool  `json:"is_active"`
	ParserID    string `json:"parser_id"`
	TagIDs      []uint `json:"tag_ids"`
}

func (h *DatasetHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))

	mappings, total, err := h.svc.List(page, perPage)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":     mappings,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

func (h *DatasetHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	mapping, err := h.svc.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "dataset mapping not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": mapping})
}

func (h *DatasetHandler) Create(c *gin.Context) {
	var req DatasetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	isDefault := false
	if req.IsDefault != nil {
		isDefault = *req.IsDefault
	}

	mapping := &model.DatasetMapping{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		DatasetID:   req.DatasetID,
		Description: req.Description,
		IsDefault:   isDefault,
		IsActive:    isActive,
		ParserID:    req.ParserID,
	}

	if mapping.ParserID == "" {
		mapping.ParserID = "naive"
	}

	if err := h.svc.Create(mapping, req.TagIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": mapping})
}

func (h *DatasetHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	mapping, err := h.svc.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "dataset mapping not found"})
		return
	}

	var req DatasetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	mapping.Name = req.Name
	mapping.DisplayName = req.DisplayName
	mapping.DatasetID = req.DatasetID
	mapping.Description = req.Description
	if req.IsDefault != nil {
		mapping.IsDefault = *req.IsDefault
	}
	if req.IsActive != nil {
		mapping.IsActive = *req.IsActive
	}
	mapping.ParserID = req.ParserID

	if err := h.svc.Update(mapping, req.TagIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": mapping})
}

func (h *DatasetHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.svc.Delete(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// --- Batch C: 高级端点 ---

// GetAll returns all active dataset mappings without pagination (dict + list for workflows)
func (h *DatasetHandler) GetAll(c *gin.Context) {
	mappings, err := h.svc.GetAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Build dict format (name -> mapping) for workflow convenience
	dict := make(map[string]interface{})
	for _, m := range mappings {
		dict[m.Name] = m
	}

	c.JSON(http.StatusOK, gin.H{
		"data": mappings,
		"dict": dict,
	})
}

// GetByName returns a dataset mapping by its name
func (h *DatasetHandler) GetByName(c *gin.Context) {
	name := c.Param("name")
	mapping, err := h.svc.GetByName(name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "dataset mapping not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": mapping})
}

// RecommendByTag recommends a dataset based on tags with fallback logic
func (h *DatasetHandler) RecommendByTag(c *gin.Context) {
	var req struct {
		Tags     []string `json:"tags"`
		Category string   `json:"category"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	mapping, matchType, err := h.svc.RecommendByTags(req.Tags, req.Category)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no matching dataset found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":       mapping,
		"match_type": matchType,
	})
}

// AddArticleTags creates article-tag associations
func (h *DatasetHandler) AddArticleTags(c *gin.Context) {
	var req struct {
		DocumentID string `json:"document_id" binding:"required"`
		DatasetID  string `json:"dataset_id" binding:"required"`
		TagIDs     []uint `json:"tag_ids" binding:"required"`
		Title      string `json:"title"`
		URL        string `json:"url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	created, err := h.svc.AddArticleTags(req.DocumentID, req.DatasetID, req.TagIDs, req.Title, req.URL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": created})
}

// GetArticleTags returns tags for a given document
func (h *DatasetHandler) GetArticleTags(c *gin.Context) {
	documentID := c.Param("document_id")
	ats, err := h.svc.GetArticleTags(documentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": ats})
}

// GetArticlesByTag returns paginated articles for a given tag
func (h *DatasetHandler) GetArticlesByTag(c *gin.Context) {
	tagID, err := strconv.ParseUint(c.Param("tag_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid tag_id"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))

	ats, total, err := h.svc.GetArticlesByTag(uint(tagID), page, perPage)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":     ats,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}
