package consumer

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"eddisonso.com/notification-service/internal/db"
	pb "eddisonso.com/notification-service/pkg/pb/notification"
)

type NotificationHandler func(n *db.Notification)

type Consumer struct {
	nc      *nats.Conn
	js      jetstream.JetStream
	db      *db.DB
	handler NotificationHandler
	cancel  context.CancelFunc
}

func New(natsURL string, database *db.DB, handler NotificationHandler) (*Consumer, error) {
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return nil, fmt.Errorf("connect to nats: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("create jetstream: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      "NOTIFICATIONS",
		Subjects:  []string{"notify.>"},
		Retention: jetstream.LimitsPolicy,
		MaxMsgs:   1000000,
		MaxBytes:  1024 * 1024 * 1024,
		MaxAge:    7 * 24 * time.Hour,
		Storage:   jetstream.FileStorage,
	})
	if err != nil {
		slog.Warn("failed to create NOTIFICATIONS stream (may already exist)", "error", err)
	}

	return &Consumer{
		nc:      nc,
		js:      js,
		db:      database,
		handler: handler,
	}, nil
}

func (c *Consumer) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	consumer, err := c.js.CreateOrUpdateConsumer(ctx, "NOTIFICATIONS", jetstream.ConsumerConfig{
		Durable:       "notification-service",
		FilterSubject: "notify.>",
		AckPolicy:     jetstream.AckExplicitPolicy,
		DeliverPolicy: jetstream.DeliverAllPolicy,
		AckWait:       30 * time.Second,
		MaxDeliver:    5,
	})
	if err != nil {
		cancel()
		return fmt.Errorf("create consumer: %w", err)
	}

	go c.consumeLoop(ctx, consumer)
	return nil
}

func (c *Consumer) consumeLoop(ctx context.Context, consumer jetstream.Consumer) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msgs, err := consumer.Fetch(10, jetstream.FetchMaxWait(5*time.Second))
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Error("failed to fetch messages", "error", err)
			time.Sleep(time.Second)
			continue
		}

		for msg := range msgs.Messages() {
			c.processMessage(msg)
		}
	}
}

func (c *Consumer) processMessage(msg jetstream.Msg) {
	var notification pb.Notification
	if err := proto.Unmarshal(msg.Data(), &notification); err != nil {
		slog.Error("failed to unmarshal notification", "error", err, "subject", msg.Subject())
		msg.Ack()
		return
	}

	// Extract user_id from subject (notify.{user_id})
	userID := notification.UserId
	if userID == "" {
		parts := strings.SplitN(msg.Subject(), ".", 2)
		if len(parts) == 2 {
			userID = parts[1]
		}
	}

	if userID == "" {
		slog.Warn("notification missing user_id", "subject", msg.Subject())
		msg.Ack()
		return
	}

	n, err := c.db.Insert(userID, notification.Title, notification.Message, notification.Link, notification.Category)
	if err != nil {
		slog.Error("failed to insert notification", "error", err, "user_id", userID)
		msg.Nak()
		return
	}

	slog.Info("notification stored", "id", n.ID, "user_id", userID, "title", notification.Title)

	if c.handler != nil {
		c.handler(n)
	}

	msg.Ack()
}

func (c *Consumer) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	c.nc.Close()
}
