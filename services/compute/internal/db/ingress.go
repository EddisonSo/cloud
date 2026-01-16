package db

import "time"

type IngressRule struct {
	ID          int64     `json:"id"`
	ContainerID string    `json:"container_id"`
	Port        int       `json:"port"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
}

// Allowed ports for ingress rules
var AllowedPorts = map[int]string{
	22:   "SSH",
	80:   "HTTP",
	443:  "HTTPS",
	8080: "HTTP Alt",
}

func (db *DB) ListIngressRules(containerID string) ([]*IngressRule, error) {
	rows, err := db.Query(`
		SELECT id, container_id, port, enabled, created_at
		FROM ingress_rules
		WHERE container_id = $1
		ORDER BY port`,
		containerID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*IngressRule
	for rows.Next() {
		r := &IngressRule{}
		if err := rows.Scan(&r.ID, &r.ContainerID, &r.Port, &r.Enabled, &r.CreatedAt); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

func (db *DB) AddIngressRule(containerID string, port int) (*IngressRule, error) {
	var r IngressRule
	err := db.QueryRow(`
		INSERT INTO ingress_rules (container_id, port, enabled)
		VALUES ($1, $2, true)
		ON CONFLICT (container_id, port) DO UPDATE SET enabled = true
		RETURNING id, container_id, port, enabled, created_at`,
		containerID, port,
	).Scan(&r.ID, &r.ContainerID, &r.Port, &r.Enabled, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (db *DB) RemoveIngressRule(containerID string, port int) error {
	_, err := db.Exec(`
		DELETE FROM ingress_rules
		WHERE container_id = $1 AND port = $2`,
		containerID, port,
	)
	return err
}

func (db *DB) IsPortAllowed(containerID string, port int) (bool, error) {
	var enabled bool
	err := db.QueryRow(`
		SELECT enabled FROM ingress_rules
		WHERE container_id = $1 AND port = $2`,
		containerID, port,
	).Scan(&enabled)
	if err != nil {
		return false, nil // Not found means not allowed
	}
	return enabled, nil
}
