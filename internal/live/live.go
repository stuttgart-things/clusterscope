// Package live enriches a ClusterProfile graph with real-time reconciliation
// status fetched from a running cluster via kubectl.
//
// No credentials are ever written to HTML output. KUBECONFIG follows standard
// kubectl conventions: --kubeconfig flag > KUBECONFIG env > ~/.kube/config.
package live

import (
	"encoding/json"
	"os/exec"
	"strings"

	"github.com/stuttgart-things/clusterscope/internal/graph"
)

// NodeStatus carries the live reconciliation status for one resource.
type NodeStatus struct {
	Status    string // "ready" | "failed" | "progressing" | "unknown"
	Message   string
	UpdatedAt string
}

// ── internal k8s list shapes ─────────────────────────────────────────────────

type k8sCondition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	Message            string `json:"message"`
	LastTransitionTime string `json:"lastTransitionTime"`
}

type k8sItem struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Status struct {
		Conditions []k8sCondition `json:"conditions"`
	} `json:"status"`
}

type k8sList struct {
	Items []k8sItem `json:"items"`
}

// ArgoCD uses health/sync sub-objects instead of standard conditions.
type argoItem struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Status struct {
		Health struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"health"`
		Sync struct {
			Status string `json:"status"`
		} `json:"sync"`
		ReconciledAt string `json:"reconciledAt"`
	} `json:"status"`
}

type argoList struct {
	Items []argoItem `json:"items"`
}

// ── public API ────────────────────────────────────────────────────────────────

// Enrich auto-detects technology and enriches the profile accordingly.
// Errors from kubectl are silently degraded to status "unknown" per the issue
// acceptance criteria — the tool must never crash on live failures.
func Enrich(profile *graph.ClusterProfile, kubeconfig string) {
	switch profile.Technology {
	case "argocd":
		EnrichArgoCD(profile, kubeconfig)
	default:
		EnrichFlux(profile, kubeconfig)
	}
}

// EnrichFlux enriches a Flux cluster profile via kubectl kustomizations +
// gitrepositories resources.
func EnrichFlux(profile *graph.ClusterProfile, kubeconfig string) {
	merged := make(map[string]NodeStatus)
	for k, v := range fetchFluxResource(kubeconfig, "kustomizations.kustomize.toolkit.fluxcd.io") {
		merged[k] = v
	}
	for k, v := range fetchFluxResource(kubeconfig, "gitrepositories.source.toolkit.fluxcd.io") {
		merged[k] = v
	}
	applyToProfile(profile, merged)
}

// EnrichArgoCD enriches an ArgoCD cluster profile via kubectl applications.
func EnrichArgoCD(profile *graph.ClusterProfile, kubeconfig string) {
	statuses := fetchArgoResource(kubeconfig, "applications.argoproj.io")
	applyToProfile(profile, statuses)
}

// ── fetcher helpers ───────────────────────────────────────────────────────────

func fetchFluxResource(kubeconfig, resource string) map[string]NodeStatus {
	out, err := runKubectl(kubeconfig, "get", resource, "--all-namespaces", "-o", "json")
	if err != nil {
		return nil
	}
	var list k8sList
	if err := json.Unmarshal(out, &list); err != nil {
		return nil
	}
	result := make(map[string]NodeStatus)
	for _, item := range list.Items {
		result[item.Metadata.Name] = statusFromConditions(item.Status.Conditions)
	}
	return result
}

func fetchArgoResource(kubeconfig, resource string) map[string]NodeStatus {
	out, err := runKubectl(kubeconfig, "get", resource, "--all-namespaces", "-o", "json")
	if err != nil {
		return nil
	}
	var list argoList
	if err := json.Unmarshal(out, &list); err != nil {
		return nil
	}
	result := make(map[string]NodeStatus)
	for _, item := range list.Items {
		result[item.Metadata.Name] = statusFromArgo(item)
	}
	return result
}

func runKubectl(kubeconfig string, args ...string) ([]byte, error) {
	if kubeconfig != "" {
		args = append([]string{"--kubeconfig", kubeconfig}, args...)
	}
	return exec.Command("kubectl", args...).Output() //nolint:gosec
}

// ── status mapping ────────────────────────────────────────────────────────────

func statusFromConditions(conditions []k8sCondition) NodeStatus {
	for _, c := range conditions {
		if c.Type != "Ready" {
			continue
		}
		var status string
		switch c.Status {
		case "True":
			status = "ready"
		case "False":
			msg := strings.ToLower(c.Message)
			if strings.Contains(msg, "progress") || strings.Contains(msg, "reconcil") {
				status = "progressing"
			} else {
				status = "failed"
			}
		default:
			status = "progressing"
		}
		return NodeStatus{Status: status, Message: c.Message, UpdatedAt: c.LastTransitionTime}
	}
	return NodeStatus{Status: "unknown"}
}

func statusFromArgo(item argoItem) NodeStatus {
	health := item.Status.Health.Status
	sync := item.Status.Sync.Status
	msg := item.Status.Health.Message
	ts := item.Status.ReconciledAt

	var status string
	switch {
	case health == "Healthy" && sync == "Synced":
		status = "ready"
	case health == "Degraded":
		status = "failed"
	case health == "Progressing" || sync == "OutOfSync":
		status = "progressing"
	default:
		status = "unknown"
	}
	return NodeStatus{Status: status, Message: msg, UpdatedAt: ts}
}

// ── apply to profile ──────────────────────────────────────────────────────────

func applyToProfile(profile *graph.ClusterProfile, statuses map[string]NodeStatus) {
	for i := range profile.Graph.Nodes {
		ns, ok := statuses[profile.Graph.Nodes[i].ID]
		if ok {
			profile.Graph.Nodes[i].Status = ns.Status
			profile.Graph.Nodes[i].Message = ns.Message
			profile.Graph.Nodes[i].UpdatedAt = ns.UpdatedAt
		} else {
			profile.Graph.Nodes[i].Status = "unknown"
		}
	}
}
