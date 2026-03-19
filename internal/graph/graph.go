// Package graph defines the shared data model for cluster resource graphs.
// Parsers for different GitOps technologies (Flux, ArgoCD, etc.) all produce
// Graph values that the render layer can consume uniformly.
package graph

// Node represents a single cluster resource in the dependency graph.
type Node struct {
	ID         string            `json:"id"`
	Layer      int               `json:"layer"`
	Type       string            `json:"type"`   // source | infra | apps | homerun2 | profile | argoapp
	Sub        string            `json:"sub"`    // short subtitle shown in the UI
	Source     string            `json:"source"` // name of the GitRepository / OCI source
	Path       string            `json:"path"`
	Version    string            `json:"version"`
	Interval   string            `json:"interval"`
	Domain     string            `json:"domain"`
	DependsOn  []string          `json:"dependsOn,omitempty"`
	Substitute map[string]string `json:"substitute,omitempty"`
	Technology string            `json:"technology"` // "flux" | "argocd"

	// Live status — populated only when --live is active.
	Status    string `json:"status,omitempty"`    // "ready" | "failed" | "progressing" | "unknown"
	Message   string `json:"message,omitempty"`   // last condition message
	UpdatedAt string `json:"updatedAt,omitempty"` // ISO-8601 timestamp
}

// Edge represents a directed dependency or watch relationship between nodes.
type Edge struct {
	S    string `json:"s"`
	T    string `json:"t"`
	Kind string `json:"kind"` // "watches" | "dependsOn" | "manages"
}

// Graph is the full graph for a single cluster profile.
type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// ClusterProfile is the unified representation produced by any parser.
type ClusterProfile struct {
	ClusterName string
	ClusterPath string
	Technology  string // "flux" | "argocd" | "mixed"
	Graph       Graph
	Meta        map[string]string // free-form metadata (version, sops, etc.)
}
