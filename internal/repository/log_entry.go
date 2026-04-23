package repository

import (
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
)

type LogEntryRepository struct {
	db *gorm.DB
}

func NewLogEntryRepository(db *gorm.DB) *LogEntryRepository {
	return &LogEntryRepository{db: db}
}

func (r *LogEntryRepository) Create(entry *model.LogEntry) error {
	return r.db.Create(entry).Error
}

func (r *LogEntryRepository) GetByID(id uint) (*model.LogEntry, error) {
	var entry model.LogEntry
	if err := r.db.Preload("Source").First(&entry, id).Error; err != nil {
		return nil, err
	}
	return &entry, nil
}

type LogEntryQuery struct {
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

func (r *LogEntryRepository) List(q LogEntryQuery) ([]model.LogEntry, int64, error) {
	var entries []model.LogEntry
	var total int64

	tx := r.db.Model(&model.LogEntry{})
	if q.SourceID > 0 {
		tx = tx.Where("source_id = ?", q.SourceID)
	}
	if q.Module != "" {
		tx = tx.Where("module = ?", q.Module)
	}
	if q.Level != "" {
		tx = tx.Where("level = ?", q.Level)
	}
	if q.Status != "" {
		tx = tx.Where("status = ?", q.Status)
	}
	if q.TraceID != "" {
		tx = tx.Where("trace_id = ?", q.TraceID)
	}
	if q.Keyword != "" {
		tx = tx.Where("summary ILIKE ?", "%"+q.Keyword+"%")
	}
	if !q.Since.IsZero() {
		tx = tx.Where("created_at >= ?", q.Since)
	}
	if !q.Until.IsZero() {
		tx = tx.Where("created_at <= ?", q.Until)
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

	if err := tx.Preload("Source").Order("created_at DESC").Offset(offset).Limit(q.Limit).Find(&entries).Error; err != nil {
		return nil, 0, err
	}
	return entries, total, nil
}

type LevelCount struct {
	Level string `json:"level"`
	Count int64  `json:"count"`
}

type ModuleCount struct {
	Module string `json:"module"`
	Count  int64  `json:"count"`
}

type SourceCount struct {
	SourceID uint  `json:"source_id"`
	Count    int64 `json:"count"`
}

func (r *LogEntryRepository) CountByLevel(since time.Time) ([]LevelCount, error) {
	var counts []LevelCount
	tx := r.db.Model(&model.LogEntry{}).Select("level, COUNT(*) as count").Group("level")
	if !since.IsZero() {
		tx = tx.Where("created_at >= ?", since)
	}
	if err := tx.Scan(&counts).Error; err != nil {
		return nil, err
	}
	return counts, nil
}

func (r *LogEntryRepository) CountByModule(since time.Time) ([]ModuleCount, error) {
	var counts []ModuleCount
	tx := r.db.Model(&model.LogEntry{}).Select("module, COUNT(*) as count").Group("module")
	if !since.IsZero() {
		tx = tx.Where("created_at >= ?", since)
	}
	if err := tx.Scan(&counts).Error; err != nil {
		return nil, err
	}
	return counts, nil
}

func (r *LogEntryRepository) CountBySource(since time.Time) ([]SourceCount, error) {
	var counts []SourceCount
	tx := r.db.Model(&model.LogEntry{}).Select("source_id, COUNT(*) as count").Group("source_id")
	if !since.IsZero() {
		tx = tx.Where("created_at >= ?", since)
	}
	if err := tx.Scan(&counts).Error; err != nil {
		return nil, err
	}
	return counts, nil
}

func (r *LogEntryRepository) CountByHourLevel(since time.Time) ([]HourLevelCount, error) {
	var counts []HourLevelCount
	tx := r.db.Model(&model.LogEntry{}).
		Select("date_trunc('hour', created_at) as hour, level, COUNT(*) as count").
		Group("hour, level").Order("hour ASC")
	if !since.IsZero() {
		tx = tx.Where("created_at >= ?", since)
	}
	if err := tx.Scan(&counts).Error; err != nil {
		return nil, err
	}
	return counts, nil
}

type HourLevelCount struct {
	Hour  time.Time `json:"hour"`
	Level string    `json:"level"`
	Count int64     `json:"count"`
}

func (r *LogEntryRepository) CountErrorsByModule(since time.Time) ([]ModuleCount, error) {
	var counts []ModuleCount
	tx := r.db.Model(&model.LogEntry{}).
		Select("module, COUNT(*) as count").
		Where("level IN ?", []string{"error", "warn"}).
		Group("module")
	if !since.IsZero() {
		tx = tx.Where("created_at >= ?", since)
	}
	if err := tx.Scan(&counts).Error; err != nil {
		return nil, err
	}
	return counts, nil
}

func (r *LogEntryRepository) CleanOldEntries(olderThanDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -olderThanDays)
	result := r.db.Where("created_at < ?", cutoff).Delete(&model.LogEntry{})
	return result.RowsAffected, result.Error
}