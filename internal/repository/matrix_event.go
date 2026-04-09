package repository

import (
	"context"
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
)

type MatrixEventRepository struct {
	db *gorm.DB
}

func NewMatrixEventRepository(db *gorm.DB) *MatrixEventRepository {
	return &MatrixEventRepository{db: db}
}

func (r *MatrixEventRepository) Create(event *model.MatrixEvent) error {
	return r.db.Create(event).Error
}

func (r *MatrixEventRepository) ExistsByEventID(eventID string) (bool, error) {
	var count int64
	if err := r.db.Model(&model.MatrixEvent{}).Where("event_id = ?", eventID).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *MatrixEventRepository) UpdateStatus(eventID, status, errMsg string) error {
	updates := map[string]interface{}{
		"processing_status": status,
		"processed_at":      time.Now(),
	}
	if errMsg != "" {
		updates["error_message"] = errMsg
	}
	return r.db.Model(&model.MatrixEvent{}).Where("event_id = ?", eventID).Updates(updates).Error
}

type MatrixEventQuery struct {
	RoomID string
	Status string
	Since  time.Time
	Page   int
	Limit  int
}

func (r *MatrixEventRepository) List(q MatrixEventQuery) ([]model.MatrixEvent, int64, error) {
	var events []model.MatrixEvent
	var total int64

	tx := r.db.Model(&model.MatrixEvent{})
	if q.RoomID != "" {
		tx = tx.Where("room_id = ?", q.RoomID)
	}
	if q.Status != "" {
		tx = tx.Where("processing_status = ?", q.Status)
	}
	if !q.Since.IsZero() {
		tx = tx.Where("created_at > ?", q.Since)
	}

	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	limit := q.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := 0
	if q.Page > 1 {
		offset = (q.Page - 1) * limit
	}

	if err := tx.Order("created_at DESC").Offset(offset).Limit(limit).Find(&events).Error; err != nil {
		return nil, 0, err
	}
	return events, total, nil
}

func (r *MatrixEventRepository) GetRecent(ctx context.Context, limit int) ([]*model.MatrixEvent, error) {
	var events []*model.MatrixEvent
	err := r.db.WithContext(ctx).
		Where("created_at > ?", time.Now().Add(-24*time.Hour)).
		Order("created_at DESC").
		Limit(limit).
		Find(&events).Error
	return events, err
}
