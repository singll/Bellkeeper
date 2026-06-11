package handler

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/pkg/response"
	"github.com/singll/bellkeeper/internal/service"
)

type DailyReportHandler struct {
	svc *service.DailyReportService
}

func NewDailyReportHandler(svc *service.DailyReportService) *DailyReportHandler {
	return &DailyReportHandler{svc: svc}
}

func (h *DailyReportHandler) DailyData(c *gin.Context) {
	date := c.Query("date")
	data, err := h.svc.Collect(date)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, data)
}

func (h *DailyReportHandler) Generate(c *gin.Context) {
	var opts service.GenerateOptions
	_ = c.ShouldBindJSON(&opts)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 180*time.Second)
	defer cancel()

	result, err := h.svc.Generate(ctx, opts)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, result)
}

func (h *DailyReportHandler) BriefData(c *gin.Context) {
	date := c.Query("date")
	data, err := h.svc.Collect(date)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	brief := &service.BriefReportData{
		Date:      data.Date,
		Crawl:     data.Crawl,
		RSSIngest: data.RSSIngest,
		PKB:       data.PKB,
		PKBCards:  data.PKBCards,
	}
	response.Success(c, brief)
}

func (h *DailyReportHandler) GenerateBrief(c *gin.Context) {
	date := c.Query("date")

	data, err := h.svc.Collect(date)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if h.svc.LLMClient() != nil {
		aiSummary, aiErr := h.svc.GenerateAISummary(c.Request.Context(), data)
		if aiErr != nil {
			data.AISummary = "(AI总结暂不可用)"
		} else {
			data.AISummary = aiSummary
		}
	}

	markdown := service.RenderBriefReport(data)

	result := map[string]interface{}{
		"date":     data.Date,
		"markdown": markdown,
		"data":     data,
	}
	response.Success(c, result)
}
