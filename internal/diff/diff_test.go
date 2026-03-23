package diff

import (
	"testing"

	"github.com/stuttgart-things/clusterscope/internal/graph"
)

func TestDiff_EmptyGraphs(t *testing.T) {
	gd := Diff(graph.Graph{}, graph.Graph{})
	if len(gd.Nodes) != 0 || len(gd.Edges) != 0 {
		t.Fatalf("expected empty diff, got %d nodes %d edges", len(gd.Nodes), len(gd.Edges))
	}
	if gd.Summary != (DiffSummary{}) {
		t.Fatalf("expected zero summary, got %+v", gd.Summary)
	}
}

func TestDiff_IdenticalGraphs(t *testing.T) {
	g := graph.Graph{
		Nodes: []graph.Node{
			{ID: "cert-manager", Layer: 1, Type: "infra", Path: "./infra/cert-manager", Version: "1.14.0"},
			{ID: "flux-system", Layer: 0, Type: "source", Path: "https://github.com/org/repo"},
		},
		Edges: []graph.Edge{
			{S: "flux-system", T: "cert-manager", Kind: "watches"},
		},
	}
	gd := Diff(g, g)
	if gd.Summary.Added != 0 || gd.Summary.Removed != 0 || gd.Summary.Modified != 0 {
		t.Fatalf("identical graphs should have no changes, got %+v", gd.Summary)
	}
	if gd.Summary.Same != 2 {
		t.Fatalf("expected 2 same nodes, got %d", gd.Summary.Same)
	}
}

func TestDiff_AddedNodes(t *testing.T) {
	base := graph.Graph{
		Nodes: []graph.Node{
			{ID: "cert-manager", Type: "infra"},
		},
	}
	head := graph.Graph{
		Nodes: []graph.Node{
			{ID: "cert-manager", Type: "infra"},
			{ID: "ingress-nginx", Type: "infra"},
			{ID: "monitoring", Type: "apps"},
		},
	}
	gd := Diff(base, head)
	if gd.Summary.Added != 2 {
		t.Fatalf("expected 2 added, got %d", gd.Summary.Added)
	}
	if gd.Summary.Same != 1 {
		t.Fatalf("expected 1 same, got %d", gd.Summary.Same)
	}

	// Added nodes should come first in output.
	if gd.Nodes[0].Kind != Added || gd.Nodes[1].Kind != Added {
		t.Fatalf("expected first two nodes to be Added, got %s %s", gd.Nodes[0].Kind, gd.Nodes[1].Kind)
	}
}

func TestDiff_RemovedNodes(t *testing.T) {
	base := graph.Graph{
		Nodes: []graph.Node{
			{ID: "cert-manager", Type: "infra"},
			{ID: "old-component", Type: "apps"},
		},
	}
	head := graph.Graph{
		Nodes: []graph.Node{
			{ID: "cert-manager", Type: "infra"},
		},
	}
	gd := Diff(base, head)
	if gd.Summary.Removed != 1 {
		t.Fatalf("expected 1 removed, got %d", gd.Summary.Removed)
	}
	var found bool
	for _, nd := range gd.Nodes {
		if nd.Node.ID == "old-component" && nd.Kind == Removed {
			found = true
		}
	}
	if !found {
		t.Fatal("old-component should be marked as removed")
	}
}

func TestDiff_ModifiedNode_VersionChange(t *testing.T) {
	base := graph.Graph{
		Nodes: []graph.Node{
			{ID: "cert-manager", Type: "infra", Version: "1.14.0", Path: "./infra/cert-manager"},
		},
	}
	head := graph.Graph{
		Nodes: []graph.Node{
			{ID: "cert-manager", Type: "infra", Version: "1.15.0", Path: "./infra/cert-manager"},
		},
	}
	gd := Diff(base, head)
	if gd.Summary.Modified != 1 {
		t.Fatalf("expected 1 modified, got %d", gd.Summary.Modified)
	}
	nd := findNode(gd, "cert-manager")
	if nd == nil {
		t.Fatal("cert-manager not found in diff")
	}
	if len(nd.Changes) != 1 || nd.Changes[0].Field != "version" {
		t.Fatalf("expected version change, got %+v", nd.Changes)
	}
	if nd.Changes[0].Old != "1.14.0" || nd.Changes[0].New != "1.15.0" {
		t.Fatalf("wrong version values: %+v", nd.Changes[0])
	}
}

func TestDiff_ModifiedNode_MultipleFields(t *testing.T) {
	base := graph.Graph{
		Nodes: []graph.Node{
			{ID: "app", Type: "apps", Version: "1.0", Path: "./apps/v1", Domain: "old.example.com"},
		},
	}
	head := graph.Graph{
		Nodes: []graph.Node{
			{ID: "app", Type: "apps", Version: "2.0", Path: "./apps/v2", Domain: "new.example.com"},
		},
	}
	gd := Diff(base, head)
	nd := findNode(gd, "app")
	if nd == nil || nd.Kind != Modified {
		t.Fatal("expected app to be modified")
	}
	if len(nd.Changes) != 3 {
		t.Fatalf("expected 3 field changes (path, version, domain), got %d: %+v", len(nd.Changes), nd.Changes)
	}
}

func TestDiff_ModifiedNode_DependsOnChange(t *testing.T) {
	base := graph.Graph{
		Nodes: []graph.Node{
			{ID: "app", Type: "apps", DependsOn: []string{"cert-manager"}},
		},
	}
	head := graph.Graph{
		Nodes: []graph.Node{
			{ID: "app", Type: "apps", DependsOn: []string{"cert-manager", "ingress-nginx"}},
		},
	}
	gd := Diff(base, head)
	nd := findNode(gd, "app")
	if nd == nil || nd.Kind != Modified {
		t.Fatal("expected app to be modified")
	}
	var hasDeps bool
	for _, c := range nd.Changes {
		if c.Field == "dependsOn" {
			hasDeps = true
		}
	}
	if !hasDeps {
		t.Fatal("expected dependsOn field change")
	}
}

func TestDiff_AddedEdge(t *testing.T) {
	base := graph.Graph{
		Edges: []graph.Edge{
			{S: "flux-system", T: "cert-manager", Kind: "watches"},
		},
	}
	head := graph.Graph{
		Edges: []graph.Edge{
			{S: "flux-system", T: "cert-manager", Kind: "watches"},
			{S: "cert-manager", T: "ingress-nginx", Kind: "dependsOn"},
		},
	}
	gd := Diff(base, head)
	var added int
	for _, ed := range gd.Edges {
		if ed.Kind == Added {
			added++
		}
	}
	if added != 1 {
		t.Fatalf("expected 1 added edge, got %d", added)
	}
}

func TestDiff_RemovedEdge(t *testing.T) {
	base := graph.Graph{
		Edges: []graph.Edge{
			{S: "a", T: "b", Kind: "watches"},
			{S: "b", T: "c", Kind: "dependsOn"},
		},
	}
	head := graph.Graph{
		Edges: []graph.Edge{
			{S: "a", T: "b", Kind: "watches"},
		},
	}
	gd := Diff(base, head)
	var removed int
	for _, ed := range gd.Edges {
		if ed.Kind == Removed {
			removed++
		}
	}
	if removed != 1 {
		t.Fatalf("expected 1 removed edge, got %d", removed)
	}
}

func TestDiff_FullScenario(t *testing.T) {
	base := graph.Graph{
		Nodes: []graph.Node{
			{ID: "flux-system", Layer: 0, Type: "source", Path: "https://github.com/org/repo"},
			{ID: "cert-manager", Layer: 1, Type: "infra", Version: "1.14.0"},
			{ID: "old-alerting", Layer: 3, Type: "apps"},
			{ID: "ingress-nginx", Layer: 1, Type: "infra"},
		},
		Edges: []graph.Edge{
			{S: "flux-system", T: "cert-manager", Kind: "watches"},
			{S: "flux-system", T: "old-alerting", Kind: "watches"},
			{S: "flux-system", T: "ingress-nginx", Kind: "watches"},
		},
	}
	head := graph.Graph{
		Nodes: []graph.Node{
			{ID: "flux-system", Layer: 0, Type: "source", Path: "https://github.com/org/repo"},
			{ID: "cert-manager", Layer: 1, Type: "infra", Version: "1.15.0"},
			{ID: "ingress-nginx", Layer: 1, Type: "infra"},
			{ID: "monitoring", Layer: 3, Type: "apps", DependsOn: []string{"cert-manager"}},
		},
		Edges: []graph.Edge{
			{S: "flux-system", T: "cert-manager", Kind: "watches"},
			{S: "flux-system", T: "ingress-nginx", Kind: "watches"},
			{S: "flux-system", T: "monitoring", Kind: "watches"},
			{S: "cert-manager", T: "monitoring", Kind: "dependsOn"},
		},
	}

	gd := Diff(base, head)

	if gd.Summary.Added != 1 {
		t.Errorf("expected 1 added (monitoring), got %d", gd.Summary.Added)
	}
	if gd.Summary.Removed != 1 {
		t.Errorf("expected 1 removed (old-alerting), got %d", gd.Summary.Removed)
	}
	if gd.Summary.Modified != 1 {
		t.Errorf("expected 1 modified (cert-manager version bump), got %d", gd.Summary.Modified)
	}
	if gd.Summary.Same != 2 {
		t.Errorf("expected 2 same (flux-system, ingress-nginx), got %d", gd.Summary.Same)
	}

	var addedEdges, removedEdges int
	for _, ed := range gd.Edges {
		switch ed.Kind {
		case Added:
			addedEdges++
		case Removed:
			removedEdges++
		}
	}
	if addedEdges != 2 {
		t.Errorf("expected 2 added edges, got %d", addedEdges)
	}
	if removedEdges != 1 {
		t.Errorf("expected 1 removed edge, got %d", removedEdges)
	}
}

func TestDiff_SubstituteChange(t *testing.T) {
	base := graph.Graph{
		Nodes: []graph.Node{
			{ID: "app", Type: "apps", Substitute: map[string]string{"DOMAIN": "old.io", "VERSION": "1.0"}},
		},
	}
	head := graph.Graph{
		Nodes: []graph.Node{
			{ID: "app", Type: "apps", Substitute: map[string]string{"DOMAIN": "new.io", "VERSION": "1.0"}},
		},
	}
	gd := Diff(base, head)
	nd := findNode(gd, "app")
	if nd == nil || nd.Kind != Modified {
		t.Fatal("expected app to be modified due to substitute change")
	}
}

// ── test helpers ─────────────────────────────────────────────────────────────

func findNode(gd GraphDiff, id string) *NodeDiff {
	for i := range gd.Nodes {
		if gd.Nodes[i].Node.ID == id {
			return &gd.Nodes[i]
		}
	}
	return nil
}
