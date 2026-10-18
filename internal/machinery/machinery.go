// Package machinery reads the cluster orders of a machinery cluster -- the
// ClusterStack XRs Crossplane builds clusters from -- as a cluster source for
// the lab view (#60).
//
// A cluster built from a ClusterStack has no cluster directory: its desired
// state is the order (profiles, distribution) and the Argo CD ApplicationSets
// those profiles select. Without this source the lab view could only call it
// "kubeconfig present, but no cluster directory".
package machinery

import (
	"context"
	"fmt"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// ClusterStacks is the XR kind of crossplane-configurations bootstrap/cluster.
var ClusterStacks = schema.GroupVersionResource{Group: "config.stuttgart-things.com", Version: "v1alpha1", Resource: "clusterstacks"}

// Record is one cluster order.
type Record struct {
	Name         string   `json:"name"`
	Namespace    string   `json:"namespace"`
	Environment  string   `json:"environment,omitempty"` // spec.environmentConfig, e.g. labda
	Provider     string   `json:"provider,omitempty"`
	Distribution string   `json:"distribution,omitempty"`
	Size         string   `json:"size,omitempty"`
	Nodes        int64    `json:"nodes,omitempty"`
	Profiles     []string `json:"profiles,omitempty"`
	// Stage is status.stage (vm, baseos, ..., ready); Ready needs both
	// status.ready and the Ready condition, as cluster-build-watch does: a
	// stack can show status.ready while Ready is still False.
	Stage  string `json:"stage,omitempty"`
	Ready  bool   `json:"ready"`
	Synced bool   `json:"synced"`
	// SyncMessage is the Synced condition's message when Synced is False.
	SyncMessage string `json:"syncMessage,omitempty"`
	// Registered is spec.rancher.argocd.register: Argo CD deploys the
	// profiles' platforms onto the cluster.
	Registered bool `json:"registered"`
	Deleting   bool `json:"deleting,omitempty"`
}

// List returns every ClusterStack of the machinery cluster, sorted by name.
func List(ctx context.Context, client dynamic.Interface) ([]Record, error) {
	list, err := client.Resource(ClusterStacks).Namespace(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list clusterstacks: %w", err)
	}
	out := make([]Record, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, FromObject(&list.Items[i]))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Namespace < out[j].Namespace
	})
	return out, nil
}

// FromObject reads a ClusterStack.
func FromObject(u *unstructured.Unstructured) Record {
	str := func(fields ...string) string {
		v, _, _ := unstructured.NestedString(u.Object, fields...)
		return v
	}
	r := Record{
		Name:         u.GetName(),
		Namespace:    u.GetNamespace(),
		Environment:  str("spec", "environmentConfig"),
		Provider:     str("spec", "provider"),
		Distribution: str("spec", "distribution"),
		Size:         str("spec", "size"),
		Stage:        str("status", "stage"),
		Deleting:     u.GetDeletionTimestamp() != nil,
	}
	r.Nodes, _, _ = unstructured.NestedInt64(u.Object, "spec", "nodes")
	r.Profiles, _, _ = unstructured.NestedStringSlice(u.Object, "spec", "profiles")
	r.Registered, _, _ = unstructured.NestedBool(u.Object, "spec", "rancher", "argocd", "register")

	statusReady, _, _ := unstructured.NestedBool(u.Object, "status", "ready")
	conds, _, _ := unstructured.NestedSlice(u.Object, "status", "conditions")
	readyCond := false
	r.Synced = true
	for _, c := range conds {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		switch m["type"] {
		case "Ready":
			readyCond = m["status"] == "True"
		case "Synced":
			if m["status"] != "True" {
				r.Synced = false
				r.SyncMessage, _ = m["message"].(string)
			}
		}
	}
	r.Ready = statusReady && readyCond
	return r
}
