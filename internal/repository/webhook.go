package repository

import (
	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
)

type WebhookRepository struct {
	db *gorm.DB
}

func NewWebhookRepository(db *gorm.DB) *WebhookRepository {
	return &WebhookRepository{db: db}
}

func (r *WebhookRepository) List(page, perPage int) ([]model.WebhookConfig, int64, error) {
	var webhooks []model.WebhookConfig
	var total int64

	query := r.db.Model(&model.WebhookConfig{})

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * perPage
	if err := query.Offset(offset).Limit(perPage).Order("id DESC").Find(&webhooks).Error; err != nil {
		return nil, 0, err
	}

	return webhooks, total, nil
}

func (r *WebhookRepository) GetByID(id uint) (*model.WebhookConfig, error) {
	var webhook model.WebhookConfig
	if err := r.db.First(&webhook, id).Error; err != nil {
		return nil, err
	}
	return &webhook, nil
}

func (r *WebhookRepository) Create(webhook *model.WebhookConfig) error {
	return r.db.Create(webhook).Error
}

func (r *WebhookRepository) Update(webhook *model.WebhookConfig) error {
	return r.db.Save(webhook).Error
}

func (r *WebhookRepository) Delete(id uint) error {
	return r.db.Delete(&model.WebhookConfig{}, id).Error
}

func (r *WebhookRepository) CreateHistory(history *model.WebhookHistory) error {
	return r.db.Create(history).Error
}

func (r *WebhookRepository) UpdateHistory(history *model.WebhookHistory) error {
	return r.db.Save(history).Error
}

func (r *WebhookRepository) GetHistory(webhookID uint, limit int) ([]model.WebhookHistory, error) {
	var history []model.WebhookHistory
	if err := r.db.Where("webhook_id = ?", webhookID).Order("created_at DESC").Limit(limit).Find(&history).Error; err != nil {
		return nil, err
	}
	return history, nil
}
