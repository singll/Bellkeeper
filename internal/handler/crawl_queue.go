package handler

import (
	"strconv"
	"strings"
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/pkg/response"
	"github.com/singll/bellkeeper/internal/repository"
	"github.com/singll/bellkeeper/internal/service"

	"github.com/gin-gonic/gin"
)

// CrawlQueueHandler handles crawl queue HTTP requests
type CrawlQueueHandler struct {
	svc *service.CrawlQueueService
}

// NewCrawlQueueHandler creates a new CrawlQueueHandler
func NewCrawlQueueHandler(svc *service.CrawlQueueService) *CrawlQueueHandler {
	return &CrawlQueueHandler{svc: svc}
}

// Stats handles GET /api/crawl/queue/stats
func (h *CrawlQueueHandler) Stats(c *gin.Context) {
	stats, err := h.svc.Stats()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, stats)
}

// Audit handles GET /api/crawl/queue/audit
func (h *CrawlQueueHandler) Audit(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	since := time.Now().Add(-24 * time.Hour)
	if rawSince := c.Query("since"); rawSince != "" {
		if parsed, err := time.Parse(time.RFC3339, rawSince); err == nil {
			since = parsed
		} else if parsed, err := time.Parse("2006-01-02", rawSince); err == nil {
			since = parsed
		} else {
			response.BadRequest(c, "invalid since")
			return
		}
	} else if rawWindow := c.DefaultQuery("window", "24h"); rawWindow != "" {
		window, err := parseAuditWindow(rawWindow)
		if err != nil {
			response.BadRequest(c, "invalid window")
			return
		}
		since = time.Now().Add(-window)
	}

	stats, err := h.svc.Audit(since, limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, stats)
}

// Domains handles GET /api/crawl/queue/domains
func (h *CrawlQueueHandler) Domains(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}

	profiles, total, err := h.svc.ListDomainProfiles(page, limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Page(c, profiles, total, page, limit)
}

func parseAuditWindow(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if strings.HasSuffix(raw, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(raw, "d"))
		if err != nil || days <= 0 {
			_, parseErr := time.ParseDuration(raw)
			return 0, parseErr
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(raw)
}

// ListJobs handles GET /api/crawl/queue/jobs
func (h *CrawlQueueHandler) ListJobs(c *gin.Context) {
	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "50")
	status := c.DefaultQuery("status", "")
	domain := c.DefaultQuery("domain", "")
	channelType := c.DefaultQuery("channel_type", "")
	sinceStr := c.DefaultQuery("since", "")

	page, _ := strconv.Atoi(pageStr)
	limit, _ := strconv.Atoi(limitStr)
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}

	var sinceTime time.Time
	if sinceStr != "" {
		if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
			sinceTime = t
		} else if t, err := time.Parse("2006-01-02", sinceStr); err == nil {
			sinceTime = t
		}
	}

	opts := repository.ListCrawlJobOpts{
		Status:      model.CrawlJobStatus(status),
		Domain:      domain,
		ChannelType: channelType,
		Since:       sinceTime,
		Page:        page,
		Limit:       limit,
	}

	jobs, total, err := h.svc.ListJobs(opts)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Page(c, jobs, total, page, limit)
}

// RetryJob handles POST /api/crawl/queue/jobs/:id/retry
func (h *CrawlQueueHandler) RetryJob(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid ID")
		return
	}

	if err := h.svc.RetryJob(uint(id)); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, gin.H{"id": id, "status": "pending"})
}

// Workers handles GET /api/crawl/queue/workers
func (h *CrawlQueueHandler) Workers(c *gin.Context) {
	statuses := h.svc.WorkerStatuses()
	response.Success(c, statuses)
}

// Blocked handles GET /api/crawl/queue/blocked
func (h *CrawlQueueHandler) Blocked(c *gin.Context) {
	jobs, err := h.svc.GetBlockedJobs()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, jobs)
}

// Unblock handles POST /api/crawl/queue/blocked/:id/unblock
func (h *CrawlQueueHandler) Unblock(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		response.BadRequest(c, "invalid ID")
		return
	}

	if err := h.svc.UnblockJob(uint(id)); err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, gin.H{"id": id, "status": "pending"})
}

// Enqueue handles POST /api/crawl/queue/enqueue
func (h *CrawlQueueHandler) Enqueue(c *gin.Context) {
	var req struct {
		URL         string                 `json:"url" binding:"required"`
		Title       string                 `json:"title"`
		SourceID    uint                   `json:"source_id"`
		ChannelType string                 `json:"channel_type"`
		Metadata    map[string]interface{} `json:"metadata"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	jobID, err := h.svc.Enqueue(req.SourceID, req.URL, req.Title, req.ChannelType, req.Metadata)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Success(c, gin.H{"queued": true, "job_id": jobID})
}

// Cleanup handles POST /api/crawl/queue/cleanup
func (h *CrawlQueueHandler) Cleanup(c *gin.Context) {
	var req struct {
		OlderThanDays int    `json:"older_than_days"`
		Domain        string `json:"domain"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.OlderThanDays <= 0 {
		req.OlderThanDays = 3
	}

	skipped, err := h.svc.CleanupStalePending(req.OlderThanDays, req.Domain)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, gin.H{"skipped": skipped})
}
