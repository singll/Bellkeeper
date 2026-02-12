package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/model"
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
	IsActive             bool   `json:"is_active"`
	FetchIntervalMinutes int    `json:"fetch_interval_minutes"`
	TagIDs               []uint `json:"tag_ids"`
}

func (h *RSSHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	category := c.Query("category")
	keyword := c.Query("keyword")

	feeds, total, err := h.svc.List(page, perPage, category, keyword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":     feeds,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

func (h *RSSHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	feed, err := h.svc.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "RSS feed not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": feed})
}

func (h *RSSHandler) Create(c *gin.Context) {
	var req RSSRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	feed := &model.RSSFeed{
		Name:                 req.Name,
		URL:                  req.URL,
		Category:             req.Category,
		Description:          req.Description,
		IsActive:             req.IsActive,
		FetchIntervalMinutes: req.FetchIntervalMinutes,
	}

	if feed.FetchIntervalMinutes == 0 {
		feed.FetchIntervalMinutes = 60
	}

	if err := h.svc.Create(feed, req.TagIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": feed})
}

func (h *RSSHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	feed, err := h.svc.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "RSS feed not found"})
		return
	}

	var req RSSRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	feed.Name = req.Name
	feed.URL = req.URL
	feed.Category = req.Category
	feed.Description = req.Description
	feed.IsActive = req.IsActive
	feed.FetchIntervalMinutes = req.FetchIntervalMinutes

	if err := h.svc.Update(feed, req.TagIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": feed})
}

func (h *RSSHandler) Delete(c *gin.Context) {
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
