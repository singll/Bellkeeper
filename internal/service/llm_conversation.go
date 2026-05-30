package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/model"
	"github.com/singll/bellkeeper/internal/repository"
	"go.uber.org/zap"
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
	m.persist(b)
}

// persist asynchronously upserts a binding to the DB so sticky routing survives
// process restarts (without it, bindings live only in memory and are lost on
// reload — defeating prompt-cache protection).
func (m *ConversationBindingManager) persist(b *model.LLMConversationBinding) {
	if m.repo == nil {
		return
	}
	snapshot := *b // copy so the goroutine doesn't race with in-memory mutation
	go func() {
		if err := m.repo.Upsert(&snapshot); err != nil {
			middleware.GetLogger().Warn("failed to persist conversation binding",
				zap.String("conversation_id", snapshot.ConversationID), zap.Error(err))
		}
	}()
}

// Touch updates LastSeenAt and renews TTL for an existing binding.
func (m *ConversationBindingManager) Touch(conversationID string, tokens, costCents int) {
	m.mu.Lock()
	b, ok := m.bindings[conversationID]
	if ok {
		b.LastSeenAt = time.Now()
		b.ExpiresAt = time.Now().Add(m.ttl)
		b.RequestCount++
		b.TotalTokens += tokens
		b.TotalCostCents += costCents
	}
	m.mu.Unlock()
	if ok {
		m.persist(b)
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

// HashCachedPrefix derives a stable conversation ID from the cached prefix of the
// request — the messages up to and including the last one carrying a cache_control
// marker. Prompt caching requires that prefix to be byte-identical across turns, so
// hashing it yields the same id for every turn of a conversation while still
// distinguishing conversations with different cached prefixes.
func HashCachedPrefix(body []byte) string {
	var req struct {
		Messages []json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &req); err != nil || len(req.Messages) == 0 {
		return HashMessages(body) // fall back to whole-body hash on unexpected shape
	}
	lastCC := 0 // anchor on the first message if no per-message marker is found
	for i, msg := range req.Messages {
		if bytes.Contains(msg, []byte("cache_control")) {
			lastCC = i
		}
	}
	h := sha256.New()
	for i := 0; i <= lastCC && i < len(req.Messages); i++ {
		h.Write(req.Messages[i])
	}
	return "conv-" + hex.EncodeToString(h.Sum(nil))[:16]
}

// ImplicitConversationID extracts or generates a conversation ID from request headers/body.
func (m *ConversationBindingManager) ImplicitConversationID(headers map[string]string, body []byte) string {
	if cid := headers["X-Conversation-ID"]; cid != "" {
		return cid
	}
	// Only derive an implicit ID when the request opts into prompt caching (body
	// contains a cache_control marker). Stateless requests get no sticky binding —
	// hashing every body would wrongly pin one-off requests and rarely collide for
	// genuine multi-turn conversations. (ROADMAP §2.6.7)
	if len(body) == 0 || !bytes.Contains(body, []byte("cache_control")) {
		return ""
	}
	return HashCachedPrefix(body)
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
