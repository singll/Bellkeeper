package repository

import (
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
)

// ConversationBindingRepository manages LLM conversation bindings in DB.
type ConversationBindingRepository struct {
	db *gorm.DB
}

// NewConversationBindingRepository creates a new repository instance.
func NewConversationBindingRepository(db *gorm.DB) *ConversationBindingRepository {
	return &ConversationBindingRepository{db: db}
}

// List returns all active (non-expired) bindings.
func (r *ConversationBindingRepository) List() ([]model.LLMConversationBinding, error) {
	var bindings []model.LLMConversationBinding
	err := r.db.Where("expires_at > ?", time.Now()).Order("last_seen_at DESC").Find(&bindings).Error
	return bindings, err
}

// GetByConversationID fetches a binding by its conversation ID.
func (r *ConversationBindingRepository) GetByConversationID(conversationID string) (*model.LLMConversationBinding, error) {
	var b model.LLMConversationBinding
	err := r.db.Where("conversation_id = ?", conversationID).First(&b).Error
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// Create inserts a new binding.
func (r *ConversationBindingRepository) Create(b *model.LLMConversationBinding) error {
	return r.db.Create(b).Error
}

// Update saves an existing binding.
func (r *ConversationBindingRepository) Update(b *model.LLMConversationBinding) error {
	return r.db.Save(b).Error
}

// Delete removes a binding by conversation ID.
func (r *ConversationBindingRepository) Delete(conversationID string) error {
	return r.db.Where("conversation_id = ?", conversationID).Delete(&model.LLMConversationBinding{}).Error
}

// CleanupExpired deletes all expired bindings.
func (r *ConversationBindingRepository) CleanupExpired() error {
	return r.db.Where("expires_at <= ?", time.Now()).Delete(&model.LLMConversationBinding{}).Error
}
