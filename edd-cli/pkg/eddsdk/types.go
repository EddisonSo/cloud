package eddsdk

// Container mirrors the compute service's container JSON.
type Container struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	Hostname     string `json:"hostname"`
	MemoryMB     int    `json:"memory_mb"`
	StorageGB    int    `json:"storage_gb"`
	InstanceType string `json:"instance_type"`
	CreatedAt    string `json:"created_at"`
	SSHEnabled   bool   `json:"ssh_enabled"`
	HTTPSEnabled bool   `json:"https_enabled"`
	PullPolicy   string `json:"pull_policy"`
}

// CreateContainerRequest is the body for creating a container.
type CreateContainerRequest struct {
	Name         string   `json:"name"`
	MemoryMB     int      `json:"memory_mb"`
	StorageGB    int      `json:"storage_gb"`
	InstanceType string   `json:"instance_type"`
	SSHKeyIDs    []int64  `json:"ssh_key_ids,omitempty"`
	SSHEnabled   bool     `json:"ssh_enabled"`
	MountPaths   []string `json:"mount_paths,omitempty"`
	Image        string   `json:"image,omitempty"`
	PullPolicy   string   `json:"pull_policy,omitempty"`
}

// IngressRule mirrors a container ingress rule.
// The API wraps rules in {"rules": [...]} and each rule includes id and created_at.
type IngressRule struct {
	ID         int64 `json:"id"`
	Port       int   `json:"port"`
	TargetPort int   `json:"target_port"`
	CreatedAt  int64 `json:"created_at"`
}
