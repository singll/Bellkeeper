package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/service"
)

type DataSourceHandler struct {
	svc *service.DataSourceService
}

func NewDataSourceHandler(svc *service.DataSourceService) *DataSourceHandler {
	return &DataSourceHandler{svc: svc}
}

type DataSourceRequest struct {
	Name        string `json:"name" binding:"required"`
	URL         string `json:"url" binding:"required"`
	Type        string `json:"type"`
	Category    string `json:"category"`
	Description string `json:"description"`
	IsActive    *bool  `json:"is_active"`
	TagIDs      []uint `json:"tag_ids"`
}

func (h *DataSourceHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	category := c.Query("category")
	keyword := c.Query("keyword")

	sources, total, err := h.svc.List(page, perPage, category, keyword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":     sources,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

func (h *DataSourceHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	source, err := h.svc.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "data source not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": source})
}

func (h *DataSourceHandler) Create(c *gin.Context) {
	var req DataSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	source := &model.DataSource{
		Name:        req.Name,
		URL:         req.URL,
		Type:        req.Type,
		Category:    req.Category,
		Description: req.Description,
		IsActive:    isActive,
	}

	if err := h.svc.Create(source, req.TagIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": source})
}

func (h *DataSourceHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	source, err := h.svc.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "data source not found"})
		return
	}

	var req DataSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	source.Name = req.Name
	source.URL = req.URL
	source.Type = req.Type
	source.Category = req.Category
	source.Description = req.Description
	if req.IsActive != nil {
		source.IsActive = *req.IsActive
	}

	if err := h.svc.Update(source, req.TagIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": source})
}

func (h *DataSourceHandler) Delete(c *gin.Context) {
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
