package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type APIToken struct {
	ID               string              `json:"id"`
	UserID           string              `json:"user_id"`
	Name             string              `json:"name"`
	TokenHash        string              `json:"-"`
	Scopes           map[string][]string `json:"scopes"`
	ExpiresAt        int64               `json:"expires_at"`
	LastUsedAt       int64               `json:"last_used_at"`
	CreatedAt        int64               `json:"created_at"`
	ServiceAccountID *string             `json:"service_account_id,omitempty"`
}

func (db *DB) CreateAPIToken(userID, name, tokenHash string, scopes map[string][]string, expiresAt int64) (*APIToken, error) {
	id, err := GenerateNanoID()
	if err != nil {
		return nil, fmt.Errorf("generate token id: %w", err)
	}

	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return nil, fmt.Errorf("marshal scopes: %w", err)
	}

	now := time.Now().Unix()
	_, err = db.Exec(`
		INSERT INTO api_tokens (id, user_id, name, token_hash, scopes, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, id, userID, name, tokenHash, scopesJSON, expiresAt, now)
	if err != nil {
		return nil, fmt.Errorf("insert api token: %w", err)
	}

	return &APIToken{
		ID:        id,
		UserID:    userID,
		Name:      name,
		Scopes:    scopes,
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}, nil
}

func (db *DB) ListAPITokensByUser(userID string) ([]*APIToken, error) {
	rows, err := db.Query(`
		SELECT id, user_id, name, scopes, expires_at, last_used_at, created_at, service_account_id
		FROM api_tokens WHERE user_id = $1 ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("query api tokens: %w", err)
	}
	defer rows.Close()

	var tokens []*APIToken
	for rows.Next() {
		t := &APIToken{}
		var scopesJSON []byte
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &scopesJSON, &t.ExpiresAt, &t.LastUsedAt, &t.CreatedAt, &t.ServiceAccountID); err != nil {
			return nil, fmt.Errorf("scan api token: %w", err)
		}
		if scopesJSON != nil {
			if err := json.Unmarshal(scopesJSON, &t.Scopes); err != nil {
				return nil, fmt.Errorf("unmarshal scopes: %w", err)
			}
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

func (db *DB) DeleteAPIToken(id, userID string) error {
	result, err := db.Exec(`DELETE FROM api_tokens WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete api token: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("token not found")
	}
	return nil
}

func (db *DB) GetAPITokenByID(id string) (*APIToken, error) {
	t := &APIToken{}
	var scopesJSON []byte
	err := db.QueryRow(`
		SELECT id, user_id, name, scopes, expires_at, last_used_at, created_at, service_account_id
		FROM api_tokens WHERE id = $1
	`, id).Scan(&t.ID, &t.UserID, &t.Name, &scopesJSON, &t.ExpiresAt, &t.LastUsedAt, &t.CreatedAt, &t.ServiceAccountID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query api token: %w", err)
	}
	if scopesJSON != nil {
		if err := json.Unmarshal(scopesJSON, &t.Scopes); err != nil {
			return nil, fmt.Errorf("unmarshal scopes: %w", err)
		}
	}
	return t, nil
}

func (db *DB) CreateServiceAccountToken(userID, saID, name, tokenHash string, expiresAt int64) (*APIToken, error) {
	id, err := GenerateNanoID()
	if err != nil {
		return nil, fmt.Errorf("generate token id: %w", err)
	}

	now := time.Now().Unix()
	_, err = db.Exec(`
		INSERT INTO api_tokens (id, user_id, name, token_hash, scopes, expires_at, created_at, service_account_id)
		VALUES ($1, $2, $3, $4, NULL, $5, $6, $7)
	`, id, userID, name, tokenHash, expiresAt, now, saID)
	if err != nil {
		return nil, fmt.Errorf("insert service account token: %w", err)
	}

	return &APIToken{
		ID:               id,
		UserID:           userID,
		Name:             name,
		ExpiresAt:        expiresAt,
		CreatedAt:        now,
		ServiceAccountID: &saID,
	}, nil
}

func (db *DB) ListTokensByServiceAccount(saID, userID string) ([]*APIToken, error) {
	rows, err := db.Query(`
		SELECT id, user_id, name, expires_at, last_used_at, created_at, service_account_id
		FROM api_tokens WHERE service_account_id = $1 AND user_id = $2 ORDER BY created_at DESC
	`, saID, userID)
	if err != nil {
		return nil, fmt.Errorf("query service account tokens: %w", err)
	}
	defer rows.Close()

	var tokens []*APIToken
	for rows.Next() {
		t := &APIToken{}
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.ExpiresAt, &t.LastUsedAt, &t.CreatedAt, &t.ServiceAccountID); err != nil {
			return nil, fmt.Errorf("scan service account token: %w", err)
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

func (db *DB) UpdateAPITokenLastUsed(id string, ts int64) error {
	_, err := db.Exec(`UPDATE api_tokens SET last_used_at = $1 WHERE id = $2`, ts, id)
	return err
}
