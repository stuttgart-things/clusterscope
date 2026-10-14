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

	// Workloads is the actual state of the workloads behind the node,
	// evaluated at refresh time — populated only when --live is active.
	Workloads *WorkloadHealth `json:"workloads,omitempty"`
}

// Workload health values.
const (
	WorkloadHealthy  = "healthy"
	WorkloadDegraded = "degraded"
	WorkloadDown     = "down"
	WorkloadNA       = "n/a" // no Deployments, StatefulSets or DaemonSets
)

// WorkloadHealth aggregates the Deployments, StatefulSets and DaemonSets that
// belong to a node (Flux inventory, HelmRelease labels or Argo CD resources).
type WorkloadHealth struct {
	Status string `json:"status"` // healthy | degraded | down | n/a
	Ready  int    `json:"ready"`  // workloads with all desired replicas available
	Total  int    `json:"total"`
	// Drift is set when the GitOps controller reports ready but the
	// workloads (or HelmReleases) are not — the state Flux hides until the
	// next reconcile.
	Drift        bool                `json:"drift,omitempty"`
	Workloads    []Workload          `json:"items,omitempty"`
	HelmReleases []HelmReleaseStatus `json:"helmReleases,omitempty"`
}

// Workload is one Deployment, StatefulSet or DaemonSet.
type Workload struct {
	Kind      string   `json:"kind"`
	Namespace string   `json:"namespace"`
	Name      string   `json:"name"`
	Ready     int32    `json:"ready"`
	Desired   int32    `json:"desired"`
	Status    string   `json:"status"`           // healthy | degraded | down
	Issues    []string `json:"issues,omitempty"` // "<pod>: CrashLoopBackOff"
	// Restarts counts container restarts since the previous refresh.
	Restarts int32 `json:"restarts,omitempty"`
}

// HelmReleaseStatus is the Ready condition of a Flux HelmRelease in a node's
// inventory.
type HelmReleaseStatus struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Status    string `json:"status"` // ready | failed | progressing | unknown
	Message   string `json:"message,omitempty"`
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
