package gateway

import (
	"context"
	"fmt"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/matrix/infra"
	"github.com/singll/bellkeeper/internal/middleware"
	"github.com/singll/bellkeeper/internal/repository"
	"go.uber.org/zap"
)

// Client wraps mautrix client with additional functionality
type Client struct {
	client *mautrix.Client
	config config.MatrixConfig
	redis  *infra.RedisClient
	repos  *repository.Repositories
}

// NewClient creates a new Matrix client
func NewClient(
	cfg config.MatrixConfig,
	redis *infra.RedisClient,
	repos *repository.Repositories,
) (*Client, error) {
	client, err := mautrix.NewClient(cfg.HomeserverURL, id.UserID(cfg.BotUserID), cfg.BotAccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create Matrix client: %w", err)
	}

	// Set device ID if provided
	if cfg.DeviceID != "" {
		client.DeviceID = id.DeviceID(cfg.DeviceID)
	}

	// Test connection by getting whoami
	whoami, err := client.Whoami(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate with Matrix homeserver: %w", err)
	}

	middleware.GetLogger().Info("authenticated with Matrix homeserver",
		zap.String("user_id", whoami.UserID.String()),
		zap.String("device", string(whoami.DeviceID)))

	return &Client{
		client: client,
		config: cfg,
		redis:  redis,
		repos:  repos,
	}, nil
}

// GetClient returns the underlying mautrix client
func (c *Client) GetClient() *mautrix.Client {
	return c.client
}

// SendMessage sends a text message to a room
func (c *Client) SendMessage(ctx context.Context, roomID, message string) (string, error) {
	resp, err := c.client.SendText(ctx, id.RoomID(roomID), message)
	if err != nil {
		return "", fmt.Errorf("failed to send message: %w", err)
	}
	return resp.EventID.String(), nil
}

// SendHTMLMessage sends an HTML formatted message to a room
func (c *Client) SendHTMLMessage(ctx context.Context, roomID, htmlBody, textBody string) (string, error) {
	content := map[string]interface{}{
		"msgtype":        "m.text",
		"body":           textBody,
		"format":         "org.matrix.custom.html",
		"formatted_body": htmlBody,
	}

	resp, err := c.client.SendMessageEvent(ctx, id.RoomID(roomID), event.EventMessage, content)
	if err != nil {
		return "", fmt.Errorf("failed to send HTML message: %w", err)
	}
	return resp.EventID.String(), nil
}

// JoinRoom joins a Matrix room
func (c *Client) JoinRoom(ctx context.Context, roomID string) error {
	_, err := c.client.JoinRoom(ctx, roomID, nil)
	if err != nil {
		return fmt.Errorf("failed to join room %s: %w", roomID, err)
	}
	middleware.GetLogger().Info("joined room", zap.String("room_id", roomID))
	return nil
}

// LeaveRoom leaves a Matrix room
func (c *Client) LeaveRoom(ctx context.Context, roomID string) error {
	_, err := c.client.LeaveRoom(ctx, id.RoomID(roomID), nil)
	if err != nil {
		return fmt.Errorf("failed to leave room %s: %w", roomID, err)
	}
	middleware.GetLogger().Info("left room", zap.String("room_id", roomID))
	return nil
}

// GetConfig returns the Matrix configuration
func (c *Client) GetConfig() config.MatrixConfig {
	return c.config
}

// GetRedis returns the Redis client
func (c *Client) GetRedis() *infra.RedisClient {
	return c.redis
}

// GetRepos returns the repositories
func (c *Client) GetRepos() *repository.Repositories {
	return c.repos
}
