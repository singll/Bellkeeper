package repository

import (
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// Upsert inserts or updates a binding keyed by conversation_id (atomic). Used by
// the in-memory manager to persist bindings so sticky routing survives restarts.
func (r *ConversationBindingRepository) Upsert(b *model.LLMConversationBinding) error {
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "conversation_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"channel_id", "channel_name", "model", "task_type",
			"last_seen_at", "expires_at", "request_count", "total_tokens", "total_cost_cents",
		}),
	}).Create(b).Error
}

// Delete removes a binding by conversation ID.
func (r *ConversationBindingRepository) Delete(conversationID string) error {
	return r.db.Where("conversation_id = ?", conversationID).Delete(&model.LLMConversationBinding{}).Error
}

// CleanupExpired deletes all expired bindings.
func (r *ConversationBindingRepository) CleanupExpired() error {
	return r.db.Where("expires_at <= ?", time.Now()).Delete(&model.LLMConversationBinding{}).Error
}
