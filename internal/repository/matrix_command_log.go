package repository

import (
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
)

type MatrixCommandLogRepository struct {
	db *gorm.DB
}

func NewMatrixCommandLogRepository(db *gorm.DB) *MatrixCommandLogRepository {
	return &MatrixCommandLogRepository{db: db}
}

func (r *MatrixCommandLogRepository) Create(log *model.MatrixCommandLog) error {
	return r.db.Create(log).Error
}

func (r *MatrixCommandLogRepository) Complete(eventID, status, errMsg, responseEventID string, durationMs int) error {
	updates := map[string]interface{}{
		"execution_status":  status,
		"execution_time_ms": durationMs,
		"completed_at":      time.Now(),
	}
	if errMsg != "" {
		updates["error_message"] = errMsg
	}
	if responseEventID != "" {
		updates["response_event_id"] = responseEventID
	}
	return r.db.Model(&model.MatrixCommandLog{}).Where("event_id = ?", eventID).Updates(updates).Error
}

// List retrieves command logs with optional filtering and pagination
func (r *MatrixCommandLogRepository) List(page, pageSize int, command, status string) ([]model.MatrixCommandLog, int64, error) {
	var logs []model.MatrixCommandLog
	var total int64

	query := r.db.Model(&model.MatrixCommandLog{})

	// Apply filters
	if command != "" {
		query = query.Where("command = ?", command)
	}
	if status != "" {
		query = query.Where("execution_status = ?", status)
	}

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	// Query with pagination
	err := query.
		Order("created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&logs).Error

	return logs, total, err
}

// CountByStatus returns counts grouped by execution status since the given time
func (r *MatrixCommandLogRepository) CountByStatus(since time.Time) (map[string]int64, error) {
	type StatusCount struct {
		Status string
		Count  int64
	}

	var results []StatusCount
	err := r.db.Model(&model.MatrixCommandLog{}).
		Select("execution_status as status, count(*) as count").
		Where("created_at >= ?", since).
		Group("execution_status").
		Find(&results).Error

	if err != nil {
		return nil, err
	}

	counts := make(map[string]int64)
	for _, r := range results {
		counts[r.Status] = r.Count
	}
	return counts, nil
}
