package infra

import (
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/singll/bellkeeper/internal/config"
)

// NATSClient wraps NATS client with JetStream support
type NATSClient struct {
	conn    *nats.Conn
	js      nats.JetStreamContext
	streams config.NATSStreamsConfig
}

// NewNATSClient creates a new NATS client with JetStream
func NewNATSClient(cfg config.NATSConfig) (*NATSClient, error) {
	opts := []nats.Option{
		nats.Name("bellkeeper-matrix"),
		nats.ReconnectWait(2 * time.Second),
		nats.MaxReconnects(-1), // unlimited reconnects
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				log.Printf("[NATS] disconnected: %v", err)
			}
		}),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			log.Printf("[NATS] reconnected")
		}),
	}

	conn, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	client := &NATSClient{
		conn:    conn,
		js:      js,
		streams: cfg.Streams,
	}

	// Ensure streams exist
	if err := client.ensureStreams(); err != nil {
		conn.Close()
		return nil, err
	}

	return client, nil
}

// ensureStreams creates JetStream streams if they don't exist
func (n *NATSClient) ensureStreams() error {
	streams := []struct {
		name     string
		subjects []string
	}{
		{
			name:     n.streams.Notifications,
			subjects: []string{n.streams.Notifications + ".>"},
		},
		{
			name:     n.streams.Commands,
			subjects: []string{n.streams.Commands + ".>"},
		},
	}

	for _, s := range streams {
		_, err := n.js.StreamInfo(s.name)
		if err == nats.ErrStreamNotFound {
			_, err = n.js.AddStream(&nats.StreamConfig{
				Name:      s.name,
				Subjects:  s.subjects,
				Retention: nats.WorkQueuePolicy,
				MaxAge:    72 * time.Hour, // keep messages for 3 days
				Storage:   nats.FileStorage,
			})
			if err != nil {
				return fmt.Errorf("failed to create stream %s: %w", s.name, err)
			}
			log.Printf("[NATS] created stream: %s", s.name)
		} else if err != nil {
			return fmt.Errorf("failed to get stream info for %s: %w", s.name, err)
		}
	}

	return nil
}

// Publish publishes a message to a subject
func (n *NATSClient) Publish(subject string, data []byte) error {
	_, err := n.js.Publish(subject, data)
	if err != nil {
		return fmt.Errorf("failed to publish to %s: %w", subject, err)
	}
	return nil
}

// Subscribe creates a durable pull subscription
func (n *NATSClient) Subscribe(subject, durableName string) (*nats.Subscription, error) {
	sub, err := n.js.PullSubscribe(subject, durableName, nats.AckExplicit())
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to %s: %w", subject, err)
	}
	return sub, nil
}

// Close closes the NATS connection
func (n *NATSClient) Close() {
	n.conn.Drain()
}

// JetStream returns the JetStream context for advanced operations
func (n *NATSClient) JetStream() nats.JetStreamContext {
	return n.js
}

// StreamsConfig returns configured stream names
func (n *NATSClient) StreamsConfig() config.NATSStreamsConfig {
	return n.streams
}
