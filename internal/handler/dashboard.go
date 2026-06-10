package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/pkg/response"
	"github.com/singll/bellkeeper/internal/service"
)

// DashboardHandler serves aggregated home-dashboard statistics.
type DashboardHandler struct {
	svc *service.DashboardService
}

// NewDashboardHandler creates a dashboard handler.
func NewDashboardHandler(svc *service.DashboardService) *DashboardHandler {
	return &DashboardHandler{svc: svc}
}

// Stats handles GET /api/dashboard/stats
func (h *DashboardHandler) Stats(c *gin.Context) {
	stats, err := h.svc.Stats()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, stats)
}
