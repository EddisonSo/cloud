package db

import (
	"database/sql"
	"fmt"
	"strings"
)

// Repository represents a container image repository record.
// This table is created by the registry service; it may not exist yet.
type Repository struct {
	ID         string
	OwnerID    string
	Name       string
	Visibility int // 0 = private, 1 = public
}

// GetRepositoryByName fetches a repository by its name.
// Returns (nil, nil) if the repository does not exist.
// Returns (nil, sql.ErrNoRows) if the repositories table doesn't exist yet
// (it is created by the registry service, not the auth service).
func (db *DB) GetRepositoryByName(name string) (*Repository, error) {
	r := &Repository{}
	err := db.QueryRow(`
		SELECT id, owner_id, name, visibility
		FROM repositories WHERE name = $1
	`, name).Scan(&r.ID, &r.OwnerID, &r.Name, &r.Visibility)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		// pq error code 42P01 = undefined_table (table not created yet)
		msg := err.Error()
		if strings.Contains(msg, "42P01") || strings.Contains(msg, "does not exist") {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("query repository: %w", err)
	}
	return r, nil
}
