package service

import (
	"encoding/json"
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
)

type ActivityLogService struct {
	repo *repository.ActivityLogRepository
}

func NewActivityLogService(repo *repository.ActivityLogRepository) *ActivityLogService {
	return &ActivityLogService{repo: repo}
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

func (s *ActivityLogService) LogActivity(p LogActivityParams) {
	go func() {
		detailStr := ""
		if p.Detail != nil {
			if b, err := json.Marshal(p.Detail); err == nil {
				detailStr = string(b)
			}
		}
		_ = s.repo.Create(&model.ActivityLog{
			Module:     p.Module,
			Action:     p.Action,
			Status:     p.Status,
			Summary:    p.Summary,
			Detail:     detailStr,
			RefID:      p.RefID,
			DurationMs: p.DurationMs,
			CreatedAt:  time.Now(),
		})
	}()
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
