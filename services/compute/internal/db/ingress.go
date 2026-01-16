package db

import "time"

type IngressRule struct {
	ID          int64     `json:"id"`
	ContainerID string    `json:"container_id"`
	Port        int       `json:"port"`
	Protocol    string    `json:"protocol"` // tcp or udp
	CreatedAt   time.Time `json:"created_at"`
}

// AllowedPorts returns the list of allowed ports for the UI dropdown
// Port 22 is reserved for SSH (controlled via ssh_enabled toggle)
func AllowedPorts() []int {
	ports := []int{80, 443}
	for p := 8000; p <= 8999; p++ {
		ports = append(ports, p)
	}
	return ports
}

// IsPortAllowed checks if a port can be used for ingress rules
// Allowed: 80, 443, 8000-8999
func IsPortAllowed(port int) bool {
	if port == 80 || port == 443 {
		return true
	}
	if port >= 8000 && port <= 8999 {
		return true
	}
	return false
}

func (db *DB) ListIngressRules(containerID string) ([]*IngressRule, error) {
	rows, err := db.Query(`
		SELECT id, container_id, port, protocol, created_at
		FROM ingress_rules
		WHERE container_id = $1
		ORDER BY port, protocol`,
		containerID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*IngressRule
	for rows.Next() {
		r := &IngressRule{}
		if err := rows.Scan(&r.ID, &r.ContainerID, &r.Port, &r.Protocol, &r.CreatedAt); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

func (db *DB) AddIngressRule(containerID string, port int, protocol string) (*IngressRule, error) {
	var r IngressRule
	err := db.QueryRow(`
		INSERT INTO ingress_rules (container_id, port, protocol)
		VALUES ($1, $2, $3)
		ON CONFLICT (container_id, port) DO UPDATE SET protocol = $3
		RETURNING id, container_id, port, protocol, created_at`,
		containerID, port, protocol,
	).Scan(&r.ID, &r.ContainerID, &r.Port, &r.Protocol, &r.CreatedAt)
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

func (db *DB) RemoveIngressRuleByID(id int64) error {
	_, err := db.Exec(`DELETE FROM ingress_rules WHERE id = $1`, id)
	return err
}
