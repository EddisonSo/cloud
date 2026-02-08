package publisher

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	pbcommon "eddisonso.com/notification-service/pkg/pb/common"
	pb "eddisonso.com/notification-service/pkg/pb/notification"
)

type Publisher struct {
	js     jetstream.JetStream
	nc     *nats.Conn
	source string
}

func New(natsURL, source string) (*Publisher, error) {
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

	// Ensure NOTIFICATIONS stream exists
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

	return &Publisher{
		js:     js,
		nc:     nc,
		source: source,
	}, nil
}

func (p *Publisher) Close() {
	p.nc.Close()
}

func (p *Publisher) Notify(ctx context.Context, userID, title, message, link, category, scope string) error {
	notification := &pb.Notification{
		Metadata: &pbcommon.EventMetadata{
			EventId:  generateUUID(),
			EntityId: userID,
			Timestamp: &pbcommon.Timestamp{
				Seconds: time.Now().Unix(),
			},
			Source: p.source,
		},
		UserId:   userID,
		Title:    title,
		Message:  message,
		Link:     link,
		Category: category,
		Scope:    scope,
	}

	data, err := proto.Marshal(notification)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}

	subject := fmt.Sprintf("notify.%s", userID)
	pubCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if _, err := p.js.Publish(pubCtx, subject, data); err != nil {
		return fmt.Errorf("publish notification: %w", err)
	}

	slog.Debug("published notification", "subject", subject, "title", title)
	return nil
}

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
