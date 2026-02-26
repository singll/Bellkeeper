package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/response"
	"github.com/singll/bellkeeper/internal/service"
)

// --- Batch A: 高级标签端点 ---

// GetAll returns all tags without pagination (for dropdowns/workflows)
func (h *TagHandler) GetAll(c *gin.Context) {
	tags, err := h.svc.GetAll()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, tags)
}

// BatchGetOrCreate bulk get/create tags with auto_create support
func (h *TagHandler) BatchGetOrCreate(c *gin.Context) {
	var req struct {
		Names      []string `json:"names" binding:"required"`
		AutoCreate bool     `json:"auto_create"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	found, created, notFound, err := h.svc.BatchGetOrCreate(req.Names, req.AutoCreate)
	if err != nil {
		response.InternalError(c, err.Error())
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
		response.BadRequest(c, err.Error())
		return
	}

	tags, err := h.svc.MatchByKeywords(req.Keywords)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, tags)
}

// GetByNames returns tags matching the given name list
func (h *TagHandler) GetByNames(c *gin.Context) {
	var req struct {
		Names []string `json:"names" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	tags, err := h.svc.GetByNames(req.Names)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, tags)
}

type TagHandler struct {
	svc *service.TagService
}

func NewTagHandler(svc *service.TagService) *TagHandler {
	return &TagHandler{svc: svc}
}

func (h *TagHandler) List(c *gin.Context) {
	page, perPage := response.ParsePagination(c)
	keyword := c.Query("keyword")

	tags, total, err := h.svc.List(page, perPage, keyword)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Page(c, tags, total, page, perPage)
}

func (h *TagHandler) Get(c *gin.Context) {
	id, ok := response.ParseID(c, "id")
	if !ok {
		return
	}

	tag, err := h.svc.GetByID(id)
	if err != nil {
		response.NotFound(c, "tag not found")
		return
	}

	response.Success(c, tag)
}

func (h *TagHandler) Create(c *gin.Context) {
	var tag model.Tag
	if err := c.ShouldBindJSON(&tag); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if err := h.svc.Create(&tag); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Created(c, tag)
}

func (h *TagHandler) Update(c *gin.Context) {
	id, ok := response.ParseID(c, "id")
	if !ok {
		return
	}

	tag, err := h.svc.GetByID(id)
	if err != nil {
		response.NotFound(c, "tag not found")
		return
	}

	if err := c.ShouldBindJSON(tag); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if err := h.svc.Update(tag); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, tag)
}

func (h *TagHandler) Delete(c *gin.Context) {
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
