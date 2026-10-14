// Package live enriches a ClusterProfile graph with real-time reconciliation
// status fetched from a running cluster via the Kubernetes API (client-go).
//
// In-cluster: when running inside Kubernetes, the pod's ServiceAccount token
// is used automatically — no kubectl or KUBECONFIG required.
//
// Out-of-cluster: falls back to the kubeconfig path (--kubeconfig flag,
// KUBECONFIG env, or ~/.kube/config).
//
// Multi-cluster: Clients resolves a separate kubeconfig per cluster (see
// ClientConfig) and reports whether each cluster's API server is reachable.
package live

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strings"
	"time"

	"github.com/stuttgart-things/clusterscope/internal/graph"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
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

// ── k8s client construction ───────────────────────────────────────────────────

// newRESTConfig builds a REST config for a kubeconfig path. An empty path uses
// the in-cluster ServiceAccount when running inside Kubernetes, otherwise the
// default loading rules (KUBECONFIG env, ~/.kube/config).
func newRESTConfig(kubeconfig string) (*rest.Config, error) {
	var cfg *rest.Config
	var err error

	// Try in-cluster first (ServiceAccount token mount)
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" && kubeconfig == "" {
		cfg, err = rest.InClusterConfig()
	} else {
		// Out-of-cluster: explicit path or default loading rules
		rules := clientcmd.NewDefaultClientConfigLoadingRules()
		if kubeconfig != "" {
			rules.ExplicitPath = kubeconfig
		}
		cfg, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			rules, &clientcmd.ConfigOverrides{},
		).ClientConfig()
	}
	if err != nil {
		return nil, err
	}

	cfg.Timeout = 10 * time.Second
	return cfg, nil
}

// ── GVR definitions ───────────────────────────────────────────────────────────

var (
	gvrKustomizations   = schema.GroupVersionResource{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"}
	gvrGitRepositories  = schema.GroupVersionResource{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"}
	gvrArgoApplications = schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "applications"}
)

// ── Flux enrichment ───────────────────────────────────────────────────────────

func enrichFlux(profile *graph.ClusterProfile, client dynamic.Interface) {
	merged := make(map[string]NodeStatus)
	for k, v := range fetchResource(client, gvrKustomizations) {
		merged[k] = v
	}
	for k, v := range fetchResource(client, gvrGitRepositories) {
		merged[k] = v
	}
	applyToProfile(profile, merged)
}

// ── ArgoCD enrichment ─────────────────────────────────────────────────────────

func enrichArgoCD(profile *graph.ClusterProfile, client dynamic.Interface) {
	statuses := fetchArgoResource(client, gvrArgoApplications)
	applyToProfile(profile, statuses)
}

// ── fetcher helpers ───────────────────────────────────────────────────────────

func fetchResource(client dynamic.Interface, gvr schema.GroupVersionResource) map[string]NodeStatus {
	list, err := client.Resource(gvr).Namespace("").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		log.Printf("live: list %s: %v", gvr.Resource, err)
		return nil
	}
	result := make(map[string]NodeStatus)
	for _, item := range list.Items {
		raw, _ := item.MarshalJSON()
		var ki k8sItem
		if json.Unmarshal(raw, &ki) == nil {
			result[ki.Metadata.Name] = statusFromConditions(ki.Status.Conditions)
		}
	}
	return result
}

func fetchArgoResource(client dynamic.Interface, gvr schema.GroupVersionResource) map[string]NodeStatus {
	list, err := client.Resource(gvr).Namespace("").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		log.Printf("live: list %s: %v", gvr.Resource, err)
		return nil
	}
	result := make(map[string]NodeStatus)
	for _, item := range list.Items {
		raw, _ := item.MarshalJSON()
		var ai argoItem
		if json.Unmarshal(raw, &ai) == nil {
			result[ai.Metadata.Name] = statusFromArgo(ai)
		}
	}
	return result
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
