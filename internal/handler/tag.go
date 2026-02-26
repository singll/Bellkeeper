package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/service"
)

// --- Batch A: 高级标签端点 ---

// GetAll returns all tags without pagination (for dropdowns/workflows)
func (h *TagHandler) GetAll(c *gin.Context) {
	tags, err := h.svc.GetAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": tags})
}

// BatchGetOrCreate bulk get/create tags with auto_create support
func (h *TagHandler) BatchGetOrCreate(c *gin.Context) {
	var req struct {
		Names      []string `json:"names" binding:"required"`
		AutoCreate bool     `json:"auto_create"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	found, created, notFound, err := h.svc.BatchGetOrCreate(req.Names, req.AutoCreate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"found":     found,
		"created":   created,
		"not_found": notFound,
	})
}

// Match intelligently matches tags by keywords
func (h *TagHandler) Match(c *gin.Context) {
	var req struct {
		Keywords []string `json:"keywords" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tags, err := h.svc.MatchByKeywords(req.Keywords)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": tags})
}

// GetByNames returns tags matching the given name list
func (h *TagHandler) GetByNames(c *gin.Context) {
	var req struct {
		Names []string `json:"names" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tags, err := h.svc.GetByNames(req.Names)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": tags})
}

type TagHandler struct {
	svc *service.TagService
}

func NewTagHandler(svc *service.TagService) *TagHandler {
	return &TagHandler{svc: svc}
}

func (h *TagHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	keyword := c.Query("keyword")

	tags, total, err := h.svc.List(page, perPage, keyword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":     tags,
		"total":    total,
		"page":     page,
		"per_page": perPage,
	})
}

func (h *TagHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	tag, err := h.svc.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tag not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": tag})
}

func (h *TagHandler) Create(c *gin.Context) {
	var tag model.Tag
	if err := c.ShouldBindJSON(&tag); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.svc.Create(&tag); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": tag})
}

func (h *TagHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	tag, err := h.svc.GetByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tag not found"})
		return
	}

	if err := c.ShouldBindJSON(tag); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.svc.Update(tag); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": tag})
}

func (h *TagHandler) Delete(c *gin.Context) {
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
