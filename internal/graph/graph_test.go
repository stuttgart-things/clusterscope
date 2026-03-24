package graph

import (
	"encoding/json"
	"testing"
)

func TestNode_JSONRoundTrip(t *testing.T) {
	n := Node{
		ID:         "cert-manager",
		Layer:      1,
		Type:       "infra",
		Sub:        "v1.14.0",
		Source:     "flux-system",
		Path:       "./infra/cert-manager",
		Version:    "1.14.0",
		Interval:   "5m",
		Domain:     "example.com",
		DependsOn:  []string{"nfs-csi"},
		Substitute: map[string]string{"VERSION": "1.14.0"},
		Technology: "flux",
		Status:     "ready",
		Message:    "Applied revision: main@sha1:abc",
		UpdatedAt:  "2025-01-01T00:00:00Z",
	}
	b, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Node
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != n.ID || got.Layer != n.Layer || got.Type != n.Type {
		t.Fatalf("basic fields mismatch: got %+v", got)
	}
	if len(got.DependsOn) != 1 || got.DependsOn[0] != "nfs-csi" {
		t.Fatalf("DependsOn mismatch: %v", got.DependsOn)
	}
	if got.Substitute["VERSION"] != "1.14.0" {
		t.Fatalf("Substitute mismatch: %v", got.Substitute)
	}
	if got.Status != "ready" || got.Message == "" || got.UpdatedAt == "" {
		t.Fatalf("live fields mismatch: %+v", got)
	}
}

func TestNode_OmitEmpty(t *testing.T) {
	n := Node{
		ID:         "simple",
		Layer:      0,
		Type:       "source",
		Technology: "flux",
	}
	b, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	raw := string(b)
	for _, field := range []string{"dependsOn", "substitute", "status", "message", "updatedAt"} {
		if contains(raw, `"`+field+`"`) {
			t.Errorf("expected %q to be omitted, got: %s", field, raw)
		}
	}
}

func TestEdge_JSON(t *testing.T) {
	e := Edge{S: "flux-system", T: "cert-manager", Kind: "watches"}
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Edge
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got != e {
		t.Fatalf("edge mismatch: got %+v, want %+v", got, e)
	}
}

func TestGraph_JSONRoundTrip(t *testing.T) {
	g := Graph{
		Nodes: []Node{
			{ID: "flux-system", Layer: 0, Type: "source", Technology: "flux"},
			{ID: "cert-manager", Layer: 1, Type: "infra", Technology: "flux"},
		},
		Edges: []Edge{
			{S: "flux-system", T: "cert-manager", Kind: "watches"},
		},
	}
	b, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Graph
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(got.Nodes))
	}
	if len(got.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(got.Edges))
	}
	if got.Edges[0].Kind != "watches" {
		t.Fatalf("edge kind mismatch: %s", got.Edges[0].Kind)
	}
}

func TestClusterProfile_Fields(t *testing.T) {
	p := ClusterProfile{
		ClusterName: "dev-cluster",
		ClusterPath: "/clusters/dev",
		Technology:  "flux",
		Graph: Graph{
			Nodes: []Node{{ID: "n1"}},
		},
		Meta: map[string]string{"fluxVersion": "2.3.0"},
	}
	if p.ClusterName != "dev-cluster" {
		t.Fatalf("ClusterName: %s", p.ClusterName)
	}
	if p.Meta["fluxVersion"] != "2.3.0" {
		t.Fatalf("Meta: %v", p.Meta)
	}
	if len(p.Graph.Nodes) != 1 {
		t.Fatalf("Graph.Nodes: %d", len(p.Graph.Nodes))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
