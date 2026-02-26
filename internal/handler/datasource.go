package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/response"
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
	page, perPage := response.ParsePagination(c)
	category := c.Query("category")
	keyword := c.Query("keyword")

	sources, total, err := h.svc.List(page, perPage, category, keyword)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Page(c, sources, total, page, perPage)
}

func (h *DataSourceHandler) Get(c *gin.Context) {
	id, ok := response.ParseID(c, "id")
	if !ok {
		return
	}

	source, err := h.svc.GetByID(id)
	if err != nil {
		response.NotFound(c, "data source not found")
		return
	}

	response.Success(c, source)
}

func (h *DataSourceHandler) Create(c *gin.Context) {
	var req DataSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
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
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, source)
}

func (h *DataSourceHandler) Update(c *gin.Context) {
	id, ok := response.ParseID(c, "id")
	if !ok {
		return
	}

	source, err := h.svc.GetByID(id)
	if err != nil {
		response.NotFound(c, "data source not found")
		return
	}

	var req DataSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
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
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, source)
}

func (h *DataSourceHandler) Delete(c *gin.Context) {
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
