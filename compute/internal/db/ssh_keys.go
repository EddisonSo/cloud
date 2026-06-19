package db

import (
	"database/sql"
	"fmt"
	"time"
)

type SSHKey struct {
	ID          int64
	UserID      string
	Name        string
	PublicKey   string
	Fingerprint string
	CreatedAt   time.Time
}

func (db *DB) CreateSSHKey(key *SSHKey) error {
	err := db.QueryRow(`
		INSERT INTO ssh_keys (user_id, name, public_key, fingerprint)
		VALUES ($1, $2, $3, $4)
		RETURNING id`,
		key.UserID, key.Name, key.PublicKey, key.Fingerprint,
	).Scan(&key.ID)
	if err != nil {
		return fmt.Errorf("insert ssh key: %w", err)
	}
	return nil
}

func (db *DB) GetSSHKey(id int64) (*SSHKey, error) {
	key := &SSHKey{}
	err := db.QueryRow(`
		SELECT id, user_id, name, public_key, fingerprint, created_at
		FROM ssh_keys WHERE id = $1`, id,
	).Scan(&key.ID, &key.UserID, &key.Name, &key.PublicKey, &key.Fingerprint, &key.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query ssh key: %w", err)
	}
	return key, nil
}

func (db *DB) ListSSHKeysByUser(userID string) ([]*SSHKey, error) {
	rows, err := db.Query(`
		SELECT id, user_id, name, public_key, fingerprint, created_at
		FROM ssh_keys WHERE user_id = $1 ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("query ssh keys: %w", err)
	}
	defer rows.Close()

	var keys []*SSHKey
	for rows.Next() {
		key := &SSHKey{}
		if err := rows.Scan(&key.ID, &key.UserID, &key.Name, &key.PublicKey, &key.Fingerprint, &key.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan ssh key: %w", err)
		}
		keys = append(keys, key)
	}
	return keys, nil
}

func (db *DB) GetSSHKeysByIDs(userID string, ids []int64) ([]*SSHKey, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	// Build query with PostgreSQL placeholders ($1, $2, etc.)
	query := `SELECT id, user_id, name, public_key, fingerprint, created_at
		FROM ssh_keys WHERE user_id = $1 AND id IN (`
	args := []any{userID}
	for i, id := range ids {
		if i > 0 {
			query += ","
		}
		query += fmt.Sprintf("$%d", i+2)
		args = append(args, id)
	}
	query += ")"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query ssh keys by ids: %w", err)
	}
	defer rows.Close()

	var keys []*SSHKey
	for rows.Next() {
		key := &SSHKey{}
		if err := rows.Scan(&key.ID, &key.UserID, &key.Name, &key.PublicKey, &key.Fingerprint, &key.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan ssh key: %w", err)
		}
		keys = append(keys, key)
	}
	return keys, nil
}

func (db *DB) DeleteSSHKey(id int64, userID string) error {
	result, err := db.Exec(`DELETE FROM ssh_keys WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete ssh key: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("ssh key not found or not owned by user")
	}
	return nil
}

func (db *DB) CountSSHKeysByUser(userID string) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM ssh_keys WHERE user_id = $1`, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count ssh keys: %w", err)
	}
	return count, nil
}
