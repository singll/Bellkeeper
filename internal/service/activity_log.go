package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
	"go.uber.org/zap"
)

type ActivityLogService struct {
	repo       *repository.ActivityLogRepository
	logCenter  *LogCenterService
}

func NewActivityLogService(repo *repository.ActivityLogRepository) *ActivityLogService {
	return &ActivityLogService{repo: repo}
}

func (s *ActivityLogService) SetLogCenter(lc *LogCenterService) {
	s.logCenter = lc
}

type LogActivityParams struct {
	Module     string
	Action     string
	Status     string
	Summary    string
	Detail     any
	RefID      string
	DurationMs int
}

// LogActivityCtx 是 LogActivity 的 context-aware 版本（1.0 §4.4）：
// 从 ctx 取 trace_id 贯穿到 log_center.LogEntryParams.TraceID。
// 新代码/HTTP 入口应优先用此方法。
func (s *ActivityLogService) LogActivityCtx(ctx context.Context, p LogActivityParams) {
	if s.logCenter != nil {
		level := "info"
		if p.Status == "failed" {
			level = "error"
		}
		s.logCenter.LogActivity(LogEntryParams{
			SourceID:   1, // bellkeeper-core
			Module:     p.Module,
			Action:     p.Action,
			Level:      level,
			Status:     p.Status,
			Summary:    p.Summary,
			Detail:     p.Detail,
			RefID:      p.RefID,
			DurationMs: p.DurationMs,
			TraceID:    middleware.TraceIDFromContext(ctx),
		})
	}

	// 始终写入 activity_logs 表（带 trace_id）
	SafeGo("activity_log.writeActivityLogCtx", func() {
		s.writeActivityLog(p, middleware.TraceIDFromContext(ctx))
	})
}

func (s *ActivityLogService) LogActivity(p LogActivityParams) {
	// 同时写入 LogCenter 和 activity_logs 表，保证 /api/logs 端点可查
	if s.logCenter != nil {
		level := "info"
		if p.Status == "failed" {
			level = "error"
		}
		s.logCenter.LogActivity(LogEntryParams{
			SourceID:   1, // bellkeeper-core
			Module:     p.Module,
			Action:     p.Action,
			Level:      level,
			Status:     p.Status,
			Summary:    p.Summary,
			Detail:     p.Detail,
			RefID:      p.RefID,
			DurationMs: p.DurationMs,
		})
	}

	// 始终写入 activity_logs 表（O02 日报等通过 /api/logs 查询此表）
	SafeGo("activity_log.writeActivityLog", func() {
		s.writeActivityLog(p, "")
	})
}

// writeActivityLog 写入 activity_logs 表（提取公共逻辑，含 trace_id）。
func (s *ActivityLogService) writeActivityLog(p LogActivityParams, traceID string) {
	detailStr := ""
	if p.Detail != nil {
		if b, err := json.Marshal(p.Detail); err == nil {
			detailStr = string(b)
		}
	}
	entry := &model.ActivityLog{
		Module:     p.Module,
		Action:     p.Action,
		Status:     p.Status,
		Summary:    p.Summary,
		Detail:     detailStr,
		RefID:      p.RefID,
		TraceID:    traceID,
		DurationMs: p.DurationMs,
		CreatedAt:  time.Now(),
	}
	if err := s.repo.Create(entry); err != nil {
		middleware.GetLogger().Error("failed to log activity",
			zap.String("module", p.Module),
			zap.String("action", p.Action),
			zap.Error(err))
	}
}

type ListActivityLogsQuery struct {
	Module string
	Status string
	RefID  string
	Since  time.Time
	Page   int
	Limit  int
}

type ActivityLogsPage struct {
	Items []model.ActivityLog `json:"items"`
	Total int64              `json:"total"`
	Page  int                `json:"page"`
	Limit int                `json:"limit"`
}

func (s *ActivityLogService) List(q ListActivityLogsQuery) (*ActivityLogsPage, error) {
	items, total, err := s.repo.List(repository.ActivityLogQuery{
		Module: q.Module,
		Status: q.Status,
		RefID:  q.RefID,
		Since:  q.Since,
		Page:   q.Page,
		Limit:  q.Limit,
	})
	if err != nil {
		return nil, err
	}
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.Limit <= 0 {
		q.Limit = 50
	}
	return &ActivityLogsPage{Items: items, Total: total, Page: q.Page, Limit: q.Limit}, nil
}

func (s *ActivityLogService) Modules() ([]string, error) {
	return s.repo.GetDistinctModules()
}

func (s *ActivityLogService) Stats(since time.Time) ([]repository.ModuleStat, error) {
	return s.repo.GetModuleStats(since)
}
