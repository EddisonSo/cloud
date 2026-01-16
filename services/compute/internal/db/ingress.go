package db

import "time"

type IngressRule struct {
	ID          int64     `json:"id"`
	ContainerID string    `json:"container_id"`
	Port        int       `json:"port"`        // External port
	TargetPort  int       `json:"target_port"` // Internal container port
	Protocol    string    `json:"protocol"`    // tcp or udp
	CreatedAt   time.Time `json:"created_at"`
}

// AllowedExternalPorts for ingress rules (external-facing ports)
// Port 22 is reserved for SSH (controlled via ssh_enabled toggle)
var AllowedExternalPorts = []int{80, 443, 8080}

// IsExternalPortAllowed checks if an external port can be used
func IsExternalPortAllowed(port int) bool {
	for _, p := range AllowedExternalPorts {
		if p == port {
			return true
		}
	}
	return false
}

// IsTargetPortAllowed checks if a target port is valid (1-65535)
func IsTargetPortAllowed(port int) bool {
	return port >= 1 && port <= 65535
}

func (db *DB) ListIngressRules(containerID string) ([]*IngressRule, error) {
	rows, err := db.Query(`
		SELECT id, container_id, port, COALESCE(target_port, port), protocol, created_at
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
		if err := rows.Scan(&r.ID, &r.ContainerID, &r.Port, &r.TargetPort, &r.Protocol, &r.CreatedAt); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

func (db *DB) AddIngressRule(containerID string, port, targetPort int, protocol string) (*IngressRule, error) {
	var r IngressRule
	err := db.QueryRow(`
		INSERT INTO ingress_rules (container_id, port, target_port, protocol)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (container_id, port) DO UPDATE SET target_port = $3, protocol = $4
		RETURNING id, container_id, port, target_port, protocol, created_at`,
		containerID, port, targetPort, protocol,
	).Scan(&r.ID, &r.ContainerID, &r.Port, &r.TargetPort, &r.Protocol, &r.CreatedAt)
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
