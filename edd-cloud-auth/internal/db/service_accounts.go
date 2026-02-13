package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lib/pq"
)

type ServiceAccount struct {
	ID        string              `json:"id"`
	UserID    string              `json:"user_id"`
	Name      string              `json:"name"`
	Scopes    map[string][]string `json:"scopes"`
	CreatedAt int64               `json:"created_at"`
	Version   int64               `json:"version"`
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

	db.invalidatePermissionsCache()

	return &ServiceAccount{
		ID:        id,
		UserID:    userID,
		Name:      name,
		Scopes:    scopes,
		CreatedAt: now,
		Version:   1,
	}, nil
}

func (db *DB) ListServiceAccountsByUser(userID string) ([]*ServiceAccount, error) {
	rows, err := db.Query(`
		SELECT id, user_id, name, scopes, created_at, COALESCE(version, 1)
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
		if err := rows.Scan(&sa.ID, &sa.UserID, &sa.Name, &scopesJSON, &sa.CreatedAt, &sa.Version); err != nil {
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
		SELECT id, user_id, name, scopes, created_at, COALESCE(version, 1)
		FROM service_accounts WHERE id = $1
	`, id).Scan(&sa.ID, &sa.UserID, &sa.Name, &scopesJSON, &sa.CreatedAt, &sa.Version)
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

func (db *DB) UpdateServiceAccountScopes(id, userID string, scopes map[string][]string) (int64, error) {
	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return 0, fmt.Errorf("marshal scopes: %w", err)
	}

	var newVersion int64
	err = db.QueryRow(`
		UPDATE service_accounts SET scopes = $1, version = COALESCE(version, 1) + 1
		WHERE id = $2 AND user_id = $3
		RETURNING version
	`, scopesJSON, id, userID).Scan(&newVersion)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("service account not found")
	}
	if err != nil {
		return 0, fmt.Errorf("update service account scopes: %w", err)
	}

	db.invalidatePermissionsCache()

	return newVersion, nil
}

func (db *DB) DeleteServiceAccount(id, userID string) (int64, error) {
	var version int64
	err := db.QueryRow(`
		DELETE FROM service_accounts WHERE id = $1 AND user_id = $2
		RETURNING COALESCE(version, 1)
	`, id, userID).Scan(&version)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("service account not found")
	}
	if err != nil {
		return 0, fmt.Errorf("delete service account: %w", err)
	}

	db.invalidatePermissionsCache()

	return version, nil
}

func (db *DB) ListAllServiceAccountPermissions() ([]*ServiceAccount, error) {
	db.permMu.RLock()
	if db.permissionsCacheValid {
		cached := db.permissionsCache
		db.permMu.RUnlock()
		return cached, nil
	}
	db.permMu.RUnlock()

	rows, err := db.Query(`
		SELECT id, user_id, name, scopes, created_at, COALESCE(version, 1)
		FROM service_accounts ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query all service accounts: %w", err)
	}
	defer rows.Close()

	var accounts []*ServiceAccount
	for rows.Next() {
		sa := &ServiceAccount{}
		var scopesJSON []byte
		if err := rows.Scan(&sa.ID, &sa.UserID, &sa.Name, &scopesJSON, &sa.CreatedAt, &sa.Version); err != nil {
			return nil, fmt.Errorf("scan service account: %w", err)
		}
		if err := json.Unmarshal(scopesJSON, &sa.Scopes); err != nil {
			return nil, fmt.Errorf("unmarshal scopes: %w", err)
		}
		accounts = append(accounts, sa)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	db.permMu.Lock()
	db.permissionsCache = accounts
	db.permissionsCacheValid = true
	db.permMu.Unlock()

	return accounts, nil
}

func (db *DB) CountTokensByServiceAccounts(saIDs []string) (map[string]int, error) {
	if len(saIDs) == 0 {
		return map[string]int{}, nil
	}

	rows, err := db.Query(`
		SELECT service_account_id, COUNT(*)
		FROM api_tokens
		WHERE service_account_id = ANY($1)
		GROUP BY service_account_id
	`, pq.Array(saIDs))
	if err != nil {
		return nil, fmt.Errorf("count tokens by service accounts: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int, len(saIDs))
	for rows.Next() {
		var id string
		var count int
		if err := rows.Scan(&id, &count); err != nil {
			return nil, fmt.Errorf("scan token count: %w", err)
		}
		counts[id] = count
	}
	return counts, rows.Err()
}

func (db *DB) CountServiceAccountTokens(saID string) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM api_tokens WHERE service_account_id = $1`, saID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count service account tokens: %w", err)
	}
	return count, nil
}
