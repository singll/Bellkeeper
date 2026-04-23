package service

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
	"gorm.io/datatypes"
)

type LogCenterService struct {
	entryRepo     *repository.LogEntryRepository
	sourceRepo    *repository.LogSourceRepository
	alertRuleRepo *repository.LogAlertRuleRepository
}

func NewLogCenterService(
	entryRepo *repository.LogEntryRepository,
	sourceRepo *repository.LogSourceRepository,
	alertRuleRepo *repository.LogAlertRuleRepository,
) *LogCenterService {
	return &LogCenterService{
		entryRepo:     entryRepo,
		sourceRepo:    sourceRepo,
		alertRuleRepo: alertRuleRepo,
	}
}

type LogEntryParams struct {
	SourceID   uint
	Module     string
	Action     string
	Level      string
	Status     string
	Summary    string
	Detail     any
	RefID      string
	DurationMs int
	TraceID    string
}

func (s *LogCenterService) LogActivity(p LogEntryParams) {
	go func() {
		var detailJSON datatypes.JSON
		if p.Detail != nil {
			if b, err := json.Marshal(p.Detail); err == nil {
				detailJSON = datatypes.JSON(b)
			}
		}
		entry := &model.LogEntry{
			SourceID:   p.SourceID,
			Module:     p.Module,
			Action:     p.Action,
			Level:      p.Level,
			Status:     p.Status,
			Summary:    p.Summary,
			Detail:     detailJSON,
			RefID:      p.RefID,
			DurationMs: p.DurationMs,
			TraceID:    p.TraceID,
			CreatedAt:  time.Now(),
		}
		if err := s.entryRepo.Create(entry); err != nil {
			log.Printf("[ERROR] LogCenter: failed to log entry: module=%s action=%s error=%v", p.Module, p.Action, err)
		}
	}()
}

type ListEntriesQuery struct {
	SourceID uint
	Module   string
	Level    string
	Status   string
	TraceID  string
	Keyword  string
	Since    time.Time
	Until    time.Time
	Page     int
	Limit    int
}

type EntriesPage struct {
	Items []model.LogEntry `json:"items"`
	Total int64            `json:"total"`
	Page  int              `json:"page"`
	Limit int              `json:"limit"`
}

func (s *LogCenterService) ListEntries(q ListEntriesQuery) (*EntriesPage, error) {
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.Limit <= 0 {
		q.Limit = 50
	}
	entries, total, err := s.entryRepo.List(repository.LogEntryQuery{
		SourceID: q.SourceID,
		Module:   q.Module,
		Level:    q.Level,
		Status:   q.Status,
		TraceID:  q.TraceID,
		Keyword:  q.Keyword,
		Since:    q.Since,
		Until:    q.Until,
		Page:     q.Page,
		Limit:    q.Limit,
	})
	if err != nil {
		return nil, err
	}
	return &EntriesPage{Items: entries, Total: total, Page: q.Page, Limit: q.Limit}, nil
}

func (s *LogCenterService) GetEntry(id uint) (*model.LogEntry, error) {
	return s.entryRepo.GetByID(id)
}

func (s *LogCenterService) ListSources() ([]model.LogSource, error) {
	return s.sourceRepo.List()
}

func (s *LogCenterService) RegisterSource(name, sourceType, description string) (*model.LogSource, error) {
	apiKey, err := generateSourceAPIKey()
	if err != nil {
		return nil, err
	}
	source := &model.LogSource{
		Name:        name,
		SourceType:  sourceType,
		Description: description,
		APIKey:      apiKey,
		IsActive:    true,
	}
	if err := s.sourceRepo.Create(source); err != nil {
		return nil, err
	}
	return source, nil
}

func (s *LogCenterService) UpdateSource(id uint, name, sourceType, description *string, isActive *bool) (*model.LogSource, error) {
	source, err := s.sourceRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if name != nil {
		source.Name = *name
	}
	if sourceType != nil {
		source.SourceType = *sourceType
	}
	if description != nil {
		source.Description = *description
	}
	if isActive != nil {
		source.IsActive = *isActive
	}
	if err := s.sourceRepo.Update(source); err != nil {
		return nil, err
	}
	return source, nil
}

func (s *LogCenterService) DeleteSource(id uint) error {
	return s.sourceRepo.Delete(id)
}

func (s *LogCenterService) FindSourceByAPIKey(apiKey string) (*model.LogSource, error) {
	return s.sourceRepo.GetByAPIKey(apiKey)
}

func (s *LogCenterService) FindSourceByName(name string) (*model.LogSource, error) {
	return s.sourceRepo.GetByName(name)
}

type DashboardData struct {
	ByLevel   []repository.LevelCount     `json:"by_level"`
	ByModule  []repository.ModuleCount    `json:"by_module"`
	BySource  []repository.SourceCount    `json:"by_source"`
	ByHour    []repository.HourLevelCount `json:"by_hour"`
	TopErrors []repository.ModuleCount    `json:"top_errors"`
}

func (s *LogCenterService) GetDashboard(since time.Time) (*DashboardData, error) {
	byLevel, err := s.entryRepo.CountByLevel(since)
	if err != nil {
		return nil, err
	}
	byModule, err := s.entryRepo.CountByModule(since)
	if err != nil {
		return nil, err
	}
	bySource, err := s.entryRepo.CountBySource(since)
	if err != nil {
		return nil, err
	}
	byHour, err := s.entryRepo.CountByHourLevel(since)
	if err != nil {
		return nil, err
	}
	topErrors, err := s.entryRepo.CountErrorsByModule(since)
	if err != nil {
		return nil, err
	}
	return &DashboardData{
		ByLevel:   byLevel,
		ByModule:  byModule,
		BySource:  bySource,
		ByHour:    byHour,
		TopErrors: topErrors,
	}, nil
}

func (s *LogCenterService) GetDashboardByPeriod(period string) (*DashboardData, error) {
	var since time.Time
	switch period {
	case "24h":
		since = time.Now().Add(-24 * time.Hour)
	case "7d":
		since = time.Now().Add(-7 * 24 * time.Hour)
	case "30d":
		since = time.Now().Add(-30 * 24 * time.Hour)
	default:
		since = time.Now().Add(-24 * time.Hour)
	}
	return s.GetDashboard(since)
}

func (s *LogCenterService) ListAlertRules() ([]model.LogAlertRule, error) {
	return s.alertRuleRepo.List()
}

func (s *LogCenterService) CreateAlertRule(name string, condition json.RawMessage, notifyChannel string) (*model.LogAlertRule, error) {
	rule := &model.LogAlertRule{
		Name:          name,
		Condition:     datatypes.JSON(condition),
		NotifyChannel: notifyChannel,
		IsActive:      true,
	}
	if err := s.alertRuleRepo.Create(rule); err != nil {
		return nil, err
	}
	return rule, nil
}

func (s *LogCenterService) UpdateAlertRule(id uint, name *string, condition json.RawMessage, notifyChannel *string, isActive *bool) (*model.LogAlertRule, error) {
	rule, err := s.alertRuleRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if name != nil {
		rule.Name = *name
	}
	if condition != nil {
		rule.Condition = datatypes.JSON(condition)
	}
	if notifyChannel != nil {
		rule.NotifyChannel = *notifyChannel
	}
	if isActive != nil {
		rule.IsActive = *isActive
	}
	if err := s.alertRuleRepo.Update(rule); err != nil {
		return nil, err
	}
	return rule, nil
}

func (s *LogCenterService) DeleteAlertRule(id uint) error {
	return s.alertRuleRepo.Delete(id)
}

func (s *LogCenterService) CleanOldEntries(olderThanDays int) (int64, error) {
	return s.entryRepo.CleanOldEntries(olderThanDays)
}

type AlertCheckResult struct {
	RuleName  string `json:"rule_name"`
	Triggered bool   `json:"triggered"`
	Count     int64  `json:"count"`
}

func (s *LogCenterService) CheckAlerts() ([]AlertCheckResult, error) {
	rules, err := s.alertRuleRepo.ListActive()
	if err != nil {
		return nil, err
	}

	var results []AlertCheckResult
	for _, rule := range rules {
		var cond AlertCondition
		if err := json.Unmarshal(rule.Condition, &cond); err != nil {
			continue
		}

		since := time.Now().Add(-time.Duration(cond.WindowMinutes) * time.Minute)
		query := repository.LogEntryQuery{
			Module: cond.Module,
			Level:  cond.Level,
			Since:  since,
			Page:   1,
			Limit:  1,
		}
		_, total, err := s.entryRepo.List(query)
		if err != nil {
			continue
		}

		results = append(results, AlertCheckResult{
			RuleName:  rule.Name,
			Triggered: total >= int64(cond.Threshold),
			Count:     total,
		})
	}
	return results, nil
}

type AlertCondition struct {
	Module        string `json:"module"`
	Level         string `json:"level"`
	Threshold     int    `json:"threshold"`
	WindowMinutes int    `json:"window_minutes"`
}

func generateSourceAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate API key: %w", err)
	}
	return "ls_" + hex.EncodeToString(b), nil
}