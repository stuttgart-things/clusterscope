package machinery

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynfake "k8s.io/client-go/dynamic/fake"
)

func stack(name string, spec, status map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "config.stuttgart-things.com/v1alpha1",
		"kind":       "ClusterStack",
		"metadata":   map[string]any{"name": name, "namespace": "default"},
		"spec":       spec,
		"status":     status,
	}}
}

func TestList(t *testing.T) {
	app := stack("sthings-app", map[string]any{
		"environmentConfig": "labda", "provider": "vsphere", "distribution": "rancher-rke2",
		"size": "medium-mem", "nodes": int64(3),
		"profiles": []any{"network", "storage-longhorn", "homerun2"},
		"rancher":  map[string]any{"argocd": map[string]any{"register": true}},
	}, map[string]any{
		"stage": "ready", "ready": true,
		"conditions": []any{
			map[string]any{"type": "Ready", "status": "True"},
			map[string]any{"type": "Synced", "status": "True"},
		},
	})
	// status.ready without the Ready condition is not ready.
	half := stack("half", map[string]any{"environmentConfig": "labul"}, map[string]any{
		"stage": "platform", "ready": true,
		"conditions": []any{
			map[string]any{"type": "Ready", "status": "False"},
			map[string]any{"type": "Synced", "status": "False", "message": "cannot compose"},
		},
	})
	scheme := runtime.NewScheme()
	client := dynfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{ClusterStacks: "ClusterStackList"}, half, app)

	recs, err := List(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 || recs[0].Name != "half" || recs[1].Name != "sthings-app" {
		t.Fatalf("records = %+v", recs)
	}
	a := recs[1]
	if a.Environment != "labda" || a.Distribution != "rancher-rke2" || a.Nodes != 3 || !a.Ready || !a.Synced || !a.Registered ||
		len(a.Profiles) != 3 || a.Profiles[1] != "storage-longhorn" || a.Stage != "ready" {
		t.Errorf("sthings-app = %+v", a)
	}
	h := recs[0]
	if h.Ready || h.Synced || h.SyncMessage != "cannot compose" || h.Registered || h.Stage != "platform" {
		t.Errorf("half = %+v", h)
	}
}
