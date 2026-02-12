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
	IsDefault   bool   `json:"is_default"`
	IsActive    bool   `json:"is_active"`
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

	mapping := &model.DatasetMapping{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		DatasetID:   req.DatasetID,
		Description: req.Description,
		IsDefault:   req.IsDefault,
		IsActive:    req.IsActive,
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
	mapping.IsDefault = req.IsDefault
	mapping.IsActive = req.IsActive
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
