package diff

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stuttgart-things/clusterscope/internal/graph"
)

func TestWriteMarkdown_NoChanges(t *testing.T) {
	gd := Diff(
		graph.Graph{Nodes: []graph.Node{{ID: "a", Type: "infra"}}},
		graph.Graph{Nodes: []graph.Node{{ID: "a", Type: "infra"}}},
	)
	gd.ClusterName = "prod"

	var buf bytes.Buffer
	if err := WriteMarkdown(&buf, gd); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "No changes detected") {
		t.Error("expected 'No changes detected' for identical graphs")
	}
}

func TestWriteMarkdown_FullScenario(t *testing.T) {
	base := graph.Graph{
		Nodes: []graph.Node{
			{ID: "cert-manager", Type: "infra", Version: "1.14.0"},
			{ID: "old-app", Type: "apps"},
			{ID: "ingress", Type: "infra"},
		},
		Edges: []graph.Edge{
			{S: "cert-manager", T: "old-app", Kind: "dependsOn"},
		},
	}
	head := graph.Graph{
		Nodes: []graph.Node{
			{ID: "cert-manager", Type: "infra", Version: "1.15.0"},
			{ID: "ingress", Type: "infra"},
			{ID: "monitoring", Type: "apps"},
		},
		Edges: []graph.Edge{
			{S: "cert-manager", T: "monitoring", Kind: "dependsOn"},
		},
	}
	gd := Diff(base, head)
	gd.ClusterName = "staging"

	var buf bytes.Buffer
	if err := WriteMarkdown(&buf, gd); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	checks := []string{
		"## Clusterscope Diff: staging",
		"**+1** added",
		"**-1** removed",
		"**~1** modified",
		"### Added",
		"`monitoring`",
		"### Removed",
		"~~`old-app`~~",
		"### Modified",
		"`cert-manager`",
		"**version**: `1.14.0` → `1.15.0`",
		"### Edge Changes",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in markdown output:\n%s", want, out)
		}
	}
}
