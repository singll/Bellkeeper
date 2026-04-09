package repository

import (
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
)

type MatrixNotificationRepository struct {
	db *gorm.DB
}

func NewMatrixNotificationRepository(db *gorm.DB) *MatrixNotificationRepository {
	return &MatrixNotificationRepository{db: db}
}

func (r *MatrixNotificationRepository) Create(n *model.MatrixNotification) error {
	return r.db.Create(n).Error
}

func (r *MatrixNotificationRepository) GetByNotificationID(id string) (*model.MatrixNotification, error) {
	var n model.MatrixNotification
	if err := r.db.Where("notification_id = ?", id).First(&n).Error; err != nil {
		return nil, err
	}
	return &n, nil
}

func (r *MatrixNotificationRepository) UpdateStatus(id, status, lastError, sentEventID string) error {
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}
	if lastError != "" {
		updates["last_error"] = lastError
	}
	if sentEventID != "" {
		updates["sent_event_id"] = sentEventID
		updates["sent_at"] = time.Now()
	}
	if status == "retrying" {
		updates["retry_count"] = gorm.Expr("retry_count + 1")
	}
	return r.db.Model(&model.MatrixNotification{}).Where("notification_id = ?", id).Updates(updates).Error
}
