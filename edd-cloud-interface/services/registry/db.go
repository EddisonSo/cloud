package main

import (
	"context"
	"database/sql"
	"time"
)

// gcOrphanedBlob holds information about a blob that was swept by GC.
type gcOrphanedBlob struct {
	RepositoryID int
	Digest       string
	Size         int64
}

// initDB creates all tables if they do not exist.
func initDB(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS repositories (
	id         SERIAL PRIMARY KEY,
	name       TEXT UNIQUE NOT NULL,
	owner_id   TEXT NOT NULL,
	visibility INT NOT NULL DEFAULT 0,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS manifests (
	id            SERIAL PRIMARY KEY,
	repository_id INT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
	digest        TEXT NOT NULL,
	media_type    TEXT NOT NULL,
	size          BIGINT NOT NULL,
	created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	UNIQUE(repository_id, digest)
);

CREATE TABLE IF NOT EXISTS tags (
	id              SERIAL PRIMARY KEY,
	repository_id   INT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
	name            TEXT NOT NULL,
	manifest_digest TEXT NOT NULL,
	updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	UNIQUE(repository_id, name)
);

CREATE TABLE IF NOT EXISTS repository_blobs (
	repository_id INT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
	digest        TEXT NOT NULL,
	size          BIGINT NOT NULL,
	gc_marked_at  TIMESTAMPTZ,
	created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	PRIMARY KEY(repository_id, digest)
);

CREATE TABLE IF NOT EXISTS manifest_blobs (
	manifest_id  INT NOT NULL REFERENCES manifests(id) ON DELETE CASCADE,
	blob_digest  TEXT NOT NULL,
	PRIMARY KEY(manifest_id, blob_digest)
);

CREATE TABLE IF NOT EXISTS upload_sessions (
	uuid          TEXT PRIMARY KEY,
	repository_id INT NOT NULL REFERENCES repositories(id) ON DELETE CASCADE,
	hash_state    BYTEA,
	bytes_received BIGINT NOT NULL DEFAULT 0,
	created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`
	_, err := db.Exec(schema)
	return err
}

// getOrCreateRepo returns the ID of the named repository, creating it if it
// does not exist.
func getOrCreateRepo(ctx context.Context, db *sql.DB, name, ownerID string) (int, error) {
	const q = `
INSERT INTO repositories (name, owner_id)
VALUES ($1, $2)
ON CONFLICT (name) DO UPDATE SET updated_at = NOW()
RETURNING id`
	var id int
	err := db.QueryRowContext(ctx, q, name, ownerID).Scan(&id)
	return id, err
}

// getRepoByName looks up a repository by name.
func getRepoByName(ctx context.Context, db *sql.DB, name string) (id int, ownerID string, visibility int, err error) {
	const q = `SELECT id, owner_id, visibility FROM repositories WHERE name = $1`
	err = db.QueryRowContext(ctx, q, name).Scan(&id, &ownerID, &visibility)
	return
}

// listRepos returns repository names visible to ownerID. When includePublic is
// true, repositories with visibility > 0 owned by other users are also included.
func listRepos(ctx context.Context, db *sql.DB, ownerID string, includePublic bool, limit, offset int) ([]string, error) {
	var rows *sql.Rows
	var err error
	if includePublic {
		const q = `SELECT name FROM repositories WHERE owner_id = $1 OR visibility > 0 ORDER BY name LIMIT $2 OFFSET $3`
		rows, err = db.QueryContext(ctx, q, ownerID, limit, offset)
	} else {
		const q = `SELECT name FROM repositories WHERE owner_id = $1 ORDER BY name LIMIT $2 OFFSET $3`
		rows, err = db.QueryContext(ctx, q, ownerID, limit, offset)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// insertRepoBlob records that a blob exists in a repository. Duplicate inserts
// are silently ignored.
func insertRepoBlob(ctx context.Context, db *sql.DB, repoID int, digest string, size int64) error {
	const q = `
INSERT INTO repository_blobs (repository_id, digest, size)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING`
	_, err := db.ExecContext(ctx, q, repoID, digest, size)
	return err
}

// blobExistsInRepo checks whether a blob with the given digest is recorded in
// the repository.
func blobExistsInRepo(ctx context.Context, db *sql.DB, repoID int, digest string) (bool, int64, error) {
	const q = `SELECT size FROM repository_blobs WHERE repository_id = $1 AND digest = $2`
	var size int64
	err := db.QueryRowContext(ctx, q, repoID, digest).Scan(&size)
	if err == sql.ErrNoRows {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}
	return true, size, nil
}

// clearBlobGCMark removes the gc_marked_at timestamp from a blob so it is no
// longer eligible for garbage collection.
func clearBlobGCMark(ctx context.Context, db *sql.DB, repoID int, digest string) error {
	const q = `UPDATE repository_blobs SET gc_marked_at = NULL WHERE repository_id = $1 AND digest = $2`
	_, err := db.ExecContext(ctx, q, repoID, digest)
	return err
}

// upsertManifest inserts or updates a manifest record, returning its ID.
func upsertManifest(ctx context.Context, db *sql.DB, repoID int, digest, mediaType string, size int64) (int, error) {
	const q = `
INSERT INTO manifests (repository_id, digest, media_type, size)
VALUES ($1, $2, $3, $4)
ON CONFLICT (repository_id, digest) DO UPDATE SET media_type = EXCLUDED.media_type, size = EXCLUDED.size
RETURNING id`
	var id int
	err := db.QueryRowContext(ctx, q, repoID, digest, mediaType, size).Scan(&id)
	return id, err
}

// getManifest retrieves a manifest by repository ID and digest.
func getManifest(ctx context.Context, db *sql.DB, repoID int, digest string) (id int, mediaType string, size int64, err error) {
	const q = `SELECT id, media_type, size FROM manifests WHERE repository_id = $1 AND digest = $2`
	err = db.QueryRowContext(ctx, q, repoID, digest).Scan(&id, &mediaType, &size)
	return
}

// insertManifestBlobs records the blob digests referenced by a manifest. Duplicate
// entries are silently ignored.
func insertManifestBlobs(ctx context.Context, db *sql.DB, manifestID int, blobDigests []string) error {
	for _, d := range blobDigests {
		const q = `INSERT INTO manifest_blobs (manifest_id, blob_digest) VALUES ($1, $2) ON CONFLICT DO NOTHING`
		if _, err := db.ExecContext(ctx, q, manifestID, d); err != nil {
			return err
		}
	}
	return nil
}

// upsertTag creates or updates a tag, returning the previous manifest digest
// (empty string if it did not previously exist).
func upsertTag(ctx context.Context, db *sql.DB, repoID int, tag, digest string) (string, error) {
	const q = `
INSERT INTO tags (repository_id, name, manifest_digest)
VALUES ($1, $2, $3)
ON CONFLICT (repository_id, name) DO UPDATE SET manifest_digest = EXCLUDED.manifest_digest, updated_at = NOW()
RETURNING (SELECT manifest_digest FROM tags WHERE repository_id = $1 AND name = $2)`

	// We need the old digest before the upsert; use a CTE for atomicity.
	const qCTE = `
WITH old AS (
	SELECT manifest_digest FROM tags WHERE repository_id = $1 AND name = $2
)
INSERT INTO tags (repository_id, name, manifest_digest)
VALUES ($1, $2, $3)
ON CONFLICT (repository_id, name) DO UPDATE SET manifest_digest = EXCLUDED.manifest_digest, updated_at = NOW()
RETURNING (SELECT manifest_digest FROM old)`

	var oldDigest sql.NullString
	err := db.QueryRowContext(ctx, qCTE, repoID, tag, digest).Scan(&oldDigest)
	if err != nil {
		return "", err
	}
	return oldDigest.String, nil
}

// getTagDigest returns the manifest digest for the given tag.
func getTagDigest(ctx context.Context, db *sql.DB, repoID int, tag string) (string, error) {
	const q = `SELECT manifest_digest FROM tags WHERE repository_id = $1 AND name = $2`
	var digest string
	err := db.QueryRowContext(ctx, q, repoID, tag).Scan(&digest)
	return digest, err
}

// listTags returns all tag names for a repository.
func listTags(ctx context.Context, db *sql.DB, repoID int) ([]string, error) {
	const q = `SELECT name FROM tags WHERE repository_id = $1 ORDER BY name`
	rows, err := db.QueryContext(ctx, q, repoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tags = append(tags, name)
	}
	return tags, rows.Err()
}

// manifestHasOtherTags reports whether any tag other than excludeTag points at
// the given manifest digest in the repository.
func manifestHasOtherTags(ctx context.Context, db *sql.DB, repoID int, digest, excludeTag string) (bool, error) {
	const q = `SELECT EXISTS(SELECT 1 FROM tags WHERE repository_id = $1 AND manifest_digest = $2 AND name != $3)`
	var exists bool
	err := db.QueryRowContext(ctx, q, repoID, digest, excludeTag).Scan(&exists)
	return exists, err
}

// createUploadSession inserts a new upload session record.
func createUploadSession(ctx context.Context, db *sql.DB, uuid string, repoID int) error {
	const q = `INSERT INTO upload_sessions (uuid, repository_id) VALUES ($1, $2)`
	_, err := db.ExecContext(ctx, q, uuid, repoID)
	return err
}

// getUploadSession retrieves an upload session by UUID.
func getUploadSession(ctx context.Context, db *sql.DB, uuid string) (repoID int, hashState []byte, bytesReceived int64, err error) {
	const q = `SELECT repository_id, hash_state, bytes_received FROM upload_sessions WHERE uuid = $1`
	err = db.QueryRowContext(ctx, q, uuid).Scan(&repoID, &hashState, &bytesReceived)
	return
}

// updateUploadSession updates the hash state and byte count for an in-progress upload.
func updateUploadSession(ctx context.Context, db *sql.DB, uuid string, hashState []byte, bytesReceived int64) error {
	const q = `UPDATE upload_sessions SET hash_state = $2, bytes_received = $3 WHERE uuid = $1`
	_, err := db.ExecContext(ctx, q, uuid, hashState, bytesReceived)
	return err
}

// deleteUploadSession removes an upload session.
func deleteUploadSession(ctx context.Context, db *sql.DB, uuid string) error {
	const q = `DELETE FROM upload_sessions WHERE uuid = $1`
	_, err := db.ExecContext(ctx, q, uuid)
	return err
}

// deleteStaleUploadSessions deletes upload sessions older than olderThan and
// returns the number deleted.
func deleteStaleUploadSessions(ctx context.Context, db *sql.DB, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	const q = `DELETE FROM upload_sessions WHERE created_at < $1`
	res, err := db.ExecContext(ctx, q, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// markOrphanedBlobs sets gc_marked_at on repository_blobs that are not
// referenced by any manifest_blobs row. Returns the number of rows updated.
func markOrphanedBlobs(ctx context.Context, db *sql.DB) (int64, error) {
	const q = `
UPDATE repository_blobs rb
SET gc_marked_at = NOW()
WHERE gc_marked_at IS NULL
  AND NOT EXISTS (
    SELECT 1
    FROM manifest_blobs mb
    JOIN manifests m ON mb.manifest_id = m.id
    WHERE m.repository_id = rb.repository_id
      AND mb.blob_digest   = rb.digest
  )`
	res, err := db.ExecContext(ctx, q)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// sweepMarkedBlobs deletes repository_blobs that were marked for GC before
// markedBefore and returns information about each deleted blob so the caller
// can remove the underlying GFS objects.
func sweepMarkedBlobs(ctx context.Context, db *sql.DB, markedBefore time.Time) ([]gcOrphanedBlob, error) {
	const q = `
DELETE FROM repository_blobs
WHERE gc_marked_at IS NOT NULL AND gc_marked_at < $1
RETURNING repository_id, digest, size`
	rows, err := db.QueryContext(ctx, q, markedBefore)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blobs []gcOrphanedBlob
	for rows.Next() {
		var b gcOrphanedBlob
		if err := rows.Scan(&b.RepositoryID, &b.Digest, &b.Size); err != nil {
			return nil, err
		}
		blobs = append(blobs, b)
	}
	return blobs, rows.Err()
}
