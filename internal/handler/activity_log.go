package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/pkg/response"
	"github.com/singll/bellkeeper/internal/service"
)

type ActivityLogHandler struct {
	svc *service.ActivityLogService
}

func NewActivityLogHandler(svc *service.ActivityLogService) *ActivityLogHandler {
	return &ActivityLogHandler{svc: svc}
}

func (h *ActivityLogHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	var since time.Time
	if s := c.Query("since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			since = t
		} else if t, err := time.ParseInLocation("2006-01-02T15:04:05", s, time.Local); err == nil {
			since = t
		} else if t, err := time.Parse("2006-01-02", s); err == nil {
			since = t
		}
	}

	result, err := h.svc.List(service.ListActivityLogsQuery{
		Module: c.Query("module"),
		Status: c.Query("status"),
		RefID:  c.Query("ref_id"),
		Since:  since,
		Page:   page,
		Limit:  limit,
	})
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *ActivityLogHandler) Modules(c *gin.Context) {
	modules, err := h.svc.Modules()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, modules)
}

func (h *ActivityLogHandler) Stats(c *gin.Context) {
	var since time.Time
	if s := c.Query("since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			since = t
		} else if t, err := time.ParseInLocation("2006-01-02T15:04:05", s, time.Local); err == nil {
			since = t
		} else if t, err := time.Parse("2006-01-02", s); err == nil {
			since = t
		}
	}
	stats, err := h.svc.Stats(since)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, stats)
}

// CreateRequest represents a request to create an activity log entry.
type CreateRequest struct {
	Module     string `json:"module" binding:"required"`
	Action     string `json:"action" binding:"required"`
	Status     string `json:"status" binding:"required"`
	Summary    string `json:"summary"`
	Detail     any    `json:"detail"`
	RefID      string `json:"ref_id"`
	DurationMs int    `json:"duration_ms"`
}

// Create handles POST /api/logs
func (h *ActivityLogHandler) Create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	h.svc.LogActivity(service.LogActivityParams{
		Module:     req.Module,
		Action:     req.Action,
		Status:     req.Status,
		Summary:    req.Summary,
		Detail:     req.Detail,
		RefID:      req.RefID,
		DurationMs: req.DurationMs,
	})

	response.Success(c, gin.H{"message": "activity logged"})
}
