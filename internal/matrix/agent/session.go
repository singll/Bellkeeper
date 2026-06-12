package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/singll/bellkeeper/internal/llmclient"
)

type SessionStore struct {
	redis  *redis.Client
	ttl    time.Duration
	maxMsg int
}

func NewSessionStore(redisClient *redis.Client, ttl time.Duration, maxMsg int) *SessionStore {
	if maxMsg <= 0 {
		maxMsg = 20
	}
	return &SessionStore{
		redis:  redisClient,
		ttl:    ttl,
		maxMsg: maxMsg,
	}
}

func (s *SessionStore) key(roomID string) string {
	return fmt.Sprintf("matrix:agent:session:%s", roomID)
}

func (s *SessionStore) Get(ctx context.Context, roomID string) ([]llmclient.ChatMessage, error) {
	raw, err := s.redis.Get(ctx, s.key(roomID)).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	var msgs []llmclient.ChatMessage
	if err := json.Unmarshal(raw, &msgs); err != nil {
		return nil, fmt.Errorf("unmarshal session: %w", err)
	}
	return msgs, nil
}

func (s *SessionStore) Append(ctx context.Context, roomID string, msgs ...llmclient.ChatMessage) error {
	existing, err := s.Get(ctx, roomID)
	if err != nil {
		return err
	}
	existing = append(existing, msgs...)
	if len(existing) > s.maxMsg {
		start := len(existing) - s.maxMsg
		if existing[0].Role == "system" && start > 0 {
			system := existing[0]
			existing = append([]llmclient.ChatMessage{system}, existing[start+1:]...)
		} else {
			existing = existing[start:]
		}
	}
	data, err := json.Marshal(existing)
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	if err := s.redis.Set(ctx, s.key(roomID), data, s.ttl).Err(); err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return nil
}

func (s *SessionStore) Clear(ctx context.Context, roomID string) error {
	return s.redis.Del(ctx, s.key(roomID)).Err()
}
