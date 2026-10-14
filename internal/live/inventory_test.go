package live

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/stuttgart-things/clusterscope/internal/graph"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestEnrich_CompareApps(t *testing.T) {
	dir := t.TempDir()
	prod := writeKubeconfig(t, dir, "prod", "prod")
	c := fakeClients(ClientConfig{KubeconfigDir: dir}, map[string][]runtime.Object{prod: {
		readyKustomization("apps"),
		readyKustomization("hand-applied"),
		nestedKustomization("child", "apps"),
	}}, nil, nil)

	p := &graph.ClusterProfile{Technology: "flux", Graph: graph.Graph{Nodes: []graph.Node{
		{ID: "flux-system", Type: "source", Technology: "flux"},
		{ID: "sync-root", Type: "source", Technology: "flux"},
		{ID: "apps", Type: "apps", Technology: "flux"},
		{ID: "monitoring", Type: "infra", Technology: "flux"},
	}}}
	st := c.Enrich(p, "prod")
	if !reflect.DeepEqual(st.NotDeployed, []string{"monitoring"}) {
		t.Errorf("not deployed: %v", st.NotDeployed)
	}
	if !reflect.DeepEqual(st.NotInGit, []string{"flux-system/hand-applied"}) {
		t.Errorf("not in git: %v", st.NotInGit)
	}
	if !reflect.DeepEqual(st.Nested, []string{"flux-system/child"}) {
		t.Errorf("nested: %v", st.Nested)
	}
}

func nestedKustomization(name, parent string) *unstructured.Unstructured {
	k := readyKustomization(name)
	k.SetLabels(map[string]string{"kustomize.toolkit.fluxcd.io/name": parent, "kustomize.toolkit.fluxcd.io/namespace": "flux-system"})
	return k
}

func TestAppsOf_Managed(t *testing.T) {
	plain := readyKustomization("a")
	owned := readyKustomization("b")
	owned.SetOwnerReferences([]metav1.OwnerReference{{Kind: "ApplicationSet", Name: "set"}})
	tracked := readyKustomization("c")
	tracked.SetAnnotations(map[string]string{"argocd.argoproj.io/tracking-id": "root:argoproj.io/Application:argocd/c"})
	got := appsOf([]unstructured.Unstructured{*plain, *owned, *tracked}, "Application")
	if got[0].managed || !got[1].managed || !got[2].managed {
		t.Errorf("managed flags: %+v", got)
	}
}

func TestCompareApps_ListFailed(t *testing.T) {
	p := &graph.ClusterProfile{Graph: graph.Graph{Nodes: []graph.Node{{ID: "apps", Type: "apps"}}}}
	if nd, ng, n := compareApps(p, nil); nd != nil || ng != nil || n != nil {
		t.Errorf("failed list must not report findings: %v %v", nd, ng)
	}
}

func TestIsAppNode_Argo(t *testing.T) {
	for _, tt := range []struct {
		n    graph.Node
		want bool
	}{
		{graph.Node{Type: "argoapp", Technology: "argocd"}, true},
		{graph.Node{Type: "argoappset", Technology: "argocd"}, false},
		{graph.Node{Type: "argoproject", Technology: "argocd"}, false},
		{graph.Node{Type: "homerun2", Technology: "flux"}, true},
	} {
		if got := isAppNode(tt.n); got != tt.want {
			t.Errorf("%+v: %v", tt.n, got)
		}
	}
}

func TestConfigured(t *testing.T) {
	dir := t.TempDir()
	writeKubeconfig(t, dir, "prod", "x")
	writeKubeconfig(t, dir, "dev", "x")
	writeKubeconfig(t, dir, ".hidden", "x")
	if err := os.Mkdir(filepath.Join(dir, "..2026_10_14"), 0o755); err != nil { // Secret mount internals
		t.Fatal(err)
	}
	if err := os.Symlink("..2026_10_14", filepath.Join(dir, "..data")); err != nil {
		t.Fatal(err)
	}
	c := NewClients(ClientConfig{KubeconfigDir: dir, Map: map[string]string{"edge": "/abs/edge"}})
	if got := c.Configured(); !reflect.DeepEqual(got, []string{"dev", "edge", "prod"}) {
		t.Errorf("configured: %v", got)
	}
}

func TestPing(t *testing.T) {
	dir := t.TempDir()
	prod := writeKubeconfig(t, dir, "prod", "prod")
	dead := writeKubeconfig(t, dir, "dead", "dead")
	c := fakeClients(ClientConfig{KubeconfigDir: dir}, nil, map[string]error{dead: errors.New("timeout")}, nil)
	if st := c.Ping("prod"); st.Reachability != Reachable || st.Source != prod {
		t.Errorf("prod: %+v", st)
	}
	if st := c.Ping("dead"); st.Reachability != Unreachable {
		t.Errorf("dead: %+v", st)
	}
	if st := c.Ping("nope"); st.Reachability != Unconfigured {
		t.Errorf("nope: %+v", st)
	}
}
