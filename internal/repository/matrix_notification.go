package repository

import (
	"context"
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

func (r *MatrixNotificationRepository) Create(ctx context.Context, n *model.MatrixNotification) error {
	return r.db.WithContext(ctx).Create(n).Error
}

func (r *MatrixNotificationRepository) GetByNotificationID(ctx context.Context, id string) (*model.MatrixNotification, error) {
	var n model.MatrixNotification
	if err := r.db.WithContext(ctx).Where("notification_id = ?", id).First(&n).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &n, nil
}

func (r *MatrixNotificationRepository) UpdateStatus(ctx context.Context, id, status, lastError string) error {
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}
	if lastError != "" {
		updates["last_error"] = lastError
	}
	if status == "retrying" {
		updates["retry_count"] = gorm.Expr("retry_count + 1")
	}
	return r.db.WithContext(ctx).Model(&model.MatrixNotification{}).Where("notification_id = ?", id).Updates(updates).Error
}

func (r *MatrixNotificationRepository) MarkSent(ctx context.Context, id, eventID string) error {
	return r.db.WithContext(ctx).Model(&model.MatrixNotification{}).Where("notification_id = ?", id).Updates(map[string]interface{}{
		"status":         "sent",
		"sent_event_id":  eventID,
		"sent_at":        time.Now(),
		"updated_at":     time.Now(),
	}).Error
}

func (r *MatrixNotificationRepository) GetFailed(ctx context.Context, maxRetries int) ([]*model.MatrixNotification, error) {
	var notifications []*model.MatrixNotification
	err := r.db.WithContext(ctx).
		Where("status = ? AND retry_count < ?", "failed", maxRetries).
		Order("created_at ASC").
		Find(&notifications).Error
	return notifications, err
}

func (r *MatrixNotificationRepository) GetByChannel(ctx context.Context, channel string, limit int) ([]*model.MatrixNotification, error) {
	var notifications []*model.MatrixNotification
	err := r.db.WithContext(ctx).
		Where("channel_name = ?", channel).
		Order("created_at DESC").
		Limit(limit).
		Find(&notifications).Error
	return notifications, err
}

func (r *MatrixNotificationRepository) GetRecent(ctx context.Context, limit int) ([]*model.MatrixNotification, error) {
	var notifications []*model.MatrixNotification
	err := r.db.WithContext(ctx).
		Where("created_at > ?", time.Now().Add(-24*time.Hour)).
		Order("created_at DESC").
		Limit(limit).
		Find(&notifications).Error
	return notifications, err
}
