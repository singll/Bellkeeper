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
		}
	}
	stats, err := h.svc.Stats(since)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, stats)
}
