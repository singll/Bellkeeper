package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/pkg/response"
	"github.com/singll/bellkeeper/internal/service"
)

// CrawlerHandler handles crawl management API endpoints
type CrawlerHandler struct {
	svc *service.CrawlService
}

// NewCrawlerHandler creates a new crawl handler
func NewCrawlerHandler(svc *service.CrawlService) *CrawlerHandler {
	return &CrawlerHandler{svc: svc}
}

// SourceHealth returns health dashboard for all crawl sources
// GET /api/crawl/sources/health
func (h *CrawlerHandler) SourceHealth(c *gin.Context) {
	statuses, err := h.svc.GetSourceHealthStatus()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, statuses)
}

// ResumeSource manually resumes a paused source
// POST /api/crawl/sources/:id/resume
func (h *CrawlerHandler) ResumeSource(c *gin.Context) {
	id, ok := response.ParseID(c, "id")
	if !ok {
		return
	}

	if err := h.svc.ResumeSource(id); err != nil {
		response.ErrorFromService(c, err)
		return
	}

	response.Message(c, "source resumed successfully")
}

// PauseSource manually pauses an active source
// POST /api/crawl/sources/:id/pause
func (h *CrawlerHandler) PauseSource(c *gin.Context) {
	id, ok := response.ParseID(c, "id")
	if !ok {
		return
	}

	if err := h.svc.PauseSource(id); err != nil {
		response.ErrorFromService(c, err)
		return
	}

	response.Message(c, "source paused successfully")
}

// CrawlJobs returns recent crawl job records (derived from activity logs)
// GET /api/crawl/jobs
func (h *CrawlerHandler) CrawlJobs(c *gin.Context) {
	page, perPage := response.ParsePagination(c)

	result, err := h.svc.GetRecentCrawlJobs(page, perPage)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}

// FetchSource triggers a single source fetch
// POST /api/crawl/fetch/:sourceId
func (h *CrawlerHandler) FetchSource(c *gin.Context) {
	id, ok := response.ParseID(c, "sourceId")
	if !ok {
		return
	}

	result, err := h.svc.FetchSource(c.Request.Context(), id)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, result)
}