package flux

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseDir_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	p, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Technology != "flux" {
		t.Fatalf("expected technology flux, got %s", p.Technology)
	}
	if len(p.Graph.Nodes) != 0 {
		t.Fatalf("expected 0 nodes, got %d", len(p.Graph.Nodes))
	}
}

func TestParseDir_Kustomization(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "ks.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: cert-manager
  namespace: flux-system
spec:
  interval: 5m
  path: ./infra/cert-manager
  sourceRef:
    kind: GitRepository
    name: flux-system
  dependsOn:
    - name: nfs-csi
  postBuild:
    substitute:
      VERSION: "1.14.0"
      DOMAIN: example.com
`)
	p, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Graph.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(p.Graph.Nodes))
	}
	n := p.Graph.Nodes[0]
	if n.ID != "cert-manager" {
		t.Errorf("expected ID cert-manager, got %s", n.ID)
	}
	if n.Version != "1.14.0" {
		t.Errorf("expected version 1.14.0, got %s", n.Version)
	}
	if n.Domain != "example.com" {
		t.Errorf("expected domain example.com, got %s", n.Domain)
	}
	if n.Interval != "5m" {
		t.Errorf("expected interval 5m, got %s", n.Interval)
	}
	if n.Source != "flux-system" {
		t.Errorf("expected source flux-system, got %s", n.Source)
	}
	if len(n.DependsOn) != 1 || n.DependsOn[0] != "nfs-csi" {
		t.Errorf("expected dependsOn [nfs-csi], got %v", n.DependsOn)
	}
}

func TestParseDir_GitRepository(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "repo.yaml", `
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: flux-system
  namespace: flux-system
spec:
  url: https://github.com/org/repo
  ref:
    branch: main
  interval: 1m
`)
	p, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Graph.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(p.Graph.Nodes))
	}
	n := p.Graph.Nodes[0]
	if n.ID != "flux-system" {
		t.Errorf("expected ID flux-system, got %s", n.ID)
	}
	if n.Type != "source" {
		t.Errorf("expected type source, got %s", n.Type)
	}
	if n.Layer != 0 {
		t.Errorf("expected layer 0, got %d", n.Layer)
	}
	if n.Path != "https://github.com/org/repo" {
		t.Errorf("expected path to be repo URL, got %s", n.Path)
	}
}

func TestParseDir_FluxInstance(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "flux.yaml", `
apiVersion: fluxcd.controlplane.io/v1
kind: FluxInstance
metadata:
  name: flux
spec:
  distribution:
    version: "2.3.0"
  sync:
    url: https://github.com/org/repo
    path: clusters/prod
    ref: main
  components:
    - source-controller
    - kustomize-controller
`)
	p, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// FluxInstance creates a sync-root source node
	if len(p.Graph.Nodes) != 1 {
		t.Fatalf("expected 1 node (sync-root), got %d", len(p.Graph.Nodes))
	}
	if p.Graph.Nodes[0].ID != "sync-root" {
		t.Errorf("expected sync-root, got %s", p.Graph.Nodes[0].ID)
	}
	if p.Meta["fluxVersion"] != "2.3.0" {
		t.Errorf("expected fluxVersion 2.3.0, got %s", p.Meta["fluxVersion"])
	}
	if p.Meta["fluxSyncURL"] != "https://github.com/org/repo" {
		t.Errorf("expected fluxSyncURL, got %s", p.Meta["fluxSyncURL"])
	}
}

func TestParseDir_MultiDoc(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "multi.yaml", `
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: flux-system
spec:
  url: https://github.com/org/repo
  ref:
    branch: main
  interval: 1m
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: infra-controllers
spec:
  interval: 10m
  path: ./infra/controllers
  sourceRef:
    name: flux-system
`)
	p, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Graph.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(p.Graph.Nodes))
	}
}

func TestParseDir_SkipsVaultDir(t *testing.T) {
	dir := t.TempDir()
	vaultDir := filepath.Join(dir, "vault")
	writeFile(t, vaultDir, "secret.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: vault-secret
spec:
  interval: 5m
  path: ./vault
  sourceRef:
    name: flux-system
`)
	p, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Graph.Nodes) != 0 {
		t.Fatalf("expected 0 nodes (vault skipped), got %d", len(p.Graph.Nodes))
	}
}

func TestParseDir_SkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()
	hidden := filepath.Join(dir, ".git")
	writeFile(t, hidden, "ks.yaml", `
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: hidden
spec:
  interval: 5m
  path: ./hidden
  sourceRef:
    name: flux-system
`)
	p, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Graph.Nodes) != 0 {
		t.Fatalf("expected 0 nodes (hidden dir skipped), got %d", len(p.Graph.Nodes))
	}
}

// ── classifyLayer ───────────────────────────────────────────────────────────

func TestClassifyLayer(t *testing.T) {
	tests := []struct {
		path string
		name string
		want string
	}{
		{"./infra/cert-manager", "cert-manager", "infra"},
		{"./infra", "base-infra", "infra"},
		{"./apps/homerun/config", "homerun-config", "homerun2"},
		{"./apps/other", "homerun-deploy", "homerun2"},
		{"./apps/monitoring", "profile-metrics", "profile"},
		{"./apps/monitoring", "alert-manager", "profile"},
		{"./apps/web", "frontend", "apps"},
		{"./deploy", "backend", "apps"},
	}
	for _, tt := range tests {
		got := classifyLayer(tt.path, tt.name)
		if got != tt.want {
			t.Errorf("classifyLayer(%q, %q) = %q, want %q", tt.path, tt.name, got, tt.want)
		}
	}
}

// ── sanitizeSubstitute ──────────────────────────────────────────────────────

func TestSanitizeSubstitute(t *testing.T) {
	input := map[string]string{
		"VERSION":     "1.0.0",
		"DOMAIN":      "example.com",
		"SECRET":      "ENC[AES256_GCM,data:abc123]",
		"OTHER_ENC":   "ENC[something]",
		"NORMAL_ENC":  "not-encrypted",
	}
	got := sanitizeSubstitute(input)
	if got["VERSION"] != "1.0.0" {
		t.Errorf("expected VERSION=1.0.0, got %s", got["VERSION"])
	}
	if got["DOMAIN"] != "example.com" {
		t.Errorf("expected DOMAIN=example.com, got %s", got["DOMAIN"])
	}
	if _, ok := got["SECRET"]; ok {
		t.Error("SECRET should be filtered out")
	}
	if _, ok := got["OTHER_ENC"]; ok {
		t.Error("OTHER_ENC should be filtered out")
	}
	if got["NORMAL_ENC"] != "not-encrypted" {
		t.Errorf("expected NORMAL_ENC=not-encrypted, got %s", got["NORMAL_ENC"])
	}
}

func TestSanitizeSubstitute_Empty(t *testing.T) {
	got := sanitizeSubstitute(map[string]string{})
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestSanitizeSubstitute_Nil(t *testing.T) {
	got := sanitizeSubstitute(nil)
	if got == nil {
		t.Error("expected non-nil empty map")
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

// ── buildGraph ──────────────────────────────────────────────────────────────

func TestBuildGraph_Layers(t *testing.T) {
	data := &clusterData{
		ClusterName: "test",
		GitRepos: []gitRepository{
			{Name: "flux-system", URL: "https://github.com/org/repo", Branch: "main", Interval: "1m"},
		},
		Kustomizations: []kustomization{
			{Name: "nfs-csi", Source: "flux-system", Path: "./infra/nfs-csi", Layer: "infra"},
			{Name: "cert-manager", Source: "flux-system", Path: "./infra/cert-manager", Layer: "infra", DependsOn: []string{"nfs-csi"}},
			{Name: "frontend", Source: "flux-system", Path: "./apps/frontend", Layer: "apps"},
			{Name: "homerun-app", Source: "flux-system", Path: "./apps/homerun", Layer: "homerun2"},
		},
	}
	g := buildGraph(data)

	layerMap := map[string]int{}
	for _, n := range g.Nodes {
		layerMap[n.ID] = n.Layer
	}
	if layerMap["flux-system"] != 0 {
		t.Errorf("flux-system: expected layer 0, got %d", layerMap["flux-system"])
	}
	if layerMap["nfs-csi"] != 1 {
		t.Errorf("nfs-csi (base infra): expected layer 1, got %d", layerMap["nfs-csi"])
	}
	if layerMap["cert-manager"] != 2 {
		t.Errorf("cert-manager (dep infra): expected layer 2, got %d", layerMap["cert-manager"])
	}
	if layerMap["frontend"] != 3 {
		t.Errorf("frontend (apps): expected layer 3, got %d", layerMap["frontend"])
	}
	if layerMap["homerun-app"] != 4 {
		t.Errorf("homerun-app: expected layer 4, got %d", layerMap["homerun-app"])
	}
}

func TestBuildGraph_Edges(t *testing.T) {
	data := &clusterData{
		ClusterName: "test",
		GitRepos: []gitRepository{
			{Name: "flux-system", URL: "https://github.com/org/repo", Branch: "main"},
		},
		Kustomizations: []kustomization{
			{Name: "base-infra", Source: "flux-system", Path: "./infra/base", Layer: "infra"},
			{Name: "dep-infra", Source: "flux-system", Path: "./infra/dep", Layer: "infra", DependsOn: []string{"base-infra"}},
		},
	}
	g := buildGraph(data)

	edgeMap := map[string]string{}
	for _, e := range g.Edges {
		edgeMap[e.S+"->"+e.T] = e.Kind
	}
	if edgeMap["flux-system->base-infra"] != "watches" {
		t.Error("missing watches edge: flux-system -> base-infra")
	}
	if edgeMap["flux-system->dep-infra"] != "watches" {
		t.Error("missing watches edge: flux-system -> dep-infra")
	}
	if edgeMap["base-infra->dep-infra"] != "dependsOn" {
		t.Error("missing dependsOn edge: base-infra -> dep-infra")
	}
}

func TestBuildGraph_SyncRoot(t *testing.T) {
	data := &clusterData{
		ClusterName: "test",
		FluxInstance: &fluxInstance{
			Version: "2.3.0",
			SyncURL: "https://github.com/org/repo",
			SyncRef: "main",
		},
	}
	g := buildGraph(data)
	if len(g.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(g.Nodes))
	}
	if g.Nodes[0].ID != "sync-root" {
		t.Errorf("expected sync-root, got %s", g.Nodes[0].ID)
	}
	if g.Nodes[0].Layer != 0 {
		t.Errorf("expected layer 0, got %d", g.Nodes[0].Layer)
	}
}
