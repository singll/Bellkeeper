package handler

import (
	"errors"
	"os"
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

// FeedTimeline GET /api/pkb/feed/timeline?days=14&before=YYYY-MM-DD —— 列最近 N 天有资讯库存档的
// 日子（before 供「往前翻全部历史」分页）。供总览资讯时间线只读浏览（ADR-0006 例外）。
func (h *PKBReportHandler) FeedTimeline(c *gin.Context) {
	days := parsePKBReportLimit(c.DefaultQuery("days", "14"))
	before := c.Query("before")
	if before != "" {
		if _, ok := h.svc.ReportDate(before); !ok {
			response.BadRequest(c, "invalid before; expected YYYY-MM-DD")
			return
		}
	}
	result, err := h.svc.FeedTimeline(days, before)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}
	response.Success(c, result)
}

// FeedArchive GET /api/pkb/feed/archive?date=YYYY-MM-DD&domain=<dir> —— 读单篇资讯库每日存档并
// 只读渲染（服务端 goldmark+bluemonday 清洗）。ADR-0006 唯一例外，仅限资讯库存档。
func (h *PKBReportHandler) FeedArchive(c *gin.Context) {
	date := c.Query("date")
	domain := c.Query("domain")
	if date == "" || domain == "" {
		response.BadRequest(c, "date 与 domain 必填")
		return
	}
	result, err := h.svc.FeedArchiveHTML(date, domain)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			response.NotFound(c, "资讯库存档不存在")
			return
		}
		response.BadRequest(c, err.Error())
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
