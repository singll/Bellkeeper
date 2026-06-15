package handler

import (
	"strconv"

	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/response"
	"github.com/singll/bellkeeper/internal/repository"
	"github.com/singll/bellkeeper/internal/service"

	"github.com/gin-gonic/gin"
)

type CrawlFailureHandler struct {
	svc *service.CrawlFailureService
}

func NewCrawlFailureHandler(svc *service.CrawlFailureService) *CrawlFailureHandler {
	return &CrawlFailureHandler{svc: svc}
}

func (h *CrawlFailureHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	domain := c.Query("domain")
	statusStr := c.Query("status")

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}

	opts := repository.ListCrawlFailuresOpts{
		Domain: domain,
		Status: model.CrawlFailureStatus(statusStr),
		Page:   page,
		Limit:  limit,
	}

	failures, total, err := h.svc.List(opts)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Page(c, failures, total, page, limit)
}

func (h *CrawlFailureHandler) Get(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid ID")
		return
	}

	failure, err := h.svc.GetByID(uint(id))
	if err != nil {
		response.NotFound(c, "failure not found")
		return
	}
	response.Success(c, failure)
}

func (h *CrawlFailureHandler) Retry(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid ID")
		return
	}

	if err := h.svc.Retry(uint(id)); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"id": id, "status": "retrying"})
}

func (h *CrawlFailureHandler) Abandon(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid ID")
		return
	}

	if err := h.svc.Abandon(uint(id)); err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"id": id, "status": "abandoned"})
}
