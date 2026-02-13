package db

import (
	"fmt"
	"time"
)

type WebAuthnCredential struct {
	ID              []byte
	UserID          string
	Name            string
	PublicKey       []byte
	AttestationType string
	AAGUID          []byte
	SignCount       uint32
	CreatedAt       int64
}

func (db *DB) CreateCredential(cred *WebAuthnCredential) error {
	_, err := db.Exec(`
		INSERT INTO webauthn_credentials (id, user_id, name, public_key, attestation_type, aaguid, sign_count, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, cred.ID, cred.UserID, cred.Name, cred.PublicKey, cred.AttestationType, cred.AAGUID, cred.SignCount, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("create credential: %w", err)
	}
	return nil
}

func (db *DB) GetCredentialsByUserID(userID string) ([]WebAuthnCredential, error) {
	rows, err := db.Query(`
		SELECT id, user_id, name, public_key, attestation_type, COALESCE(aaguid, ''), sign_count, created_at
		FROM webauthn_credentials WHERE user_id = $1
		ORDER BY created_at
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("query credentials: %w", err)
	}
	defer rows.Close()

	var creds []WebAuthnCredential
	for rows.Next() {
		var c WebAuthnCredential
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.PublicKey, &c.AttestationType, &c.AAGUID, &c.SignCount, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan credential: %w", err)
		}
		creds = append(creds, c)
	}
	return creds, nil
}

func (db *DB) CountCredentialsByUserID(userID string) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM webauthn_credentials WHERE user_id = $1`, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count credentials: %w", err)
	}
	return count, nil
}

func (db *DB) DeleteCredential(credID []byte, userID string) error {
	result, err := db.Exec(`DELETE FROM webauthn_credentials WHERE id = $1 AND user_id = $2`, credID, userID)
	if err != nil {
		return fmt.Errorf("delete credential: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("credential not found")
	}
	return nil
}

func (db *DB) UpdateCredentialName(credID []byte, userID, name string) error {
	result, err := db.Exec(`UPDATE webauthn_credentials SET name = $1 WHERE id = $2 AND user_id = $3`, name, credID, userID)
	if err != nil {
		return fmt.Errorf("update credential name: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("credential not found")
	}
	return nil
}

func (db *DB) UpdateCredentialSignCount(credID []byte, signCount uint32) error {
	_, err := db.Exec(`UPDATE webauthn_credentials SET sign_count = $1 WHERE id = $2`, signCount, credID)
	if err != nil {
		return fmt.Errorf("update sign count: %w", err)
	}
	return nil
}
