package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/defaults"
	"github.com/singll/bellkeeper/internal/pkg/response"
	"github.com/singll/bellkeeper/internal/service"
)

type RSSHandler struct {
	svc *service.RSSService
}

func NewRSSHandler(svc *service.RSSService) *RSSHandler {
	return &RSSHandler{svc: svc}
}

type RSSRequest struct {
	Name                 string `json:"name" binding:"required"`
	URL                  string `json:"url" binding:"required"`
	Category             string `json:"category"`
	Description          string `json:"description"`
	IsActive             *bool  `json:"is_active"`
	FetchIntervalMinutes int    `json:"fetch_interval_minutes"`
	TagIDs               []uint `json:"tag_ids"`
}

func (h *RSSHandler) List(c *gin.Context) {
	page, perPage := response.ParsePagination(c)
	category := c.Query("category")
	keyword := c.Query("keyword")

	feeds, total, err := h.svc.List(page, perPage, category, keyword)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Page(c, feeds, total, page, perPage)
}

func (h *RSSHandler) Get(c *gin.Context) {
	id, ok := response.ParseID(c, "id")
	if !ok {
		return
	}

	feed, err := h.svc.GetByID(id)
	if err != nil {
		response.NotFound(c, "RSS feed not found")
		return
	}

	response.Success(c, feed)
}

func (h *RSSHandler) Create(c *gin.Context) {
	var req RSSRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	feed := &model.RSSFeed{
		Name:                 req.Name,
		URL:                  req.URL,
		Category:             req.Category,
		Description:          req.Description,
		IsActive:             isActive,
		FetchIntervalMinutes: req.FetchIntervalMinutes,
	}

	if feed.FetchIntervalMinutes == 0 {
		feed.FetchIntervalMinutes = defaults.DefaultFetchInterval
	}

	if err := h.svc.Create(feed, req.TagIDs); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, feed)
}

func (h *RSSHandler) Update(c *gin.Context) {
	id, ok := response.ParseID(c, "id")
	if !ok {
		return
	}

	feed, err := h.svc.GetByID(id)
	if err != nil {
		response.NotFound(c, "RSS feed not found")
		return
	}

	var req RSSRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	feed.Name = req.Name
	feed.URL = req.URL
	feed.Category = req.Category
	feed.Description = req.Description
	if req.IsActive != nil {
		feed.IsActive = *req.IsActive
	}
	feed.FetchIntervalMinutes = req.FetchIntervalMinutes

	if err := h.svc.Update(feed, req.TagIDs); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, feed)
}

func (h *RSSHandler) Delete(c *gin.Context) {
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
