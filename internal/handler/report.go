package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/pkg/response"
	"github.com/singll/bellkeeper/internal/service"
)

// ReportHandler handles report file write requests.
type ReportHandler struct {
	svc *service.ReportService
}

// NewReportHandler creates a new report handler.
func NewReportHandler(svc *service.ReportService) *ReportHandler {
	return &ReportHandler{svc: svc}
}

// Write handles POST /api/reports/write
func (h *ReportHandler) Write(c *gin.Context) {
	var req service.WriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request: "+err.Error())
		return
	}

	result, err := h.svc.WriteMessage(&req)
	if err != nil {
		response.InternalError(c, "failed to write report: "+err.Error())
		return
	}

	response.Success(c, result)
}
