package db

import (
	"crypto/rand"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

// nanoid alphabet (URL-safe)
const nanoIDAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// GenerateNanoID generates a random 6-character ID
func GenerateNanoID() (string, error) {
	bytes := make([]byte, 6)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	for i := range bytes {
		bytes[i] = nanoIDAlphabet[bytes[i]%byte(len(nanoIDAlphabet))]
	}
	return string(bytes), nil
}

type DB struct {
	*sql.DB
}

func Open(connStr string) (*DB, error) {
	sqlDB, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	db := &DB{sqlDB}
	if err := db.migrate(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

func (db *DB) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS users (
			user_id TEXT PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			display_name TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id SERIAL PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
			token TEXT NOT NULL UNIQUE,
			expires_at BIGINT NOT NULL,
			created_at BIGINT NOT NULL DEFAULT 0,
			ip_address TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_users_username ON users(username)`,
		`CREATE TABLE IF NOT EXISTS api_tokens (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			scopes JSONB NOT NULL,
			expires_at BIGINT NOT NULL DEFAULT 0,
			last_used_at BIGINT NOT NULL DEFAULT 0,
			created_at BIGINT NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_api_tokens_user_id ON api_tokens(user_id)`,
		`CREATE TABLE IF NOT EXISTS service_accounts (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			scopes JSONB NOT NULL,
			created_at BIGINT NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_service_accounts_user_id ON service_accounts(user_id)`,
		`ALTER TABLE api_tokens ADD COLUMN IF NOT EXISTS service_account_id TEXT REFERENCES service_accounts(id) ON DELETE CASCADE`,
		`ALTER TABLE api_tokens ALTER COLUMN scopes DROP NOT NULL`,
		`ALTER TABLE service_accounts ADD COLUMN IF NOT EXISTS version BIGINT NOT NULL DEFAULT 1`,
		`CREATE TABLE IF NOT EXISTS webauthn_credentials (
			id BYTEA PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
			name TEXT NOT NULL DEFAULT '',
			public_key BYTEA NOT NULL,
			attestation_type TEXT NOT NULL,
			aaguid BYTEA,
			sign_count INTEGER NOT NULL DEFAULT 0,
			created_at BIGINT NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_webauthn_creds_user ON webauthn_credentials(user_id)`,
	}

	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return fmt.Errorf("execute migration: %w", err)
		}
	}

	return nil
}

// InitDefaultUser creates the default admin user if it doesn't exist
func (db *DB) InitDefaultUser(username, passwordHash string) error {
	// Generate user_id for default user
	userID, err := GenerateNanoID()
	if err != nil {
		return fmt.Errorf("generate user_id: %w", err)
	}

	_, err = db.Exec(`
		INSERT INTO users (user_id, username, password_hash, display_name)
		VALUES ($1, $2, $3, $2)
		ON CONFLICT (username) DO NOTHING
	`, userID, username, passwordHash)
	if err != nil {
		return fmt.Errorf("insert default user: %w", err)
	}

	return nil
}
