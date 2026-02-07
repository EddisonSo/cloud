package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

type IdentityPermission struct {
	ServiceAccountID string
	UserID           string
	Scopes           map[string][]string
	Version          int64
}

// UpsertIdentityPermissions inserts or updates identity permissions only if version > current.
func (db *DB) UpsertIdentityPermissions(saID, userID string, scopes map[string][]string, version int64) error {
	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return fmt.Errorf("marshal scopes: %w", err)
	}

	_, err = db.Exec(`
		INSERT INTO identity_permissions (service_account_id, user_id, scopes, version)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (service_account_id) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			scopes = EXCLUDED.scopes,
			version = EXCLUDED.version
		WHERE identity_permissions.version < EXCLUDED.version
	`, saID, userID, scopesJSON, version)
	if err != nil {
		return fmt.Errorf("upsert identity permissions: %w", err)
	}
	return nil
}

// DeleteIdentityPermissions removes identity permissions for a service account.
func (db *DB) DeleteIdentityPermissions(saID string) error {
	_, err := db.Exec(`DELETE FROM identity_permissions WHERE service_account_id = $1`, saID)
	if err != nil {
		return fmt.Errorf("delete identity permissions: %w", err)
	}
	return nil
}

// GetIdentityPermissions retrieves identity permissions for a service account.
func (db *DB) GetIdentityPermissions(saID string) (*IdentityPermission, error) {
	ip := &IdentityPermission{}
	var scopesJSON []byte
	err := db.QueryRow(`
		SELECT service_account_id, user_id, scopes, version
		FROM identity_permissions WHERE service_account_id = $1
	`, saID).Scan(&ip.ServiceAccountID, &ip.UserID, &scopesJSON, &ip.Version)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query identity permissions: %w", err)
	}
	if err := json.Unmarshal(scopesJSON, &ip.Scopes); err != nil {
		return nil, fmt.Errorf("unmarshal scopes: %w", err)
	}
	return ip, nil
}
