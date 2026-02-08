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

	notifypub "eddisonso.com/notification-service/pkg/publisher"
)

type Publisher struct {
	nc       *nats.Conn
	js       jetstream.JetStream
	source   string
	notifier *notifypub.Publisher
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

	// Initialize notification publisher (reuses same NATS connection URL)
	notifier, err := notifypub.New(natsURL, source)
	if err != nil {
		slog.Warn("failed to create notification publisher", "error", err)
	}

	return &Publisher{
		nc:       nc,
		js:       js,
		source:   source,
		notifier: notifier,
	}, nil
}

func (p *Publisher) Close() error {
	if p.notifier != nil {
		p.notifier.Close()
	}
	p.nc.Close()
	return nil
}

// Notify sends a user-facing notification via the notification service.
func (p *Publisher) Notify(userID, title, message, link, category string) {
	if p.notifier == nil {
		return
	}
	if err := p.notifier.Notify(context.Background(), userID, title, message, link, category, ""); err != nil {
		slog.Error("failed to send notification", "error", err, "user_id", userID, "title", title)
	}
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

func (p *Publisher) PublishUserCreated(userID, username, displayName string) error {
	subject := fmt.Sprintf("auth.user.%s.created", userID)
	event := UserCreated{
		Metadata:    p.newMetadata(userID),
		UserID:      userID,
		Username:    username,
		DisplayName: displayName,
	}
	if err := p.publish(subject, event); err != nil {
		slog.Error("failed to publish user.created", "error", err, "user_id", userID)
		return err
	}
	slog.Info("published user.created", "user_id", userID, "username", username)
	return nil
}

func (p *Publisher) PublishUserDeleted(userID, username string) error {
	subject := fmt.Sprintf("auth.user.%s.deleted", userID)
	event := UserDeleted{
		Metadata: p.newMetadata(userID),
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

func (p *Publisher) PublishUserUpdated(userID, username, displayName string) error {
	subject := fmt.Sprintf("auth.user.%s.updated", userID)
	event := UserUpdated{
		Metadata:    p.newMetadata(userID),
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

func (p *Publisher) PublishSessionCreated(sessionID int64, userID string, expiresAt time.Time) error {
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

func (p *Publisher) PublishSessionInvalidated(sessionToken, userID string) error {
	subject := fmt.Sprintf("auth.session.%s.invalidated", userID)
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

func (p *Publisher) PublishIdentityPermissionsUpdated(saID, userID string, scopes map[string][]string, version int64) error {
	subject := fmt.Sprintf("auth.identity.%s.updated", saID)
	event := IdentityPermissionsUpdated{
		Metadata:         p.newMetadata(saID),
		ServiceAccountID: saID,
		UserID:           userID,
		Scopes:           scopes,
		Version:          version,
	}
	if err := p.publish(subject, event); err != nil {
		slog.Error("failed to publish identity.updated", "error", err, "sa_id", saID)
		return err
	}
	slog.Info("published identity.updated", "sa_id", saID, "version", version)
	return nil
}

func (p *Publisher) PublishIdentityPermissionsDeleted(saID, userID string, version int64) error {
	subject := fmt.Sprintf("auth.identity.%s.deleted", saID)
	event := IdentityPermissionsDeleted{
		Metadata:         p.newMetadata(saID),
		ServiceAccountID: saID,
		UserID:           userID,
		Version:          version,
	}
	if err := p.publish(subject, event); err != nil {
		slog.Error("failed to publish identity.deleted", "error", err, "sa_id", saID)
		return err
	}
	slog.Info("published identity.deleted", "sa_id", saID, "version", version)
	return nil
}
