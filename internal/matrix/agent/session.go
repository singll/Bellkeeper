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

// userModelTTL 是 per-user 模型组覆盖的过期时间（远长于会话 TTL，近似「持久」）。
const userModelTTL = 720 * time.Hour // 30 天

func (s *SessionStore) userModelKey(userID string) string {
	return fmt.Sprintf("matrix:agent:usermodel:%s", userID)
}

// GetUserModel 返回该用户持久化的模型组覆盖；无则返回空串。
func (s *SessionStore) GetUserModel(ctx context.Context, userID string) (string, error) {
	v, err := s.redis.Get(ctx, s.userModelKey(userID)).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get user model: %w", err)
	}
	return v, nil
}

// SetUserModel 持久化该用户的模型组覆盖（独立于会话 key，故 !reset 不清除）。
// group 为空串表示清除覆盖、回退到房间默认模型。
func (s *SessionStore) SetUserModel(ctx context.Context, userID, group string) error {
	if group == "" {
		if err := s.redis.Del(ctx, s.userModelKey(userID)).Err(); err != nil {
			return fmt.Errorf("clear user model: %w", err)
		}
		return nil
	}
	if err := s.redis.Set(ctx, s.userModelKey(userID), group, userModelTTL).Err(); err != nil {
		return fmt.Errorf("set user model: %w", err)
	}
	return nil
}
