package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/defaults"
	"github.com/singll/bellkeeper/internal/pkg/response"
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
	page, perPage := response.ParsePagination(c)

	mappings, total, err := h.svc.List(page, perPage)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Page(c, mappings, total, page, perPage)
}

func (h *DatasetHandler) Get(c *gin.Context) {
	id, ok := response.ParseID(c, "id")
	if !ok {
		return
	}

	mapping, err := h.svc.GetByID(id)
	if err != nil {
		response.NotFound(c, "dataset mapping not found")
		return
	}

	response.Success(c, mapping)
}

func (h *DatasetHandler) Create(c *gin.Context) {
	var req DatasetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
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
		mapping.ParserID = defaults.DefaultParserID
	}

	if err := h.svc.Create(mapping, req.TagIDs); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, mapping)
}

func (h *DatasetHandler) Update(c *gin.Context) {
	id, ok := response.ParseID(c, "id")
	if !ok {
		return
	}

	mapping, err := h.svc.GetByID(id)
	if err != nil {
		response.NotFound(c, "dataset mapping not found")
		return
	}

	var req DatasetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
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
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, mapping)
}

func (h *DatasetHandler) Delete(c *gin.Context) {
	id, ok := response.ParseID(c, "id")
	if !ok {
		return
	}

	if err := h.svc.Delete(id); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Deleted(c)
}

// --- Batch C: 高级端点 ---

// GetAll returns all active dataset mappings without pagination (dict + list for workflows)
func (h *DatasetHandler) GetAll(c *gin.Context) {
	mappings, err := h.svc.GetAll()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

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
		response.NotFound(c, "dataset mapping not found")
		return
	}

	response.Success(c, mapping)
}

// RecommendByTag recommends a dataset based on tags with fallback logic
func (h *DatasetHandler) RecommendByTag(c *gin.Context) {
	var req struct {
		Tags     []string `json:"tags"`
		Category string   `json:"category"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	mapping, matchType, err := h.svc.RecommendByTags(req.Tags, req.Category)
	if err != nil {
		response.NotFound(c, "no matching dataset found")
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
		response.BadRequest(c, err.Error())
		return
	}

	created, err := h.svc.AddArticleTags(req.DocumentID, req.DatasetID, req.TagIDs, req.Title, req.URL)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, created)
}

// GetArticleTags returns tags for a given document
func (h *DatasetHandler) GetArticleTags(c *gin.Context) {
	documentID := c.Param("document_id")
	ats, err := h.svc.GetArticleTags(documentID)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, ats)
}

// GetArticlesByTag returns paginated articles for a given tag
func (h *DatasetHandler) GetArticlesByTag(c *gin.Context) {
	tagID, ok := response.ParseID(c, "tag_id")
	if !ok {
		return
	}

	page, perPage := response.ParsePagination(c)

	ats, total, err := h.svc.GetArticlesByTag(tagID, page, perPage)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Page(c, ats, total, page, perPage)
}

// CheckURL checks if a URL exists in the local ArticleTag table with normalization and fuzzy matching
func (h *DatasetHandler) CheckURL(c *gin.Context) {
	var req struct {
		URL       string   `json:"url"`
		URLs      []string `json:"urls"`
		Normalize *bool    `json:"normalize"`
		Fuzzy     *bool    `json:"fuzzy"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	normalize := true
	if req.Normalize != nil {
		normalize = *req.Normalize
	}
	fuzzy := false
	if req.Fuzzy != nil {
		fuzzy = *req.Fuzzy
	}

	// Batch mode
	if len(req.URLs) > 0 {
		results, err := h.svc.BatchCheckURLs(req.URLs, normalize, fuzzy)
		if err != nil {
			response.InternalError(c, err.Error())
			return
		}
		c.JSON(http.StatusOK, gin.H{"results": results})
		return
	}

	// Single URL mode
	if req.URL == "" {
		response.BadRequest(c, "url or urls required")
		return
	}

	result, err := h.svc.CheckURL(req.URL, normalize, fuzzy)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}
