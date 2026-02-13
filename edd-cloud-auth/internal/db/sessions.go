package db

import (
	"fmt"
	"time"
)

type Session struct {
	ID        int64
	UserID    string
	Token     string
	ExpiresAt int64
	CreatedAt int64
	IPAddress string
}

type SessionWithUser struct {
	Session
	Username    string
	DisplayName string
}

func (db *DB) CreateSession(userID string, token string, expiresAt time.Time, ipAddress string) (*Session, error) {
	now := time.Now().Unix()

	// Delete existing session for same user+IP
	_, _ = db.Exec(`DELETE FROM sessions WHERE user_id = $1 AND ip_address = $2`, userID, ipAddress)

	var id int64
	err := db.QueryRow(`
		INSERT INTO sessions (user_id, token, expires_at, created_at, ip_address)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id
	`, userID, token, expiresAt.Unix(), now, ipAddress).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	return &Session{
		ID:        id,
		UserID:    userID,
		Token:     token,
		ExpiresAt: expiresAt.Unix(),
		CreatedAt: now,
		IPAddress: ipAddress,
	}, nil
}

func (db *DB) GetSessionByToken(token string) (*Session, error) {
	s := &Session{}
	err := db.QueryRow(`
		SELECT id, user_id, token, expires_at, created_at, COALESCE(ip_address, '')
		FROM sessions WHERE token = $1
	`, token).Scan(&s.ID, &s.UserID, &s.Token, &s.ExpiresAt, &s.CreatedAt, &s.IPAddress)
	if err != nil {
		return nil, fmt.Errorf("query session: %w", err)
	}
	return s, nil
}

func (db *DB) DeleteSession(token string) error {
	_, err := db.Exec(`DELETE FROM sessions WHERE token = $1`, token)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (db *DB) DeleteUserSessions(userID string) error {
	_, err := db.Exec(`DELETE FROM sessions WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("delete user sessions: %w", err)
	}
	return nil
}

func (db *DB) ListActiveSessions() ([]*SessionWithUser, error) {
	now := time.Now().Unix()
	rows, err := db.Query(`
		SELECT DISTINCT ON (s.user_id, s.ip_address)
			s.id, s.user_id, s.token, s.expires_at, s.created_at, COALESCE(s.ip_address, ''),
			u.username, COALESCE(u.display_name, u.username)
		FROM sessions s
		JOIN users u ON s.user_id = u.user_id
		WHERE s.expires_at > $1
		ORDER BY s.user_id, s.ip_address, s.created_at DESC
	`, now)
	if err != nil {
		return nil, fmt.Errorf("query sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*SessionWithUser
	for rows.Next() {
		s := &SessionWithUser{}
		if err := rows.Scan(&s.ID, &s.UserID, &s.Token, &s.ExpiresAt, &s.CreatedAt, &s.IPAddress,
			&s.Username, &s.DisplayName); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

func (db *DB) ListSessionsByUserID(userID string) ([]*Session, error) {
	now := time.Now().Unix()
	rows, err := db.Query(`
		SELECT id, user_id, token, expires_at, created_at, COALESCE(ip_address, '')
		FROM sessions WHERE user_id = $1 AND expires_at > $2
		ORDER BY created_at DESC
	`, userID, now)
	if err != nil {
		return nil, fmt.Errorf("query user sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		s := &Session{}
		if err := rows.Scan(&s.ID, &s.UserID, &s.Token, &s.ExpiresAt, &s.CreatedAt, &s.IPAddress); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

func (db *DB) CleanupExpiredSessions() (int64, error) {
	now := time.Now().Unix()
	result, err := db.Exec(`DELETE FROM sessions WHERE expires_at <= $1`, now)
	if err != nil {
		return 0, fmt.Errorf("cleanup sessions: %w", err)
	}
	rows, _ := result.RowsAffected()
	return rows, nil
}
