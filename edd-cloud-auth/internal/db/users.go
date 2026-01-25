package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type User struct {
	UserID       string
	Username     string
	PasswordHash string
	DisplayName  string
	CreatedAt    time.Time
}

func (db *DB) GetUserByUsername(username string) (*User, error) {
	u := &User{}
	err := db.QueryRow(`
		SELECT user_id, username, password_hash, COALESCE(display_name, username), COALESCE(created_at, NOW())
		FROM users WHERE username = $1
	`, username).Scan(&u.UserID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query user: %w", err)
	}
	return u, nil
}

func (db *DB) GetUserByID(userID string) (*User, error) {
	u := &User{}
	err := db.QueryRow(`
		SELECT user_id, username, password_hash, COALESCE(display_name, username), COALESCE(created_at, NOW())
		FROM users WHERE user_id = $1
	`, userID).Scan(&u.UserID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query user: %w", err)
	}
	return u, nil
}

func (db *DB) ListUsers() ([]*User, error) {
	rows, err := db.Query(`
		SELECT user_id, username, COALESCE(display_name, username), COALESCE(created_at, NOW())
		FROM users ORDER BY created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("query users: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		u := &User{}
		if err := rows.Scan(&u.UserID, &u.Username, &u.DisplayName, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	return users, nil
}

func (db *DB) CreateUser(username, passwordHash, displayName string) (*User, error) {
	if displayName == "" {
		displayName = username
	}

	// Generate user_id with collision check
	var userID string
	var err error

	for i := 0; i < 10; i++ {
		userID, err = GenerateNanoID()
		if err != nil {
			return nil, fmt.Errorf("generate user_id: %w", err)
		}

		_, err = db.Exec(`
			INSERT INTO users (user_id, username, password_hash, display_name)
			VALUES ($1, $2, $3, $4)
		`, userID, username, passwordHash, displayName)

		if err == nil {
			break
		}
		if strings.Contains(err.Error(), "username") {
			return nil, fmt.Errorf("username already exists")
		}
		// If user_id collision, retry
		if !strings.Contains(err.Error(), "user_id") {
			return nil, fmt.Errorf("create user: %w", err)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to generate unique user_id: %w", err)
	}

	return &User{
		UserID:      userID,
		Username:    username,
		DisplayName: displayName,
		CreatedAt:   time.Now(),
	}, nil
}

func (db *DB) DeleteUser(userID string) error {
	// Sessions are deleted via ON DELETE CASCADE
	result, err := db.Exec(`DELETE FROM users WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

func (db *DB) UpdateUser(userID string, displayName string) error {
	_, err := db.Exec(`UPDATE users SET display_name = $1 WHERE user_id = $2`, displayName, userID)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	return nil
}
