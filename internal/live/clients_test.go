package live

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stuttgart-things/clusterscope/internal/graph"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynfake "k8s.io/client-go/dynamic/fake"
)

func writeKubeconfig(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestResolve(t *testing.T) {
	dir := t.TempDir()
	writeKubeconfig(t, dir, "prod", "x")
	writeKubeconfig(t, dir, "renamed.yaml", "x")
	abs := writeKubeconfig(t, t.TempDir(), "abs", "x")

	tests := []struct {
		name     string
		cfg      ClientConfig
		cluster  string
		wantPath string
		wantOK   bool
		wantErr  bool
	}{
		{"legacy: default for every cluster", ClientConfig{Kubeconfig: "/k"}, "any", "/k", true, false},
		{"legacy: in-cluster default", ClientConfig{}, "any", "", true, false},
		{"dir: file named after cluster", ClientConfig{KubeconfigDir: dir}, "prod", filepath.Join(dir, "prod"), true, false},
		{"dir: missing file is unconfigured", ClientConfig{KubeconfigDir: dir, Kubeconfig: "/k"}, "dev", "", false, false},
		{"dir: self falls back to default", ClientConfig{KubeconfigDir: dir, Kubeconfig: "/k", Self: "mgmt"}, "mgmt", "/k", true, false},
		{"dir: own file wins over self default", ClientConfig{KubeconfigDir: dir, Kubeconfig: "/k", Self: "prod"}, "prod", filepath.Join(dir, "prod"), true, false},
		{"map: relative to dir", ClientConfig{KubeconfigDir: dir, Map: map[string]string{"dev": "renamed.yaml"}}, "dev", filepath.Join(dir, "renamed.yaml"), true, false},
		{"map: absolute path", ClientConfig{Map: map[string]string{"dev": abs}}, "dev", abs, true, false},
		{"map: missing mapped file", ClientConfig{KubeconfigDir: dir, Map: map[string]string{"dev": "nope"}}, "dev", "", false, true},
		{"map only: unmapped cluster unconfigured", ClientConfig{Map: map[string]string{"dev": abs}}, "prod", "", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClients(tt.cfg)
			path, ok, err := c.resolve(tt.cluster)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if ok != tt.wantOK || path != tt.wantPath {
				t.Errorf("resolve(%q) = (%q, %v), want (%q, %v)", tt.cluster, path, ok, tt.wantPath, tt.wantOK)
			}
		})
	}
}

func TestLoadMapFile(t *testing.T) {
	p := writeKubeconfig(t, t.TempDir(), "map.yaml", "clusters:\n  prod: prod.yaml\n  dev: /abs/dev\n")
	m, err := LoadMapFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if m["prod"] != "prod.yaml" || m["dev"] != "/abs/dev" {
		t.Errorf("unexpected map: %v", m)
	}
}

var gvrToList = map[schema.GroupVersionResource]string{
	gvrKustomizations:   "KustomizationList",
	gvrGitRepositories:  "GitRepositoryList",
	gvrArgoApplications: "ApplicationList",
}

func readyKustomization(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata":   map[string]any{"name": name, "namespace": "flux-system"},
		"status": map[string]any{"conditions": []any{map[string]any{
			"type": "Ready", "status": "True", "message": "Applied revision", "lastTransitionTime": "2026-10-14T10:00:00Z",
		}}},
	}}
}

// fakeClients returns a Clients whose factory hands out a fake dynamic client
// per kubeconfig path and the given ping result.
func fakeClients(cfg ClientConfig, objs map[string][]runtime.Object, pingErr map[string]error, built *int) *Clients {
	c := NewClients(cfg)
	c.newClients = func(path string) (dynamic.Interface, pingFunc, error) {
		if built != nil {
			*built++
		}
		dyn := dynfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToList, objs[path]...)
		return dyn, func(context.Context) error { return pingErr[path] }, nil
	}
	return c
}

func fluxProfile(ids ...string) *graph.ClusterProfile {
	p := &graph.ClusterProfile{Technology: "flux"}
	for _, id := range ids {
		p.Graph.Nodes = append(p.Graph.Nodes, graph.Node{ID: id, Status: "ready", Message: "stale"})
	}
	return p
}

func TestEnrich_PerClusterClients(t *testing.T) {
	dir := t.TempDir()
	prod := writeKubeconfig(t, dir, "prod", "prod")
	dev := writeKubeconfig(t, dir, "dev", "dev")

	c := fakeClients(ClientConfig{KubeconfigDir: dir},
		map[string][]runtime.Object{
			prod: {readyKustomization("apps-prod")},
			dev:  {readyKustomization("apps-dev")},
		}, nil, nil)

	// Each cluster must be enriched from its own kubeconfig: "apps-dev" only
	// exists in dev and must not show up as ready in prod.
	p := fluxProfile("apps-prod", "apps-dev")
	st := c.Enrich(p, "prod")
	if st.Reachability != Reachable || st.Source != prod {
		t.Fatalf("prod status = %+v", st)
	}
	if got := p.Graph.Nodes[0].Status; got != "ready" {
		t.Errorf("apps-prod in prod: %q, want ready", got)
	}
	if got := p.Graph.Nodes[1].Status; got != "unknown" {
		t.Errorf("apps-dev in prod: %q, want unknown", got)
	}

	p = fluxProfile("apps-prod", "apps-dev")
	c.Enrich(p, "dev")
	if p.Graph.Nodes[0].Status != "unknown" || p.Graph.Nodes[1].Status != "ready" {
		t.Errorf("dev statuses: %q, %q", p.Graph.Nodes[0].Status, p.Graph.Nodes[1].Status)
	}
}

func TestEnrich_Unreachable(t *testing.T) {
	dir := t.TempDir()
	prod := writeKubeconfig(t, dir, "prod", "prod")
	c := fakeClients(ClientConfig{KubeconfigDir: dir}, nil,
		map[string]error{prod: errors.New("dial tcp: i/o timeout")}, nil)

	p := fluxProfile("apps")
	st := c.Enrich(p, "prod")
	if st.Reachability != Unreachable || st.Error == "" {
		t.Fatalf("status = %+v, want unreachable with error", st)
	}
	if n := p.Graph.Nodes[0]; n.Status != "unknown" || n.Message == "stale" {
		t.Errorf("stale node status not reset: %+v", n)
	}
}

func TestEnrich_Unconfigured(t *testing.T) {
	c := fakeClients(ClientConfig{KubeconfigDir: t.TempDir(), Kubeconfig: "/k"}, nil, nil, nil)

	p := fluxProfile("apps")
	st := c.Enrich(p, "dev")
	if st.Reachability != Unconfigured {
		t.Fatalf("status = %+v, want unconfigured", st)
	}
	if p.Graph.Nodes[0].Status != "unknown" {
		t.Errorf("node status = %q, want unknown", p.Graph.Nodes[0].Status)
	}
}

func TestClient_RebuiltWhenFileChanges(t *testing.T) {
	dir := t.TempDir()
	prod := writeKubeconfig(t, dir, "prod", "v1")
	built := 0
	c := fakeClients(ClientConfig{KubeconfigDir: dir}, nil, nil, &built)

	c.Enrich(fluxProfile("a"), "prod")
	c.Enrich(fluxProfile("a"), "prod")
	if built != 1 {
		t.Fatalf("client built %d times, want 1 (cached)", built)
	}

	// Simulate Secret rotation: new content and mtime.
	if err := os.WriteFile(prod, []byte("v2-rotated"), 0o600); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Minute)
	_ = os.Chtimes(prod, later, later)

	c.Enrich(fluxProfile("a"), "prod")
	if built != 2 {
		t.Errorf("client built %d times, want 2 after file change", built)
	}
}

func TestVersionPing(t *testing.T) {
	tests := []struct {
		code    int
		wantErr bool
	}{
		{http.StatusOK, false},
		{http.StatusNotFound, false},
		{http.StatusUnauthorized, true},
		{http.StatusForbidden, true},
		{http.StatusServiceUnavailable, true},
	}
	for _, tt := range tests {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/version" {
				t.Errorf("unexpected path %s", r.URL.Path)
			}
			w.WriteHeader(tt.code)
		}))
		err := versionPing(srv.URL+"/", srv.Client())(context.Background())
		if (err != nil) != tt.wantErr {
			t.Errorf("code %d: err = %v, wantErr %v", tt.code, err, tt.wantErr)
		}
		srv.Close()
	}

	if err := versionPing("http://127.0.0.1:1", http.DefaultClient)(context.Background()); err == nil {
		t.Error("expected error for closed port")
	}
}
