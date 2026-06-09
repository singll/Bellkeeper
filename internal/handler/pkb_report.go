package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/pkg/response"
	"github.com/singll/bellkeeper/internal/service"
)

type PKBReportHandler struct {
	svc *service.PKBReportService
}

func NewPKBReportHandler(svc *service.PKBReportService) *PKBReportHandler {
	return &PKBReportHandler{svc: svc}
}

func (h *PKBReportHandler) Daily(c *gin.Context) {
	date, ok := h.svc.ReportDate(c.Query("date"))
	if !ok {
		response.BadRequest(c, "invalid date; expected YYYY-MM-DD")
		return
	}
	limit := parsePKBReportLimit(c.DefaultQuery("limit", "20"))
	result, err := h.svc.Daily(date, limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *PKBReportHandler) VaultCards(c *gin.Context) {
	date := c.Query("date")
	if date != "" {
		if _, ok := h.svc.ReportDate(date); !ok {
			response.BadRequest(c, "invalid date; expected YYYY-MM-DD")
			return
		}
	}
	limit := parsePKBReportLimit(c.DefaultQuery("limit", "20"))
	result, err := h.svc.VaultCardsByDate(date, limit)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *PKBReportHandler) LatestDigests(c *gin.Context) {
	result, err := h.svc.LatestDigests()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, result)
}

func parsePKBReportLimit(raw string) int {
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}
