package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	DisplayName  string
	PublicID     string
	CreatedAt    time.Time
}

func (db *DB) GetUserByUsername(username string) (*User, error) {
	u := &User{}
	err := db.QueryRow(`
		SELECT id, username, password_hash, COALESCE(display_name, username), COALESCE(public_id, ''), COALESCE(created_at, NOW())
		FROM users WHERE username = $1
	`, username).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.PublicID, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query user: %w", err)
	}
	return u, nil
}

func (db *DB) GetUserByID(id int64) (*User, error) {
	u := &User{}
	err := db.QueryRow(`
		SELECT id, username, password_hash, COALESCE(display_name, username), COALESCE(public_id, ''), COALESCE(created_at, NOW())
		FROM users WHERE id = $1
	`, id).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.PublicID, &u.CreatedAt)
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
		SELECT id, username, COALESCE(display_name, username), COALESCE(public_id, ''), COALESCE(created_at, NOW())
		FROM users ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("query users: %w", err)
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		u := &User{}
		if err := rows.Scan(&u.ID, &u.Username, &u.DisplayName, &u.PublicID, &u.CreatedAt); err != nil {
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

	// Generate public_id with collision check
	var id int64
	var publicID string
	var err error

	for i := 0; i < 10; i++ {
		publicID, err = GenerateNanoID()
		if err != nil {
			return nil, fmt.Errorf("generate public_id: %w", err)
		}

		err = db.QueryRow(`
			INSERT INTO users (username, password_hash, display_name, public_id)
			VALUES ($1, $2, $3, $4)
			RETURNING id
		`, username, passwordHash, displayName, publicID).Scan(&id)

		if err == nil {
			break
		}
		if strings.Contains(err.Error(), "username") {
			return nil, fmt.Errorf("username already exists")
		}
		// If public_id collision, retry
		if !strings.Contains(err.Error(), "public_id") {
			return nil, fmt.Errorf("create user: %w", err)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to generate unique public_id: %w", err)
	}

	return &User{
		ID:          id,
		Username:    username,
		DisplayName: displayName,
		PublicID:    publicID,
		CreatedAt:   time.Now(),
	}, nil
}

func (db *DB) DeleteUser(id int64) error {
	// Sessions are deleted via ON DELETE CASCADE
	result, err := db.Exec(`DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

func (db *DB) UpdateUser(id int64, displayName string) error {
	_, err := db.Exec(`UPDATE users SET display_name = $1 WHERE id = $2`, displayName, id)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	return nil
}
