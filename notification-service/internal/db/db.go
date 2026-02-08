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
	Read      bool      `json:"read"`
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
			read BOOLEAN NOT NULL DEFAULT false,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_notifications_user_unread
			ON notifications (user_id, read, created_at DESC);
	`)
	return err
}

func (db *DB) Insert(userID, title, message, link, category string) (*Notification, error) {
	n := &Notification{
		UserID:   userID,
		Title:    title,
		Message:  message,
		Link:     link,
		Category: category,
	}
	err := db.conn.QueryRow(
		`INSERT INTO notifications (user_id, title, message, link, category)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, created_at`,
		userID, title, message, link, category,
	).Scan(&n.ID, &n.CreatedAt)
	if err != nil {
		return nil, err
	}
	return n, nil
}

func (db *DB) ListByUser(userID string, limit, offset int) ([]Notification, error) {
	rows, err := db.conn.Query(
		`SELECT id, user_id, title, message, link, category, read, created_at
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
		if err := rows.Scan(&n.ID, &n.UserID, &n.Title, &n.Message, &n.Link, &n.Category, &n.Read, &n.CreatedAt); err != nil {
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
