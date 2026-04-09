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
