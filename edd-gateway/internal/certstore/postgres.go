// Package certstore provides a certmagic.Storage implementation backed by
// PostgreSQL. It supplies both certificate storage and a distributed lock so
// multiple gateway replicas share a single certificate cache and never
// double-issue Let's Encrypt certificates.
package certstore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"sync"
	"time"

	"github.com/caddyserver/certmagic"
	_ "github.com/lib/pq"
)

// Compile-time assertion that PostgresStorage satisfies certmagic.Storage.
var _ certmagic.Storage = (*PostgresStorage)(nil)

// lockTTL is how long a held lock remains valid before it may be stolen by
// another replica (covering the case where the holder crashed without
// unlocking).
const lockTTL = 2 * time.Minute

// lockRetryInterval is how long to wait between attempts to acquire a
// contended lock.
const lockRetryInterval = 2 * time.Second

// PostgresStorage is a certmagic.Storage backed by PostgreSQL.
type PostgresStorage struct {
	db *sql.DB

	// instanceID identifies this process; it's folded into per-call lock
	// tokens for easier debugging of which replica holds a lock.
	instanceID string

	// tokens maps a lock name to the owner token this process used to acquire
	// it, so Unlock can verify ownership before deleting.
	mu     sync.Mutex
	tokens map[string]string
}

// New opens a connection to the given DSN, pings it, and creates the required
// tables if they do not already exist.
func New(dsn string) (*PostgresStorage, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS certmagic_data (
			key      TEXT PRIMARY KEY,
			value    BYTEA NOT NULL,
			modified TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create certmagic_data table: %w", err)
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS certmagic_locks (
			name    TEXT PRIMARY KEY,
			owner   TEXT NOT NULL,
			expires TIMESTAMPTZ NOT NULL
		)
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create certmagic_locks table: %w", err)
	}

	instanceID, err := randomHex()
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("generate instance id: %w", err)
	}

	return &PostgresStorage{
		db:         db,
		instanceID: instanceID,
		tokens:     make(map[string]string),
	}, nil
}

// Close closes the underlying database connection.
func (s *PostgresStorage) Close() error {
	return s.db.Close()
}

// randomHex returns 16 bytes of crypto-random data hex-encoded.
func randomHex() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Store upserts value at key.
func (s *PostgresStorage) Store(ctx context.Context, key string, value []byte) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO certmagic_data (key, value, modified)
		VALUES ($1, $2, now())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, modified = now()
	`, key, value)
	if err != nil {
		return fmt.Errorf("store %q: %w", key, err)
	}
	return nil
}

// Load retrieves the value at key, returning fs.ErrNotExist if absent.
func (s *PostgresStorage) Load(ctx context.Context, key string) ([]byte, error) {
	var value []byte
	err := s.db.QueryRowContext(ctx, `SELECT value FROM certmagic_data WHERE key = $1`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fs.ErrNotExist
	}
	if err != nil {
		return nil, fmt.Errorf("load %q: %w", key, err)
	}
	return value, nil
}

// Delete removes key, returning fs.ErrNotExist if it did not exist.
func (s *PostgresStorage) Delete(ctx context.Context, key string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM certmagic_data WHERE key = $1`, key)
	if err != nil {
		return fmt.Errorf("delete %q: %w", key, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete %q rows affected: %w", key, err)
	}
	if n == 0 {
		return fs.ErrNotExist
	}
	return nil
}

// Exists reports whether key exists.
func (s *PostgresStorage) Exists(ctx context.Context, key string) bool {
	var one int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM certmagic_data WHERE key = $1`, key).Scan(&one)
	return err == nil
}

// List returns all keys under prefix. When recursive is false, keys are
// collapsed to their direct child component under prefix and de-duplicated.
func (s *PostgresStorage) List(ctx context.Context, prefix string, recursive bool) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key FROM certmagic_data WHERE key LIKE $1`, prefix+"%")
	if err != nil {
		return nil, fmt.Errorf("list %q: %w", prefix, err)
	}
	defer rows.Close()

	if recursive {
		var keys []string
		for rows.Next() {
			var k string
			if err := rows.Scan(&k); err != nil {
				return nil, fmt.Errorf("scan key: %w", err)
			}
			keys = append(keys, k)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterate keys: %w", err)
		}
		return keys, nil
	}

	// Non-recursive: collapse each matching key to its direct child under
	// prefix (trim anything past the next '/') and de-duplicate.
	seen := make(map[string]struct{})
	var keys []string
	trimPrefix := strings.TrimSuffix(prefix, "/")
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, fmt.Errorf("scan key: %w", err)
		}
		rest := k
		if trimPrefix != "" {
			rest = strings.TrimPrefix(k, trimPrefix+"/")
			if rest == k {
				// Key isn't actually under prefix/ (e.g. prefix matched a
				// partial component via LIKE); keep the key as-is.
				if _, ok := seen[k]; !ok {
					seen[k] = struct{}{}
					keys = append(keys, k)
				}
				continue
			}
		}
		// Direct child = prefix + first component of rest.
		child := rest
		if i := strings.IndexByte(rest, '/'); i >= 0 {
			child = rest[:i]
		}
		var full string
		if trimPrefix != "" {
			full = trimPrefix + "/" + child
		} else {
			full = child
		}
		if _, ok := seen[full]; !ok {
			seen[full] = struct{}{}
			keys = append(keys, full)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate keys: %w", err)
	}
	return keys, nil
}

// Stat returns information about key, returning fs.ErrNotExist if absent.
func (s *PostgresStorage) Stat(ctx context.Context, key string) (certmagic.KeyInfo, error) {
	var (
		modified time.Time
		size     int64
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT modified, octet_length(value) FROM certmagic_data WHERE key = $1`, key).
		Scan(&modified, &size)
	if errors.Is(err, sql.ErrNoRows) {
		return certmagic.KeyInfo{}, fs.ErrNotExist
	}
	if err != nil {
		return certmagic.KeyInfo{}, fmt.Errorf("stat %q: %w", key, err)
	}
	return certmagic.KeyInfo{
		Key:        key,
		Modified:   modified,
		Size:       size,
		IsTerminal: true,
	}, nil
}

// Lock acquires a distributed lock on name, blocking until it is acquired or
// ctx is cancelled. Stale locks (held past lockTTL by a crashed holder) are
// stolen. An owner token is used so a replica only believes it holds the lock
// when it is the recorded owner, closing the race where two replicas both read
// back a fresh expiry.
func (s *PostgresStorage) Lock(ctx context.Context, name string) error {
	token, err := randomHex()
	if err != nil {
		return fmt.Errorf("generate lock token: %w", err)
	}
	token = s.instanceID + ":" + token

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Try to claim: insert if absent, or steal only if the existing lock
		// has expired.
		if _, err := s.db.ExecContext(ctx, `
			INSERT INTO certmagic_locks (name, owner, expires)
			VALUES ($1, $2, now() + $3::interval)
			ON CONFLICT (name) DO UPDATE
				SET owner = EXCLUDED.owner, expires = EXCLUDED.expires
				WHERE certmagic_locks.expires < now()
		`, name, token, fmt.Sprintf("%d milliseconds", lockTTL.Milliseconds())); err != nil {
			return fmt.Errorf("claim lock %q: %w", name, err)
		}

		// Read back the recorded owner. Only if it's our token do we actually
		// hold the lock.
		var owner string
		err := s.db.QueryRowContext(ctx, `SELECT owner FROM certmagic_locks WHERE name = $1`, name).Scan(&owner)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("read lock owner %q: %w", name, err)
		}
		if owner == token {
			s.mu.Lock()
			s.tokens[name] = token
			s.mu.Unlock()
			return nil
		}

		// Someone else holds a live lock; wait and retry.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(lockRetryInterval):
		}
	}
}

// Unlock releases the lock on name, but only if this process still owns it
// (verified via the stored owner token).
func (s *PostgresStorage) Unlock(ctx context.Context, name string) error {
	s.mu.Lock()
	token, ok := s.tokens[name]
	delete(s.tokens, name)
	s.mu.Unlock()

	if !ok {
		// Shouldn't happen in certmagic's usage; fall back to unconditional
		// delete so we don't leave a dangling lock.
		if _, err := s.db.ExecContext(ctx, `DELETE FROM certmagic_locks WHERE name = $1`, name); err != nil {
			return fmt.Errorf("unlock %q: %w", name, err)
		}
		return nil
	}

	if _, err := s.db.ExecContext(ctx, `DELETE FROM certmagic_locks WHERE name = $1 AND owner = $2`, name, token); err != nil {
		return fmt.Errorf("unlock %q: %w", name, err)
	}
	return nil
}
