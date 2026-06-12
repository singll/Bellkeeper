package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/singll/bellkeeper/internal/llmclient"
	"github.com/singll/bellkeeper/internal/middleware"
	"go.uber.org/zap"
)

type RateLimiter struct {
	redis *redis.Client
	max   int64
}

func NewRateLimiter(redisClient *redis.Client, maxTurnsPerHour int64) *RateLimiter {
	return &RateLimiter{
		redis: redisClient,
		max:   maxTurnsPerHour,
	}
}

func (r *RateLimiter) key(roomID string) string {
	return fmt.Sprintf("matrix:agent:ratelimit:%s", roomID)
}

func (r *RateLimiter) Allow(ctx context.Context, roomID string) (bool, int64, error) {
	key := r.key(roomID)
	pipe := r.redis.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, time.Hour)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, 0, fmt.Errorf("rate limit check: %w", err)
	}
	count := incr.Val()
	if count > r.max {
		return false, count, nil
	}
	return true, count, nil
}

func (r *RateLimiter) Reset(ctx context.Context, roomID string) error {
	return r.redis.Del(ctx, r.key(roomID)).Err()
}

type ToolLevel string

const (
	LevelReadonly ToolLevel = "readonly"
	LevelWrite   ToolLevel = "write"
	LevelDanger  ToolLevel = "danger"
)

type ToolDefinition struct {
	Tool        llmclient.Tool
	Level       ToolLevel
	Handler     func(ctx context.Context, args json.RawMessage) (string, error)
}

type ToolRegistry struct {
	tools map[string]*ToolDefinition
}

func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]*ToolDefinition),
	}
}

func (r *ToolRegistry) Register(def *ToolDefinition) {
	name := def.Tool.Function.Name
	r.tools[name] = def
	middleware.GetLogger().Info("registered agent tool", zap.String("name", name), zap.String("level", string(def.Level)))
}

func (r *ToolRegistry) Get(name string) (*ToolDefinition, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func (r *ToolRegistry) List() []llmclient.Tool {
	tools := make([]llmclient.Tool, 0, len(r.tools))
	for _, def := range r.tools {
		tools = append(tools, def.Tool)
	}
	return tools
}

func (r *ToolRegistry) Execute(ctx context.Context, name string, args json.RawMessage) (string, error) {
	def, ok := r.tools[name]
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	return def.Handler(ctx, args)
}

func (r *ToolRegistry) CheckPermission(name string, isAdmin bool) error {
	def, ok := r.tools[name]
	if !ok {
		return fmt.Errorf("unknown tool: %s", name)
	}
	if def.Level == LevelWrite && !isAdmin {
		return fmt.Errorf("tool %s requires admin permission", name)
	}
	if def.Level == LevelDanger {
		return fmt.Errorf("tool %s requires explicit confirmation", name)
	}
	return nil
}
