package argocd

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
	if p.Technology != "argocd" {
		t.Fatalf("expected technology argocd, got %s", p.Technology)
	}
	if len(p.Graph.Nodes) != 0 {
		t.Fatalf("expected 0 nodes, got %d", len(p.Graph.Nodes))
	}
}

func TestParseDir_Application(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "app.yaml", `
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: my-app
  namespace: argocd
spec:
  project: default
  source:
    repoURL: https://github.com/org/repo
    path: deploy/
    targetRevision: main
  destination:
    server: https://kubernetes.default.svc
    namespace: my-ns
`)
	p, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Graph.Nodes) != 2 {
		t.Fatalf("expected 2 nodes (project + app), got %d", len(p.Graph.Nodes))
	}
	// Should have default project (layer 0) and app (layer 2)
	var foundProject, foundApp bool
	for _, n := range p.Graph.Nodes {
		if n.Type == "argoproject" && n.Layer == 0 {
			foundProject = true
		}
		if n.Type == "argoapp" && n.Layer == 2 && n.ID == "my-app" {
			foundApp = true
			if n.Domain != "my-ns" {
				t.Errorf("expected domain my-ns, got %s", n.Domain)
			}
			if n.Path != "deploy/" {
				t.Errorf("expected path deploy/, got %s", n.Path)
			}
		}
	}
	if !foundProject {
		t.Error("missing project node")
	}
	if !foundApp {
		t.Error("missing app node")
	}
	if len(p.Graph.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(p.Graph.Edges))
	}
	if p.Graph.Edges[0].Kind != "manages" {
		t.Errorf("expected manages edge, got %s", p.Graph.Edges[0].Kind)
	}
}

func TestParseDir_ApplicationSet(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "appset.yaml", `
apiVersion: argoproj.io/v1alpha1
kind: ApplicationSet
metadata:
  name: my-appset
  namespace: argocd
spec:
  generators:
    - git:
        repoURL: https://github.com/org/config
        revision: main
        directories:
          - path: clusters/*/apps/*
  template:
    spec:
      project: default
`)
	p, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have default project (synthesized) + appset
	if len(p.Graph.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(p.Graph.Nodes))
	}
	var foundAppSet bool
	for _, n := range p.Graph.Nodes {
		if n.Type == "argoappset" && n.ID == "my-appset" && n.Layer == 1 {
			foundAppSet = true
			if n.Version != "main" {
				t.Errorf("expected version main, got %s", n.Version)
			}
		}
	}
	if !foundAppSet {
		t.Error("missing appset node")
	}
}

func TestParseDir_AppProject(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "project.yaml", `
apiVersion: argoproj.io/v1alpha1
kind: AppProject
metadata:
  name: platform
  namespace: argocd
spec:
  description: Platform team project
`)
	p, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Graph.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(p.Graph.Nodes))
	}
	n := p.Graph.Nodes[0]
	if n.ID != "project:platform" {
		t.Errorf("expected ID project:platform, got %s", n.ID)
	}
	if n.Sub != "Platform team project" {
		t.Errorf("expected sub 'Platform team project', got %s", n.Sub)
	}
}

func TestParseDir_MultiSource(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "multi.yaml", `
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: multi-src
spec:
  project: default
  sources:
    - repoURL: https://github.com/org/charts
      chart: mychart
      targetRevision: 1.2.3
    - repoURL: https://github.com/org/values
      path: overlays/prod
      targetRevision: main
  destination:
    namespace: prod
`)
	p, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var app *struct{ Path, Version string }
	for _, n := range p.Graph.Nodes {
		if n.ID == "multi-src" {
			app = &struct{ Path, Version string }{n.Path, n.Version}
		}
	}
	if app == nil {
		t.Fatal("missing multi-src node")
	}
	// First source wins: chart=mychart, revision=1.2.3
	if app.Path != "mychart" {
		t.Errorf("expected path mychart (from chart), got %s", app.Path)
	}
	if app.Version != "1.2.3" {
		t.Errorf("expected version 1.2.3, got %s", app.Version)
	}
}

func TestParseDir_MultiDocYAML(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "multi.yaml", `
apiVersion: argoproj.io/v1alpha1
kind: AppProject
metadata:
  name: infra
spec:
  description: Infra project
---
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: cert-manager
spec:
  project: infra
  source:
    repoURL: https://github.com/org/repo
    path: infra/cert-manager
    targetRevision: main
  destination:
    namespace: cert-manager
`)
	p, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Graph.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(p.Graph.Nodes))
	}
	if p.Meta["appProjects"] != "1" {
		t.Errorf("expected 1 appProject in meta, got %s", p.Meta["appProjects"])
	}
	if p.Meta["applications"] != "1" {
		t.Errorf("expected 1 application in meta, got %s", p.Meta["applications"])
	}
}

func TestParseDir_SkipsNonArgoYAML(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "configmap.yaml", `
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-config
data:
  key: value
`)
	p, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Graph.Nodes) != 0 {
		t.Fatalf("expected 0 nodes for non-argo YAML, got %d", len(p.Graph.Nodes))
	}
}

func TestParseDir_SkipsHiddenDirs(t *testing.T) {
	dir := t.TempDir()
	hidden := filepath.Join(dir, ".git")
	writeFile(t, hidden, "app.yaml", `
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: hidden-app
spec:
  project: default
  source:
    repoURL: https://example.com
    path: /
    targetRevision: main
  destination:
    namespace: default
`)
	p, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Graph.Nodes) != 0 {
		t.Fatalf("expected 0 nodes (hidden dir skipped), got %d", len(p.Graph.Nodes))
	}
}

func TestParseDir_SkipsNonYAML(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "readme.md", "# README")
	writeFile(t, dir, "app.json", `{"kind": "Application"}`)
	p, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Graph.Nodes) != 0 {
		t.Fatalf("expected 0 nodes, got %d", len(p.Graph.Nodes))
	}
}

func TestBuildGraph_DefaultProject(t *testing.T) {
	data := &clusterData{
		ClusterName: "test",
		Applications: []application{
			{Name: "app1", Project: ""},
		},
	}
	g := buildGraph(data)
	if len(g.Nodes) != 2 {
		t.Fatalf("expected 2 nodes (default project + app), got %d", len(g.Nodes))
	}
	if g.Nodes[0].ID != "project:default" {
		t.Errorf("expected synthesized default project, got %s", g.Nodes[0].ID)
	}
}

func TestBuildGraph_Sorting(t *testing.T) {
	data := &clusterData{
		ClusterName: "test",
		Applications: []application{
			{Name: "zebra", Project: "default"},
			{Name: "alpha", Project: "default"},
		},
	}
	g := buildGraph(data)
	// Apps should be sorted: alpha before zebra
	appNodes := []string{}
	for _, n := range g.Nodes {
		if n.Type == "argoapp" {
			appNodes = append(appNodes, n.ID)
		}
	}
	if len(appNodes) != 2 || appNodes[0] != "alpha" || appNodes[1] != "zebra" {
		t.Fatalf("expected [alpha, zebra], got %v", appNodes)
	}
}

func TestBuildProfile_Meta(t *testing.T) {
	data := &clusterData{
		ClusterName: "prod",
		ClusterPath: "/clusters/prod",
		Applications: []application{
			{Name: "app1"},
			{Name: "app2"},
		},
		ApplicationSets: []applicationSet{
			{Name: "set1"},
		},
		AppProjects: []appProject{
			{Name: "platform"},
		},
	}
	p := buildProfile(data)
	if p.Technology != "argocd" {
		t.Errorf("expected argocd, got %s", p.Technology)
	}
	if p.Meta["applications"] != "2" {
		t.Errorf("expected 2 applications, got %s", p.Meta["applications"])
	}
	if p.Meta["applicationSets"] != "1" {
		t.Errorf("expected 1 applicationSets, got %s", p.Meta["applicationSets"])
	}
	if p.Meta["appProjects"] != "1" {
		t.Errorf("expected 1 appProjects, got %s", p.Meta["appProjects"])
	}
}

func TestIntStr(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
	}
	for _, tt := range tests {
		if got := intStr(tt.in); got != tt.want {
			t.Errorf("intStr(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
