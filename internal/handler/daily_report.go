package handler

import (
	"context"
	"errors"
	"io"
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
	data, err := h.svc.Collect(c.Request.Context(), date)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, data)
}

func (h *DailyReportHandler) Generate(c *gin.Context) {
	var opts service.GenerateOptions
	if err := c.ShouldBindJSON(&opts); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequest(c, "invalid request body: "+err.Error())
		return
	}

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
	data, err := h.svc.Collect(c.Request.Context(), date)
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
	var opts service.BriefGenerateOptions
	if err := c.ShouldBindJSON(&opts); err != nil && !errors.Is(err, io.EOF) {
		response.BadRequest(c, "invalid request body: "+err.Error())
		return
	}

	// 早报含多组 LLM 重要性打分（并行）+ 今日总结，给足预算避免慢日 LLM 拖超时。
	ctx, cancel := context.WithTimeout(c.Request.Context(), 300*time.Second)
	defer cancel()

	result, err := h.svc.GenerateBrief(ctx, opts)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, result)
}
