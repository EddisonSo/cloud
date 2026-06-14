package eddsdk

import "time"

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

// --- Storage (SFS) types ---

// Namespace mirrors the storage service's namespaceInfo JSON.
type Namespace struct {
	Name       string  `json:"name"`
	Count      int     `json:"count"`
	Hidden     bool    `json:"hidden"`
	Visibility int     `json:"visibility"`
	OwnerID    *string `json:"owner_id,omitempty"`
}

// FileInfo mirrors the storage service's fileInfo JSON.
type FileInfo struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Namespace  string `json:"namespace"`
	Size       uint64 `json:"size"`
	CreatedAt  int64  `json:"created_at"`
	ModifiedAt int64  `json:"modified_at"`
}

// CreateNamespaceRequest is the body for creating a namespace.
type CreateNamespaceRequest struct {
	Name       string `json:"name"`
	Visibility *int   `json:"visibility,omitempty"`
}

// --- Registry types ---

// Repo mirrors the registry service's apiRepo JSON.
type Repo struct {
	Name       string    `json:"name"`
	Visibility int       `json:"visibility"`
	OwnerID    string    `json:"owner_id"`
	TagCount   int64     `json:"tag_count"`
	TotalSize  int64     `json:"total_size"`
	LastPushed time.Time `json:"last_pushed"`
}

// Tag mirrors the registry service's apiTag JSON.
type Tag struct {
	Name     string    `json:"name"`
	Digest   string    `json:"digest"`
	Size     int64     `json:"size"`
	PushedAt time.Time `json:"pushed_at"`
}

// --- Auth: service accounts + tokens ---

// ServiceAccount mirrors the auth service's serviceAccountResponse JSON.
type ServiceAccount struct {
	ID         string              `json:"id"`
	Name       string              `json:"name"`
	Scopes     map[string][]string `json:"scopes"`
	TokenCount int                 `json:"token_count"`
	CreatedAt  int64               `json:"created_at"`
}

// Token mirrors the auth service's tokenResponse JSON.
// Token field is only populated on creation.
type Token struct {
	ID               string              `json:"id"`
	Name             string              `json:"name"`
	Scopes           map[string][]string `json:"scopes"`
	ExpiresAt        int64               `json:"expires_at"`
	LastUsedAt       int64               `json:"last_used_at"`
	CreatedAt        int64               `json:"created_at"`
	ServiceAccountID *string             `json:"service_account_id,omitempty"`
	Token            string              `json:"token,omitempty"`
}

// CreateTokenRequest is the body for creating an API token.
// ExpiresIn accepts "30d", "90d", "365d", or "never".
type CreateTokenRequest struct {
	Name      string              `json:"name"`
	Scopes    map[string][]string `json:"scopes"`
	ExpiresIn string              `json:"expires_in"`
}

// --- SSH Keys ---

// SSHKey mirrors the compute service's ssh key JSON.
type SSHKey struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
	CreatedAt string `json:"created_at"`
}

// --- Networking (gateway) types ---

// Domain mirrors the gateway's domainResponse JSON.
type Domain struct {
	ID           string `json:"id"`
	Domain       string `json:"domain"`
	ContainerID  string `json:"container_id"`
	TargetPort   int    `json:"target_port"`
	Status       string `json:"status"`
	VerifyName   string `json:"verify_name"`
	VerifyToken  string `json:"verify_token"`
	DNSAutomated bool   `json:"dns_automated,omitempty"`
}

// CreateDomainRequest is the body for adding a custom domain.
type CreateDomainRequest struct {
	ContainerID string `json:"container_id"`
	Domain      string `json:"domain"`
	TargetPort  int    `json:"target_port"`
}

// CloudflareConnection mirrors the gateway's connectionResponse JSON.
type CloudflareConnection struct {
	ID        string   `json:"id"`
	Zones     []string `json:"zones"`
	CreatedAt string   `json:"created_at"`
}
