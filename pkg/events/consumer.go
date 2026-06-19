package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// EventHandler handles user events
type EventHandler interface {
	OnUserCreated(ctx context.Context, event UserCreated) error
	OnUserDeleted(ctx context.Context, event UserDeleted) error
	OnUserUpdated(ctx context.Context, event UserUpdated) error
}

// IdentityHandler handles identity permission events
type IdentityHandler interface {
	OnIdentityUpdated(ctx context.Context, saID, userID string, scopes map[string][]string, version int64) error
	OnIdentityDeleted(ctx context.Context, saID, userID string, version int64) error
}

// Consumer handles NATS JetStream event consumption
type Consumer struct {
	nc           *nats.Conn
	js           jetstream.JetStream
	handler      EventHandler
	consumerName string
	cancel       context.CancelFunc
}

// ConsumerConfig holds configuration for the consumer
type ConsumerConfig struct {
	NatsURL      string
	ConsumerName string // Unique name for this consumer (e.g., "sfs-backend", "compute")
	Handler      EventHandler
}

// NewConsumer creates a new NATS JetStream consumer
func NewConsumer(cfg ConsumerConfig) (*Consumer, error) {
	nc, err := nats.Connect(cfg.NatsURL,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			slog.Warn("NATS disconnected", "error", err)
		}),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			slog.Info("NATS reconnected")
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to nats: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("create jetstream: %w", err)
	}

	return &Consumer{
		nc:           nc,
		js:           js,
		handler:      cfg.Handler,
		consumerName: cfg.ConsumerName,
	}, nil
}

// Start begins consuming events from the AUTH stream
func (c *Consumer) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	// Get or create durable consumer on AUTH stream
	consumer, err := c.js.CreateOrUpdateConsumer(ctx, "AUTH", jetstream.ConsumerConfig{
		Durable:       c.consumerName,
		FilterSubject: "auth.user.>",
		AckPolicy:     jetstream.AckExplicitPolicy,
		DeliverPolicy: jetstream.DeliverAllPolicy, // Start from beginning to catch up
		AckWait:       30 * time.Second,
		MaxDeliver:    5, // Retry up to 5 times
	})
	if err != nil {
		return fmt.Errorf("create consumer: %w", err)
	}

	slog.Info("starting event consumer", "consumer", c.consumerName)

	// Start consuming messages
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				msgs, err := consumer.Fetch(10, jetstream.FetchMaxWait(5*time.Second))
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					slog.Warn("fetch messages error", "error", err)
					time.Sleep(time.Second)
					continue
				}

				for msg := range msgs.Messages() {
					if err := c.handleMessage(ctx, msg); err != nil {
						slog.Error("handle message failed", "error", err, "subject", msg.Subject())
						// NAK will trigger redelivery
						msg.Nak()
					} else {
						msg.Ack()
					}
				}
			}
		}
	}()

	return nil
}

// handleMessage processes a single message
func (c *Consumer) handleMessage(ctx context.Context, msg jetstream.Msg) error {
	subject := msg.Subject()
	slog.Debug("processing event", "subject", subject)

	// Parse subject: auth.user.{userID}.{action}
	parts := strings.Split(subject, ".")
	if len(parts) != 4 || parts[0] != "auth" || parts[1] != "user" {
		slog.Warn("unknown subject format", "subject", subject)
		return nil // Ack and skip unknown subjects
	}

	action := parts[3]
	data := msg.Data()

	switch action {
	case "created":
		var event UserCreated
		if err := json.Unmarshal(data, &event); err != nil {
			return fmt.Errorf("unmarshal user.created: %w", err)
		}
		return c.handler.OnUserCreated(ctx, event)

	case "deleted":
		var event UserDeleted
		if err := json.Unmarshal(data, &event); err != nil {
			return fmt.Errorf("unmarshal user.deleted: %w", err)
		}
		return c.handler.OnUserDeleted(ctx, event)

	case "updated":
		var event UserUpdated
		if err := json.Unmarshal(data, &event); err != nil {
			return fmt.Errorf("unmarshal user.updated: %w", err)
		}
		return c.handler.OnUserUpdated(ctx, event)

	default:
		slog.Warn("unknown action", "action", action)
		return nil // Ack and skip unknown actions
	}
}

// Stop stops the consumer
func (c *Consumer) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	if c.nc != nil {
		c.nc.Close()
	}
}

// IdentityConsumer handles NATS JetStream identity permission events
type IdentityConsumer struct {
	nc           *nats.Conn
	js           jetstream.JetStream
	handler      IdentityHandler
	consumerName string
	cancel       context.CancelFunc
}

// IdentityConsumerConfig holds configuration for the identity consumer
type IdentityConsumerConfig struct {
	NatsURL      string
	ConsumerName string // Unique name (e.g., "compute-identity", "sfs-identity")
	Handler      IdentityHandler
}

// NewIdentityConsumer creates a new NATS JetStream consumer for identity events
func NewIdentityConsumer(cfg IdentityConsumerConfig) (*IdentityConsumer, error) {
	nc, err := nats.Connect(cfg.NatsURL,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			slog.Warn("NATS identity consumer disconnected", "error", err)
		}),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			slog.Info("NATS identity consumer reconnected")
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("connect to nats: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("create jetstream: %w", err)
	}

	return &IdentityConsumer{
		nc:           nc,
		js:           js,
		handler:      cfg.Handler,
		consumerName: cfg.ConsumerName,
	}, nil
}

// Start begins consuming identity events from the AUTH stream
func (c *IdentityConsumer) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	consumer, err := c.js.CreateOrUpdateConsumer(ctx, "AUTH", jetstream.ConsumerConfig{
		Durable:       c.consumerName,
		FilterSubject: "auth.identity.>",
		AckPolicy:     jetstream.AckExplicitPolicy,
		DeliverPolicy: jetstream.DeliverAllPolicy,
		AckWait:       30 * time.Second,
		MaxDeliver:    5,
	})
	if err != nil {
		return fmt.Errorf("create identity consumer: %w", err)
	}

	slog.Info("starting identity event consumer", "consumer", c.consumerName)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				msgs, err := consumer.Fetch(10, jetstream.FetchMaxWait(5*time.Second))
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					slog.Warn("identity fetch messages error", "error", err)
					time.Sleep(time.Second)
					continue
				}

				for msg := range msgs.Messages() {
					if err := c.handleMessage(ctx, msg); err != nil {
						slog.Error("identity handle message failed", "error", err, "subject", msg.Subject())
						msg.Nak()
					} else {
						msg.Ack()
					}
				}
			}
		}
	}()

	return nil
}

func (c *IdentityConsumer) handleMessage(ctx context.Context, msg jetstream.Msg) error {
	subject := msg.Subject()
	slog.Debug("processing identity event", "subject", subject)

	// Parse subject: auth.identity.{saID}.{action}
	parts := strings.Split(subject, ".")
	if len(parts) != 4 || parts[0] != "auth" || parts[1] != "identity" {
		slog.Warn("unknown identity subject format", "subject", subject)
		return nil
	}

	action := parts[3]
	data := msg.Data()

	switch action {
	case "updated":
		var event IdentityPermissionsUpdated
		if err := json.Unmarshal(data, &event); err != nil {
			return fmt.Errorf("unmarshal identity.updated: %w", err)
		}
		return c.handler.OnIdentityUpdated(ctx, event.ServiceAccountID, event.UserID, event.Scopes, event.Version)

	case "deleted":
		var event IdentityPermissionsDeleted
		if err := json.Unmarshal(data, &event); err != nil {
			return fmt.Errorf("unmarshal identity.deleted: %w", err)
		}
		return c.handler.OnIdentityDeleted(ctx, event.ServiceAccountID, event.UserID, event.Version)

	default:
		slog.Warn("unknown identity action", "action", action)
		return nil
	}
}

// Stop stops the identity consumer
func (c *IdentityConsumer) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	if c.nc != nil {
		c.nc.Close()
	}
}
