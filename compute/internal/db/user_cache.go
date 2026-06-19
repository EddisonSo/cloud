package db

import "fmt"

// CachedUser represents a user in the local cache
type CachedUser struct {
	UserID      string
	Username    string
	DisplayName string
}

// UpsertUserCache inserts or updates a user in the cache
func (db *DB) UpsertUserCache(userID, username, displayName string) error {
	_, err := db.Exec(`
		INSERT INTO user_cache (user_id, username, display_name, synced_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT (user_id) DO UPDATE SET
			username = EXCLUDED.username,
			display_name = EXCLUDED.display_name,
			synced_at = CURRENT_TIMESTAMP
	`, userID, username, displayName)
	if err != nil {
		return fmt.Errorf("upsert user cache: %w", err)
	}
	return nil
}

// DeleteUserCache removes a user from the cache
func (db *DB) DeleteUserCache(userID string) error {
	_, err := db.Exec(`DELETE FROM user_cache WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("delete user cache: %w", err)
	}
	return nil
}

// GetCachedUser retrieves a user from the cache
func (db *DB) GetCachedUser(userID string) (*CachedUser, error) {
	u := &CachedUser{}
	err := db.QueryRow(`
		SELECT user_id, username, display_name
		FROM user_cache WHERE user_id = $1
	`, userID).Scan(&u.UserID, &u.Username, &u.DisplayName)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// ListAllCachedUsers returns all users in the cache
func (db *DB) ListAllCachedUsers() ([]*CachedUser, error) {
	rows, err := db.Query(`SELECT user_id, username, display_name FROM user_cache`)
	if err != nil {
		return nil, fmt.Errorf("query user cache: %w", err)
	}
	defer rows.Close()

	var users []*CachedUser
	for rows.Next() {
		u := &CachedUser{}
		if err := rows.Scan(&u.UserID, &u.Username, &u.DisplayName); err != nil {
			return nil, fmt.Errorf("scan user cache: %w", err)
		}
		users = append(users, u)
	}
	return users, nil
}

// DeleteUserData deletes all data associated with a user (containers, SSH keys)
func (db *DB) DeleteUserData(userID string) error {
	// Delete SSH keys for the user
	_, err := db.Exec(`DELETE FROM ssh_keys WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("delete user ssh keys: %w", err)
	}

	// Delete containers for the user (ingress_rules cascade automatically)
	_, err = db.Exec(`DELETE FROM containers WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("delete user containers: %w", err)
	}

	return nil
}
