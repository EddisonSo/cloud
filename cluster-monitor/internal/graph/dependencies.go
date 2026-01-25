package graph

// EdgeType represents the type of connection between services
type EdgeType string

const (
	EdgeTypeHTTP EdgeType = "http"
	EdgeTypeGRPC EdgeType = "grpc"
	EdgeTypeDB   EdgeType = "db"
	EdgeTypeNATS EdgeType = "nats"
)

// Node represents a service in the dependency graph
type Node struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Type  string `json:"type"` // "service", "database", "messaging", "storage"
}

// Edge represents a connection between services
type Edge struct {
	ID       string   `json:"id"`
	Source   string   `json:"source"`
	Target   string   `json:"target"`
	EdgeType EdgeType `json:"type"`
	Label    string   `json:"label,omitempty"`
}

// DependencyGraph contains the full service dependency graph
type DependencyGraph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// GetDependencies returns the static service dependency graph
func GetDependencies() *DependencyGraph {
	nodes := []Node{
		// Gateway
		{ID: "gateway", Label: "Gateway", Type: "service"},

		// Core services
		{ID: "auth-service", Label: "Auth Service", Type: "service"},
		{ID: "edd-compute", Label: "Compute Service", Type: "service"},
		{ID: "simple-file-share-backend", Label: "Storage API", Type: "service"},
		{ID: "cluster-monitor", Label: "Cluster Monitor", Type: "service"},
		{ID: "edd-cloud-docs", Label: "Documentation", Type: "service"},

		// Infrastructure
		{ID: "postgres", Label: "PostgreSQL", Type: "database"},

		// GFS components
		{ID: "gfs-master", Label: "GFS Master", Type: "storage"},
		{ID: "gfs-chunkserver-s1", Label: "GFS Chunk (s1)", Type: "storage"},
		{ID: "gfs-chunkserver-s2", Label: "GFS Chunk (s2)", Type: "storage"},
		{ID: "gfs-chunkserver-s3", Label: "GFS Chunk (s3)", Type: "storage"},

		// Logging
		{ID: "log-service", Label: "Log Service", Type: "service"},
	}

	edges := []Edge{
		// Gateway routes
		{ID: "e1", Source: "gateway", Target: "auth-service", EdgeType: EdgeTypeHTTP},
		{ID: "e2", Source: "gateway", Target: "edd-compute", EdgeType: EdgeTypeHTTP},
		{ID: "e3", Source: "gateway", Target: "simple-file-share-backend", EdgeType: EdgeTypeHTTP},
		{ID: "e4", Source: "gateway", Target: "cluster-monitor", EdgeType: EdgeTypeHTTP},
		{ID: "e5", Source: "gateway", Target: "edd-cloud-docs", EdgeType: EdgeTypeHTTP},

		// Database connections
		{ID: "e6", Source: "auth-service", Target: "postgres", EdgeType: EdgeTypeDB},
		{ID: "e7", Source: "edd-compute", Target: "postgres", EdgeType: EdgeTypeDB},
		{ID: "e8", Source: "simple-file-share-backend", Target: "postgres", EdgeType: EdgeTypeDB},

		// NATS event flows (producer → consumer)
		// Auth publishes user events, Compute and Storage consume them
		{ID: "e9", Source: "auth-service", Target: "edd-compute", EdgeType: EdgeTypeNATS, Label: "user events"},
		{ID: "e10", Source: "auth-service", Target: "simple-file-share-backend", EdgeType: EdgeTypeNATS, Label: "user events"},
		// Compute publishes container/ingress events, Gateway consumes them
		{ID: "e17", Source: "edd-compute", Target: "gateway", EdgeType: EdgeTypeNATS, Label: "container events"},

		// GFS connections
		{ID: "e11", Source: "simple-file-share-backend", Target: "gfs-master", EdgeType: EdgeTypeGRPC},
		{ID: "e12", Source: "gfs-master", Target: "gfs-chunkserver-s1", EdgeType: EdgeTypeGRPC},
		{ID: "e13", Source: "gfs-master", Target: "gfs-chunkserver-s2", EdgeType: EdgeTypeGRPC},
		{ID: "e14", Source: "gfs-master", Target: "gfs-chunkserver-s3", EdgeType: EdgeTypeGRPC},

		// Logging connections
		{ID: "e15", Source: "cluster-monitor", Target: "log-service", EdgeType: EdgeTypeGRPC},
		{ID: "e16", Source: "log-service", Target: "gfs-master", EdgeType: EdgeTypeGRPC},
	}

	return &DependencyGraph{
		Nodes: nodes,
		Edges: edges,
	}
}
