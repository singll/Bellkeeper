package repository

import (
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
)

type ActivityLogRepository struct {
	db *gorm.DB
}

func NewActivityLogRepository(db *gorm.DB) *ActivityLogRepository {
	return &ActivityLogRepository{db: db}
}

func (r *ActivityLogRepository) Create(log *model.ActivityLog) error {
	return r.db.Create(log).Error
}

type ActivityLogQuery struct {
	Module string
	Status string
	RefID  string
	Since  time.Time
	Page   int
	Limit  int
}

func (r *ActivityLogRepository) List(q ActivityLogQuery) ([]model.ActivityLog, int64, error) {
	var logs []model.ActivityLog
	var total int64

	tx := r.db.Model(&model.ActivityLog{})
	if q.Module != "" {
		tx = tx.Where("module = ?", q.Module)
	}
	if q.Status != "" {
		tx = tx.Where("status = ?", q.Status)
	}
	if q.RefID != "" {
		tx = tx.Where("ref_id = ?", q.RefID)
	}
	if !q.Since.IsZero() {
		tx = tx.Where("created_at > ?", q.Since)
	}

	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := 0
	if q.Page > 1 {
		offset = (q.Page - 1) * q.Limit
	}
	if q.Limit <= 0 {
		q.Limit = 50
	}

	if err := tx.Order("created_at DESC").Offset(offset).Limit(q.Limit).Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

type ModuleStat struct {
	Module string `json:"module"`
	Count  int64  `json:"count"`
}

func (r *ActivityLogRepository) GetModuleStats(since time.Time) ([]ModuleStat, error) {
	var stats []ModuleStat
	tx := r.db.Model(&model.ActivityLog{}).
		Select("module, COUNT(*) as count").
		Group("module")
	if !since.IsZero() {
		tx = tx.Where("created_at > ?", since)
	}
	if err := tx.Scan(&stats).Error; err != nil {
		return nil, err
	}
	return stats, nil
}

func (r *ActivityLogRepository) GetDistinctModules() ([]string, error) {
	var modules []string
	if err := r.db.Model(&model.ActivityLog{}).Distinct("module").Pluck("module", &modules).Error; err != nil {
		return nil, err
	}
	return modules, nil
}

type ActionStat struct {
	Action string `json:"action"`
	Status string `json:"status"`
	Count  int64  `json:"count"`
}

func (r *ActivityLogRepository) GetActionStats(module string, since time.Time) ([]ActionStat, error) {
	var stats []ActionStat
	tx := r.db.Model(&model.ActivityLog{}).
		Select("action, status, COUNT(*) as count").
		Where("module = ? AND created_at >= ?", module, since).
		Group("action, status")
	if err := tx.Scan(&stats).Error; err != nil {
		return nil, err
	}
	return stats, nil
}

func (r *ActivityLogRepository) GetRecentFailures(module string, since time.Time, limit int) ([]model.ActivityLog, error) {
	var logs []model.ActivityLog
	if limit <= 0 {
		limit = 10
	}
	if err := r.db.Where("module = ? AND status = ? AND created_at >= ?", module, "failure", since).
		Order("created_at DESC").
		Limit(limit).
		Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

func (r *ActivityLogRepository) CleanOldLogs(olderThanDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -olderThanDays)
	result := r.db.Where("created_at < ?", cutoff).Delete(&model.ActivityLog{})
	return result.RowsAffected, result.Error
}
