package db

import (
	"database/sql"
	"time"

	_ "github.com/lib/pq"
)

type DB struct {
	conn *sql.DB
}

type Notification struct {
	ID        int64     `json:"id"`
	UserID    string    `json:"user_id"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	Link      string    `json:"link"`
	Category  string    `json:"category"`
	Scope     string    `json:"scope"`
	Read      bool      `json:"read"`
	CreatedAt time.Time `json:"created_at"`
}

type NotificationMute struct {
	ID        int64     `json:"id"`
	UserID    string    `json:"user_id"`
	Category  string    `json:"category"`
	Scope     string    `json:"scope"`
	CreatedAt time.Time `json:"created_at"`
}

func Open(databaseURL string) (*DB, error) {
	conn, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
	}
	if err := conn.Ping(); err != nil {
		return nil, err
	}
	return &DB{conn: conn}, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) Migrate() error {
	_, err := db.conn.Exec(`
		CREATE TABLE IF NOT EXISTS notifications (
			id BIGSERIAL PRIMARY KEY,
			user_id TEXT NOT NULL,
			title TEXT NOT NULL,
			message TEXT NOT NULL,
			link TEXT NOT NULL DEFAULT '',
			category TEXT NOT NULL DEFAULT '',
			scope TEXT NOT NULL DEFAULT '',
			read BOOLEAN NOT NULL DEFAULT false,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_notifications_user_unread
			ON notifications (user_id, read, created_at DESC);

		ALTER TABLE notifications ADD COLUMN IF NOT EXISTS scope TEXT NOT NULL DEFAULT '';

		CREATE TABLE IF NOT EXISTS notification_mutes (
			id BIGSERIAL PRIMARY KEY,
			user_id TEXT NOT NULL,
			category TEXT NOT NULL,
			scope TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, category, scope)
		);
		CREATE INDEX IF NOT EXISTS idx_notification_mutes_user
			ON notification_mutes (user_id);
	`)
	return err
}

func (db *DB) Insert(userID, title, message, link, category, scope string) (*Notification, error) {
	n := &Notification{
		UserID:   userID,
		Title:    title,
		Message:  message,
		Link:     link,
		Category: category,
		Scope:    scope,
	}
	err := db.conn.QueryRow(
		`INSERT INTO notifications (user_id, title, message, link, category, scope)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, created_at`,
		userID, title, message, link, category, scope,
	).Scan(&n.ID, &n.CreatedAt)
	if err != nil {
		return nil, err
	}
	return n, nil
}

func (db *DB) ListByUser(userID string, limit, offset int) ([]Notification, error) {
	rows, err := db.conn.Query(
		`SELECT id, user_id, title, message, link, category, scope, read, created_at
		 FROM notifications
		 WHERE user_id = $1
		 ORDER BY created_at DESC
		 LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notifications []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.Title, &n.Message, &n.Link, &n.Category, &n.Scope, &n.Read, &n.CreatedAt); err != nil {
			return nil, err
		}
		notifications = append(notifications, n)
	}
	return notifications, rows.Err()
}

func (db *DB) UnreadCount(userID string) (int, error) {
	var count int
	err := db.conn.QueryRow(
		`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND read = false`,
		userID,
	).Scan(&count)
	return count, err
}

func (db *DB) MarkRead(id int64, userID string) error {
	_, err := db.conn.Exec(
		`UPDATE notifications SET read = true WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
	return err
}

func (db *DB) MarkAllRead(userID string) error {
	_, err := db.conn.Exec(
		`UPDATE notifications SET read = true WHERE user_id = $1 AND read = false`,
		userID,
	)
	return err
}

func (db *DB) IsMuted(userID, category, scope string) (bool, error) {
	var count int
	err := db.conn.QueryRow(
		`SELECT COUNT(*) FROM notification_mutes WHERE user_id = $1 AND category = $2 AND scope = $3`,
		userID, category, scope,
	).Scan(&count)
	return count > 0, err
}

func (db *DB) ListMutes(userID string) ([]NotificationMute, error) {
	rows, err := db.conn.Query(
		`SELECT id, user_id, category, scope, created_at
		 FROM notification_mutes
		 WHERE user_id = $1
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mutes []NotificationMute
	for rows.Next() {
		var m NotificationMute
		if err := rows.Scan(&m.ID, &m.UserID, &m.Category, &m.Scope, &m.CreatedAt); err != nil {
			return nil, err
		}
		mutes = append(mutes, m)
	}
	return mutes, rows.Err()
}

func (db *DB) AddMute(userID, category, scope string) error {
	_, err := db.conn.Exec(
		`INSERT INTO notification_mutes (user_id, category, scope)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, category, scope) DO NOTHING`,
		userID, category, scope,
	)
	return err
}

func (db *DB) RemoveMute(userID, category, scope string) error {
	_, err := db.conn.Exec(
		`DELETE FROM notification_mutes WHERE user_id = $1 AND category = $2 AND scope = $3`,
		userID, category, scope,
	)
	return err
}
