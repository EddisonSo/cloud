package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type ServiceAccount struct {
	ID        string              `json:"id"`
	UserID    string              `json:"user_id"`
	Name      string              `json:"name"`
	Scopes    map[string][]string `json:"scopes"`
	CreatedAt int64               `json:"created_at"`
}

func (db *DB) CreateServiceAccount(userID, name string, scopes map[string][]string) (*ServiceAccount, error) {
	id, err := GenerateNanoID()
	if err != nil {
		return nil, fmt.Errorf("generate service account id: %w", err)
	}

	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return nil, fmt.Errorf("marshal scopes: %w", err)
	}

	now := time.Now().Unix()
	_, err = db.Exec(`
		INSERT INTO service_accounts (id, user_id, name, scopes, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, id, userID, name, scopesJSON, now)
	if err != nil {
		return nil, fmt.Errorf("insert service account: %w", err)
	}

	return &ServiceAccount{
		ID:        id,
		UserID:    userID,
		Name:      name,
		Scopes:    scopes,
		CreatedAt: now,
	}, nil
}

func (db *DB) ListServiceAccountsByUser(userID string) ([]*ServiceAccount, error) {
	rows, err := db.Query(`
		SELECT id, user_id, name, scopes, created_at
		FROM service_accounts WHERE user_id = $1 ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("query service accounts: %w", err)
	}
	defer rows.Close()

	var accounts []*ServiceAccount
	for rows.Next() {
		sa := &ServiceAccount{}
		var scopesJSON []byte
		if err := rows.Scan(&sa.ID, &sa.UserID, &sa.Name, &scopesJSON, &sa.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan service account: %w", err)
		}
		if err := json.Unmarshal(scopesJSON, &sa.Scopes); err != nil {
			return nil, fmt.Errorf("unmarshal scopes: %w", err)
		}
		accounts = append(accounts, sa)
	}
	return accounts, rows.Err()
}

func (db *DB) GetServiceAccountByID(id string) (*ServiceAccount, error) {
	sa := &ServiceAccount{}
	var scopesJSON []byte
	err := db.QueryRow(`
		SELECT id, user_id, name, scopes, created_at
		FROM service_accounts WHERE id = $1
	`, id).Scan(&sa.ID, &sa.UserID, &sa.Name, &scopesJSON, &sa.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query service account: %w", err)
	}
	if err := json.Unmarshal(scopesJSON, &sa.Scopes); err != nil {
		return nil, fmt.Errorf("unmarshal scopes: %w", err)
	}
	return sa, nil
}

func (db *DB) UpdateServiceAccountScopes(id, userID string, scopes map[string][]string) error {
	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return fmt.Errorf("marshal scopes: %w", err)
	}

	result, err := db.Exec(`
		UPDATE service_accounts SET scopes = $1
		WHERE id = $2 AND user_id = $3
	`, scopesJSON, id, userID)
	if err != nil {
		return fmt.Errorf("update service account scopes: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("service account not found")
	}
	return nil
}

func (db *DB) DeleteServiceAccount(id, userID string) error {
	result, err := db.Exec(`DELETE FROM service_accounts WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete service account: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("service account not found")
	}
	return nil
}

func (db *DB) CountServiceAccountTokens(saID string) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM api_tokens WHERE service_account_id = $1`, saID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count service account tokens: %w", err)
	}
	return count, nil
}
