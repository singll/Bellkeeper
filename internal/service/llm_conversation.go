package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
)

// ConversationBindingManager manages X-Conversation-ID → channel bindings
// with 24h TTL to protect prompt cache across requests.
type ConversationBindingManager struct {
	mu       sync.RWMutex
	bindings map[string]*model.LLMConversationBinding // in-memory cache
	ttl      time.Duration
	repo     *repository.ConversationBindingRepository
}

// NewConversationBindingManager creates a new manager with the given TTL.
func NewConversationBindingManager(repo *repository.ConversationBindingRepository, ttl time.Duration) *ConversationBindingManager {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &ConversationBindingManager{
		bindings: make(map[string]*model.LLMConversationBinding),
		ttl:      ttl,
		repo:     repo,
	}
}

// Get returns the binding for a conversation ID, or nil if expired/not found.
func (m *ConversationBindingManager) Get(conversationID string) *model.LLMConversationBinding {
	m.mu.RLock()
	b, ok := m.bindings[conversationID]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	if time.Now().After(b.ExpiresAt) {
		m.mu.Lock()
		delete(m.bindings, conversationID)
		m.mu.Unlock()
		return nil
	}
	return b
}

// Set creates or updates a binding.
func (m *ConversationBindingManager) Set(conversationID string, channelID uint, channelName, modelName, taskType string) {
	now := time.Now()
	b := &model.LLMConversationBinding{
		ConversationID: conversationID,
		ChannelID:      channelID,
		ChannelName:    channelName,
		Model:          modelName,
		TaskType:       taskType,
		FirstSeenAt:    now,
		LastSeenAt:     now,
		ExpiresAt:      now.Add(m.ttl),
	}
	m.mu.Lock()
	if existing, ok := m.bindings[conversationID]; ok {
		b.FirstSeenAt = existing.FirstSeenAt
		b.RequestCount = existing.RequestCount
		b.TotalTokens = existing.TotalTokens
		b.TotalCostCents = existing.TotalCostCents
	}
	m.bindings[conversationID] = b
	m.mu.Unlock()
}

// Touch updates LastSeenAt and renews TTL for an existing binding.
func (m *ConversationBindingManager) Touch(conversationID string, tokens, costCents int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if b, ok := m.bindings[conversationID]; ok {
		b.LastSeenAt = time.Now()
		b.ExpiresAt = time.Now().Add(m.ttl)
		b.RequestCount++
		b.TotalTokens += tokens
		b.TotalCostCents += costCents
	}
}

// Remove deletes a binding.
func (m *ConversationBindingManager) Remove(conversationID string) {
	m.mu.Lock()
	delete(m.bindings, conversationID)
	m.mu.Unlock()
}

// CleanupExpired removes expired bindings from memory and optionally from DB.
func (m *ConversationBindingManager) CleanupExpired() {
	m.mu.Lock()
	now := time.Now()
	for id, b := range m.bindings {
		if now.After(b.ExpiresAt) {
			delete(m.bindings, id)
		}
	}
	m.mu.Unlock()
	if m.repo != nil {
		_ = m.repo.CleanupExpired()
	}
}

// List returns all active bindings.
func (m *ConversationBindingManager) List() []*model.LLMConversationBinding {
	m.mu.RLock()
	defer m.mu.RUnlock()
	now := time.Now()
	result := make([]*model.LLMConversationBinding, 0, len(m.bindings))
	for _, b := range m.bindings {
		if now.Before(b.ExpiresAt) {
			result = append(result, b)
		}
	}
	return result
}

// LoadFromDB preloads active bindings from the database into memory.
func (m *ConversationBindingManager) LoadFromDB() error {
	if m.repo == nil {
		return nil
	}
	bindings, err := m.repo.List()
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range bindings {
		b := &bindings[i]
		if time.Now().Before(b.ExpiresAt) {
			m.bindings[b.ConversationID] = b
		}
	}
	return nil
}

// HashMessages computes a deterministic conversation ID from messages.
func HashMessages(messagesJSON []byte) string {
	h := sha256.New()
	h.Write(messagesJSON)
	return "conv-" + hex.EncodeToString(h.Sum(nil))[:16]
}

// ImplicitConversationID extracts or generates a conversation ID from request headers/body.
func (m *ConversationBindingManager) ImplicitConversationID(headers map[string]string, body []byte) string {
	if cid := headers["X-Conversation-ID"]; cid != "" {
		return cid
	}
	// If body contains cache_control, use messages hash as implicit ID
	if len(body) > 0 {
		return HashMessages(body)
	}
	return ""
}

// ValidateBinding checks if a binding is still valid for the given channel.
// Returns nil if valid, error with reason if not.
func (m *ConversationBindingManager) ValidateBinding(binding *model.LLMConversationBinding, channelHealth func(string) bool) error {
	if binding == nil {
		return fmt.Errorf("no binding")
	}
	if time.Now().After(binding.ExpiresAt) {
		return fmt.Errorf("binding expired")
	}
	if !channelHealth(binding.ChannelName) {
		return fmt.Errorf("bound channel %s is unavailable", binding.ChannelName)
	}
	return nil
}
