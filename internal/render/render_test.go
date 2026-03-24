package render

import (
	"bytes"
	"html/template"
	"testing"

	"github.com/stuttgart-things/clusterscope/internal/diff"
	"github.com/stuttgart-things/clusterscope/internal/graph"
)

func TestParseSub(t *testing.T) {
	tests := []struct {
		in             string
		wantBranch     string
		wantInterval   string
	}{
		{"main · 5m", "main", "5m"},
		{"main · 10m0s", "main", "10m0s"},
		{"main", "main", ""},
		{"", "", ""},
	}
	for _, tt := range tests {
		branch, interval := parseSub(tt.in)
		if branch != tt.wantBranch || interval != tt.wantInterval {
			t.Errorf("parseSub(%q) = (%q, %q), want (%q, %q)",
				tt.in, branch, interval, tt.wantBranch, tt.wantInterval)
		}
	}
}

func TestNodeToCard(t *testing.T) {
	n := graph.Node{
		ID:        "cert-manager",
		Path:      "./infra/cert-manager",
		Version:   "1.14.0",
		Domain:    "example.com",
		DependsOn: []string{"nfs-csi"},
	}
	c := nodeToCard(n)
	if c.Name != "cert-manager" {
		t.Errorf("Name: got %s", c.Name)
	}
	if c.Path != "./infra/cert-manager" {
		t.Errorf("Path: got %s", c.Path)
	}
	if c.Version != "1.14.0" {
		t.Errorf("Version: got %s", c.Version)
	}
	if c.Domain != "example.com" {
		t.Errorf("Domain: got %s", c.Domain)
	}
	if len(c.DependsOn) != 1 || c.DependsOn[0] != "nfs-csi" {
		t.Errorf("DependsOn: got %v", c.DependsOn)
	}
}

func TestBuildTemplateData_Flux(t *testing.T) {
	profile := &graph.ClusterProfile{
		ClusterName: "dev",
		ClusterPath: "/clusters/dev",
		Technology:  "flux",
		Graph: graph.Graph{
			Nodes: []graph.Node{
				{ID: "flux-system", Layer: 0, Type: "source", Sub: "main · 1m", Path: "https://github.com/org/repo", Technology: "flux"},
				{ID: "cert-manager", Layer: 1, Type: "infra", Path: "./infra/cert-manager", Version: "1.14.0", Technology: "flux"},
				{ID: "frontend", Layer: 3, Type: "apps", Path: "./apps/frontend", Technology: "flux"},
				{ID: "homerun-app", Layer: 4, Type: "homerun2", Path: "./apps/homerun", Technology: "flux"},
			},
		},
		Meta: map[string]string{
			"fluxVersion":    "2.3.0",
			"fluxSyncURL":    "https://github.com/org/repo",
			"fluxSyncPath":   "clusters/dev",
			"fluxSyncRef":    "main",
			"fluxComponents": `["source-controller","kustomize-controller"]`,
		},
	}
	td := buildTemplateData(profile, template.JS("{}"))

	if td.ClusterName != "dev" {
		t.Errorf("ClusterName: %s", td.ClusterName)
	}
	if td.Technology != "flux" {
		t.Errorf("Technology: %s", td.Technology)
	}
	if len(td.GitRepos) != 1 {
		t.Fatalf("GitRepos: expected 1, got %d", len(td.GitRepos))
	}
	if td.GitRepos[0].Branch != "main" {
		t.Errorf("GitRepo branch: %s", td.GitRepos[0].Branch)
	}
	if !td.HasSources {
		t.Error("HasSources should be true")
	}
	if !td.HasInfra {
		t.Error("HasInfra should be true")
	}
	if !td.HasApps {
		t.Error("HasApps should be true")
	}
	if !td.HasHomerun {
		t.Error("HasHomerun should be true")
	}
	if td.FluxInstance == nil {
		t.Fatal("FluxInstance should not be nil")
	}
	if td.FluxInstance.Version != "2.3.0" {
		t.Errorf("FluxInstance.Version: %s", td.FluxInstance.Version)
	}
	if len(td.FluxInstance.Components) != 2 {
		t.Errorf("FluxInstance.Components: %v", td.FluxInstance.Components)
	}
}

func TestBuildTemplateData_ArgoCD(t *testing.T) {
	profile := &graph.ClusterProfile{
		ClusterName: "prod",
		Technology:  "argocd",
		Graph: graph.Graph{
			Nodes: []graph.Node{
				{ID: "project:default", Layer: 0, Type: "argoproject", Sub: "AppProject", Technology: "argocd"},
				{ID: "my-appset", Layer: 1, Type: "argoappset", Sub: "clusters/*/apps/*", Version: "main", Technology: "argocd"},
				{ID: "my-app", Layer: 2, Type: "argoapp", Path: "deploy/", Version: "main", Domain: "default", Technology: "argocd"},
			},
		},
		Meta: map[string]string{},
	}
	td := buildTemplateData(profile, template.JS("{}"))

	if !td.HasArgoProjects {
		t.Error("HasArgoProjects should be true")
	}
	if !td.HasArgoAppSets {
		t.Error("HasArgoAppSets should be true")
	}
	if !td.HasArgoApps {
		t.Error("HasArgoApps should be true")
	}
	if len(td.ArgoProjects) != 1 {
		t.Fatalf("ArgoProjects: expected 1, got %d", len(td.ArgoProjects))
	}
	if td.ArgoProjects[0].Name != "project:default" {
		t.Errorf("ArgoProject name: %s", td.ArgoProjects[0].Name)
	}
	if len(td.ArgoApps) != 1 || td.ArgoApps[0].Name != "my-app" {
		t.Errorf("ArgoApps: %v", td.ArgoApps)
	}
}

func TestSetBuildInfo(t *testing.T) {
	SetBuildInfo("1.2.3", "abc123", "2025-01-01")
	profile := &graph.ClusterProfile{
		ClusterName: "test",
		Technology:  "flux",
		Graph:       graph.Graph{},
		Meta:        map[string]string{},
	}
	td := buildTemplateData(profile, template.JS("{}"))
	if td.AppVersion != "1.2.3" {
		t.Errorf("AppVersion: %s", td.AppVersion)
	}
	if td.BuildCommit != "abc123" {
		t.Errorf("BuildCommit: %s", td.BuildCommit)
	}
	if td.BuildDate != "2025-01-01" {
		t.Errorf("BuildDate: %s", td.BuildDate)
	}
	// Reset
	SetBuildInfo("dev", "unknown", "unknown")
}

func TestWriteHTML(t *testing.T) {
	profile := &graph.ClusterProfile{
		ClusterName: "test-cluster",
		Technology:  "flux",
		Graph: graph.Graph{
			Nodes: []graph.Node{
				{ID: "flux-system", Type: "source", Sub: "main · 1m", Technology: "flux"},
			},
		},
		Meta: map[string]string{},
	}
	var buf bytes.Buffer
	if err := WriteHTML(&buf, profile); err != nil {
		t.Fatalf("WriteHTML: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("expected non-empty HTML output")
	}
	if !bytes.Contains(buf.Bytes(), []byte("test-cluster")) {
		t.Error("HTML should contain cluster name")
	}
}

func TestWriteShell(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteShell(&buf); err != nil {
		t.Fatalf("WriteShell: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("expected non-empty HTML output")
	}
}

func TestWriteIndex(t *testing.T) {
	clusters := []IndexCluster{
		{
			Name: "dev",
			Profile: &graph.ClusterProfile{
				ClusterName: "dev",
				Technology:  "flux",
				Graph: graph.Graph{
					Nodes: []graph.Node{{ID: "n1"}},
				},
			},
		},
		{
			Name: "prod",
			Profile: &graph.ClusterProfile{
				ClusterName: "prod",
				Technology:  "argocd",
				Graph: graph.Graph{
					Nodes: []graph.Node{{ID: "n2"}, {ID: "n3"}},
				},
			},
		},
	}
	var buf bytes.Buffer
	if err := WriteIndex(&buf, clusters); err != nil {
		t.Fatalf("WriteIndex: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("expected non-empty HTML output")
	}
}

func TestWriteDiffHTML(t *testing.T) {
	gd := diff.GraphDiff{
		ClusterName: "test",
		Summary:     diff.DiffSummary{Added: 1, Same: 1},
		Nodes: []diff.NodeDiff{
			{Kind: diff.Same, Node: graph.Node{ID: "existing", Type: "infra"}},
			{Kind: diff.Added, Node: graph.Node{ID: "new-app", Type: "apps"}},
		},
	}
	head := &graph.ClusterProfile{
		ClusterName: "test",
		Technology:  "flux",
	}
	var buf bytes.Buffer
	if err := WriteDiffHTML(&buf, gd, nil, head); err != nil {
		t.Fatalf("WriteDiffHTML: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("expected non-empty HTML output")
	}
}
