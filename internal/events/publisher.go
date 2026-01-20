package events

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type Publisher struct {
	nc     *nats.Conn
	js     jetstream.JetStream
	source string
}

func NewPublisher(natsURL, source string) (*Publisher, error) {
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

	// Ensure AUTH stream exists
	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      "AUTH",
		Subjects:  []string{"auth.>"},
		Retention: jetstream.LimitsPolicy,
		MaxMsgs:   1000000,
		MaxBytes:  1024 * 1024 * 1024, // 1GB
		MaxAge:    7 * 24 * time.Hour, // 7 days
		Storage:   jetstream.FileStorage,
	})
	if err != nil {
		slog.Warn("failed to create AUTH stream (may already exist)", "error", err)
	}

	return &Publisher{
		nc:     nc,
		js:     js,
		source: source,
	}, nil
}

func (p *Publisher) Close() error {
	p.nc.Close()
	return nil
}

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func (p *Publisher) newMetadata(entityID string) EventMetadata {
	return EventMetadata{
		EventID:   generateUUID(),
		EntityID:  entityID,
		Timestamp: time.Now().Unix(),
		Source:    p.source,
	}
}

func (p *Publisher) publish(subject string, event interface{}) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = p.js.Publish(ctx, subject, data)
	if err != nil {
		return fmt.Errorf("publish event: %w", err)
	}

	slog.Debug("published event", "subject", subject)
	return nil
}

func (p *Publisher) PublishUserCreated(userID int64, username, displayName, publicID string) error {
	subject := fmt.Sprintf("auth.user.%d.created", userID)
	event := UserCreated{
		Metadata:    p.newMetadata(fmt.Sprintf("%d", userID)),
		UserID:      userID,
		Username:    username,
		DisplayName: displayName,
		PublicID:    publicID,
	}
	if err := p.publish(subject, event); err != nil {
		slog.Error("failed to publish user.created", "error", err, "user_id", userID)
		return err
	}
	slog.Info("published user.created", "user_id", userID, "username", username)
	return nil
}

func (p *Publisher) PublishUserDeleted(userID int64, username string) error {
	subject := fmt.Sprintf("auth.user.%d.deleted", userID)
	event := UserDeleted{
		Metadata: p.newMetadata(fmt.Sprintf("%d", userID)),
		UserID:   userID,
		Username: username,
	}
	if err := p.publish(subject, event); err != nil {
		slog.Error("failed to publish user.deleted", "error", err, "user_id", userID)
		return err
	}
	slog.Info("published user.deleted", "user_id", userID, "username", username)
	return nil
}

func (p *Publisher) PublishUserUpdated(userID int64, username, displayName string) error {
	subject := fmt.Sprintf("auth.user.%d.updated", userID)
	event := UserUpdated{
		Metadata:    p.newMetadata(fmt.Sprintf("%d", userID)),
		UserID:      userID,
		Username:    username,
		DisplayName: displayName,
	}
	if err := p.publish(subject, event); err != nil {
		slog.Error("failed to publish user.updated", "error", err, "user_id", userID)
		return err
	}
	slog.Info("published user.updated", "user_id", userID, "username", username)
	return nil
}

func (p *Publisher) PublishSessionCreated(sessionID, userID int64, expiresAt time.Time) error {
	subject := fmt.Sprintf("auth.session.%d.created", sessionID)
	event := SessionCreated{
		Metadata:  p.newMetadata(fmt.Sprintf("%d", sessionID)),
		SessionID: fmt.Sprintf("%d", sessionID),
		UserID:    userID,
		ExpiresAt: expiresAt.Unix(),
	}
	if err := p.publish(subject, event); err != nil {
		slog.Error("failed to publish session.created", "error", err, "session_id", sessionID)
		return err
	}
	slog.Debug("published session.created", "session_id", sessionID, "user_id", userID)
	return nil
}

func (p *Publisher) PublishSessionInvalidated(sessionToken string, userID int64) error {
	subject := fmt.Sprintf("auth.session.%d.invalidated", userID)
	event := SessionInvalidated{
		Metadata:  p.newMetadata(sessionToken),
		SessionID: sessionToken,
		UserID:    userID,
	}
	if err := p.publish(subject, event); err != nil {
		slog.Error("failed to publish session.invalidated", "error", err, "user_id", userID)
		return err
	}
	slog.Debug("published session.invalidated", "user_id", userID)
	return nil
}
