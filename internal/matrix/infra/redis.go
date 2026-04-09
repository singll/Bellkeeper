package infra

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/singll/bellkeeper/internal/config"
)

// RedisClient wraps redis client with Matrix-specific operations
type RedisClient struct {
	client *redis.Client
}

// NewRedisClient creates a new Redis client
func NewRedisClient(cfg config.RedisConfig) (*RedisClient, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisClient{client: client}, nil
}

// Close closes the Redis connection
func (r *RedisClient) Close() error {
	return r.client.Close()
}

// GetSyncToken retrieves the sync token for a bot user
func (r *RedisClient) GetSyncToken(ctx context.Context, botUserID string) (string, error) {
	key := fmt.Sprintf("matrix:sync_token:%s", botUserID)
	val, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil // No token stored yet
	}
	if err != nil {
		return "", fmt.Errorf("failed to get sync token: %w", err)
	}
	return val, nil
}

// SetSyncToken stores the sync token for a bot user
func (r *RedisClient) SetSyncToken(ctx context.Context, botUserID, token string) error {
	key := fmt.Sprintf("matrix:sync_token:%s", botUserID)
	if err := r.client.Set(ctx, key, token, 0).Err(); err != nil {
		return fmt.Errorf("failed to set sync token: %w", err)
	}
	return nil
}

// CheckEventProcessed checks if an event has been processed (for deduplication)
func (r *RedisClient) CheckEventProcessed(ctx context.Context, eventID string) (bool, error) {
	key := fmt.Sprintf("matrix:event_processed:%s", eventID)
	val, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check event: %w", err)
	}
	return val > 0, nil
}

// MarkEventProcessed marks an event as processed with TTL (24 hours)
func (r *RedisClient) MarkEventProcessed(ctx context.Context, eventID string) error {
	key := fmt.Sprintf("matrix:event_processed:%s", eventID)
	if err := r.client.Set(ctx, key, "1", 24*time.Hour).Err(); err != nil {
		return fmt.Errorf("failed to mark event processed: %w", err)
	}
	return nil
}

// AcquireLock acquires a distributed lock
func (r *RedisClient) AcquireLock(ctx context.Context, lockKey string, ttl time.Duration) (bool, error) {
	key := fmt.Sprintf("matrix:lock:%s", lockKey)
	ok, err := r.client.SetNX(ctx, key, "1", ttl).Result()
	if err != nil {
		return false, fmt.Errorf("failed to acquire lock: %w", err)
	}
	return ok, nil
}

// ReleaseLock releases a distributed lock
func (r *RedisClient) ReleaseLock(ctx context.Context, lockKey string) error {
	key := fmt.Sprintf("matrix:lock:%s", lockKey)
	if err := r.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}
	return nil
}

// IncrRateLimit increments rate limit counter and returns current count
func (r *RedisClient) IncrRateLimit(ctx context.Context, key string, window time.Duration) (int64, error) {
	rateLimitKey := fmt.Sprintf("matrix:ratelimit:%s", key)

	pipe := r.client.Pipeline()
	incr := pipe.Incr(ctx, rateLimitKey)
	pipe.Expire(ctx, rateLimitKey, window)

	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("failed to increment rate limit: %w", err)
	}

	return incr.Val(), nil
}

// GetClient returns the underlying Redis client for advanced operations
func (r *RedisClient) GetClient() *redis.Client {
	return r.client
}
