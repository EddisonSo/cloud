package db

import (
	"database/sql"
	"fmt"
	"time"
)

type Container struct {
	ID           string
	UserID       int64
	Owner        string // Username of the owner
	Name         string
	Namespace    string
	Status       string
	ExternalIP   sql.NullString
	MemoryMB     int
	StorageGB    int
	Image        string
	Arch         string // CPU architecture: arm64 or amd64
	CreatedAt    time.Time
	StoppedAt    sql.NullTime
	SSHEnabled   bool
	HTTPSEnabled bool
}

func (db *DB) CreateContainer(c *Container) error {
	_, err := db.Exec(`
		INSERT INTO containers (id, user_id, owner_username, name, namespace, status, memory_mb, storage_gb, image, arch, ssh_enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		c.ID, c.UserID, c.Owner, c.Name, c.Namespace, c.Status, c.MemoryMB, c.StorageGB, c.Image, c.Arch, c.SSHEnabled,
	)
	if err != nil {
		return fmt.Errorf("insert container: %w", err)
	}
	return nil
}

func (db *DB) GetContainer(id string) (*Container, error) {
	c := &Container{}
	err := db.QueryRow(`
		SELECT id, user_id, name, namespace, status, external_ip, memory_mb, storage_gb, image,
		       COALESCE(arch, 'arm64'), created_at, stopped_at,
		       COALESCE(ssh_enabled, false), COALESCE(https_enabled, false)
		FROM containers WHERE id = $1`, id,
	).Scan(&c.ID, &c.UserID, &c.Name, &c.Namespace, &c.Status, &c.ExternalIP, &c.MemoryMB, &c.StorageGB, &c.Image,
		&c.Arch, &c.CreatedAt, &c.StoppedAt, &c.SSHEnabled, &c.HTTPSEnabled)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query container: %w", err)
	}
	return c, nil
}

func (db *DB) ListContainersByUser(userID int64) ([]*Container, error) {
	rows, err := db.Query(`
		SELECT id, user_id, name, namespace, status, external_ip, memory_mb, storage_gb, image,
		       COALESCE(arch, 'arm64'), created_at, stopped_at,
		       COALESCE(ssh_enabled, false), COALESCE(https_enabled, false)
		FROM containers WHERE user_id = $1 ORDER BY created_at DESC`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("query containers: %w", err)
	}
	defer rows.Close()

	var containers []*Container
	for rows.Next() {
		c := &Container{}
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.Namespace, &c.Status, &c.ExternalIP, &c.MemoryMB, &c.StorageGB, &c.Image,
			&c.Arch, &c.CreatedAt, &c.StoppedAt, &c.SSHEnabled, &c.HTTPSEnabled); err != nil {
			return nil, fmt.Errorf("scan container: %w", err)
		}
		containers = append(containers, c)
	}
	return containers, nil
}

func (db *DB) ListAllContainers() ([]*Container, error) {
	rows, err := db.Query(`
		SELECT id, user_id, COALESCE(owner_username, ''), name, namespace, status, external_ip,
		       memory_mb, storage_gb, image, COALESCE(arch, 'arm64'), created_at, stopped_at,
		       COALESCE(ssh_enabled, false), COALESCE(https_enabled, false)
		FROM containers
		ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query containers: %w", err)
	}
	defer rows.Close()

	var containers []*Container
	for rows.Next() {
		c := &Container{}
		if err := rows.Scan(&c.ID, &c.UserID, &c.Owner, &c.Name, &c.Namespace, &c.Status, &c.ExternalIP, &c.MemoryMB, &c.StorageGB, &c.Image,
			&c.Arch, &c.CreatedAt, &c.StoppedAt, &c.SSHEnabled, &c.HTTPSEnabled); err != nil {
			return nil, fmt.Errorf("scan container: %w", err)
		}
		containers = append(containers, c)
	}
	return containers, nil
}

func (db *DB) UpdateContainerStatus(id, status string) error {
	_, err := db.Exec(`UPDATE containers SET status = $1 WHERE id = $2`, status, id)
	if err != nil {
		return fmt.Errorf("update container status: %w", err)
	}
	return nil
}

func (db *DB) UpdateContainerIP(id, ip string) error {
	_, err := db.Exec(`UPDATE containers SET external_ip = $1 WHERE id = $2`, ip, id)
	if err != nil {
		return fmt.Errorf("update container ip: %w", err)
	}
	return nil
}

func (db *DB) UpdateContainerStopped(id string) error {
	_, err := db.Exec(`UPDATE containers SET status = 'stopped', stopped_at = CURRENT_TIMESTAMP WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("update container stopped: %w", err)
	}
	return nil
}

func (db *DB) DeleteContainer(id string) error {
	_, err := db.Exec(`DELETE FROM containers WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete container: %w", err)
	}
	return nil
}

func (db *DB) CountContainersByUser(userID int64) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM containers WHERE user_id = $1 AND status != 'deleted'`, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count containers: %w", err)
	}
	return count, nil
}

func (db *DB) UpdateSSHEnabled(id string, enabled bool) error {
	_, err := db.Exec(`UPDATE containers SET ssh_enabled = $1 WHERE id = $2`, enabled, id)
	if err != nil {
		return fmt.Errorf("update ssh enabled: %w", err)
	}
	return nil
}

func (db *DB) UpdateHTTPSEnabled(id string, enabled bool) error {
	_, err := db.Exec(`UPDATE containers SET https_enabled = $1 WHERE id = $2`, enabled, id)
	if err != nil {
		return fmt.Errorf("update https enabled: %w", err)
	}
	return nil
}
