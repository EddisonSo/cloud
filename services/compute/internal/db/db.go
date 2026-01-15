package db

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

type DB struct {
	*sql.DB
}

// Open connects to PostgreSQL using a connection string
// Format: postgres://user:password@host:port/dbname?sslmode=disable
func Open(connStr string) (*DB, error) {
	sqlDB, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Test connection
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
		`CREATE TABLE IF NOT EXISTS containers (
			id TEXT PRIMARY KEY,
			user_id BIGINT NOT NULL,
			name TEXT NOT NULL,
			namespace TEXT NOT NULL,
			status TEXT DEFAULT 'pending',
			external_ip TEXT,
			memory_mb INTEGER DEFAULT 512,
			storage_gb INTEGER DEFAULT 5,
			image TEXT DEFAULT 'eddisonso/ecloud-compute-base:latest',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			stopped_at TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS ssh_keys (
			id SERIAL PRIMARY KEY,
			user_id BIGINT NOT NULL,
			name TEXT NOT NULL,
			public_key TEXT NOT NULL,
			fingerprint TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_containers_user_id ON containers(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_ssh_keys_user_id ON ssh_keys(user_id)`,
	}

	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return fmt.Errorf("execute migration: %w", err)
		}
	}

	return nil
}
