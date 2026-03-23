// Package diff computes structural differences between two cluster graphs.
// It is a pure function with no I/O — feed it two graph.Graph values and it
// returns a GraphDiff describing added, removed, and modified nodes and edges.
package diff

import (
	"sort"
	"strings"

	"github.com/stuttgart-things/clusterscope/internal/graph"
)

// DiffKind classifies how a node or edge changed between base and head.
type DiffKind string

const (
	Added    DiffKind = "added"
	Removed  DiffKind = "removed"
	Modified DiffKind = "modified"
	Same     DiffKind = "same"
)

// FieldChange records a single field-level difference on a modified node.
type FieldChange struct {
	Field string `json:"field"`
	Old   string `json:"old"`
	New   string `json:"new"`
}

// NodeDiff pairs a node with its diff classification and optional field changes.
type NodeDiff struct {
	Node    graph.Node    `json:"node"`
	Kind    DiffKind      `json:"kind"`
	Changes []FieldChange `json:"changes,omitempty"`
}

// EdgeDiff pairs an edge with its diff classification.
type EdgeDiff struct {
	Edge graph.Edge `json:"edge"`
	Kind DiffKind   `json:"kind"`
}

// DiffSummary holds aggregate counts for a graph diff.
type DiffSummary struct {
	Added    int `json:"added"`
	Removed  int `json:"removed"`
	Modified int `json:"modified"`
	Same     int `json:"same"`
}

// GraphDiff is the full diff result for a single cluster.
type GraphDiff struct {
	ClusterName string      `json:"clusterName"`
	Nodes       []NodeDiff  `json:"nodes"`
	Edges       []EdgeDiff  `json:"edges"`
	Summary     DiffSummary `json:"summary"`
}

// Diff compares a base graph (current main branch) against a head graph
// (PR branch) and returns a GraphDiff describing all changes.
func Diff(base, head graph.Graph) GraphDiff {
	var gd GraphDiff
	gd.Nodes = diffNodes(base.Nodes, head.Nodes)
	gd.Edges = diffEdges(base.Edges, head.Edges)
	for _, nd := range gd.Nodes {
		switch nd.Kind {
		case Added:
			gd.Summary.Added++
		case Removed:
			gd.Summary.Removed++
		case Modified:
			gd.Summary.Modified++
		case Same:
			gd.Summary.Same++
		}
	}
	return gd
}

// ── node diffing ─────────────────────────────────────────────────────────────

func diffNodes(base, head []graph.Node) []NodeDiff {
	baseMap := indexNodes(base)
	headMap := indexNodes(head)

	var result []NodeDiff

	// Removed or modified: iterate base nodes.
	for id, bNode := range baseMap {
		hNode, exists := headMap[id]
		if !exists {
			result = append(result, NodeDiff{Node: bNode, Kind: Removed})
			continue
		}
		changes := compareNodes(bNode, hNode)
		if len(changes) > 0 {
			result = append(result, NodeDiff{Node: hNode, Kind: Modified, Changes: changes})
		} else {
			result = append(result, NodeDiff{Node: hNode, Kind: Same})
		}
	}

	// Added: nodes in head that are not in base.
	for id, hNode := range headMap {
		if _, exists := baseMap[id]; !exists {
			result = append(result, NodeDiff{Node: hNode, Kind: Added})
		}
	}

	// Stable output order: added/removed/modified first (by ID), then same.
	sort.Slice(result, func(i, j int) bool {
		oi, oj := kindOrder(result[i].Kind), kindOrder(result[j].Kind)
		if oi != oj {
			return oi < oj
		}
		return result[i].Node.ID < result[j].Node.ID
	})

	return result
}

func compareNodes(base, head graph.Node) []FieldChange {
	var changes []FieldChange
	check := func(field, old, new string) {
		if old != new {
			changes = append(changes, FieldChange{Field: field, Old: old, New: new})
		}
	}
	check("path", base.Path, head.Path)
	check("version", base.Version, head.Version)
	check("source", base.Source, head.Source)
	check("interval", base.Interval, head.Interval)
	check("domain", base.Domain, head.Domain)
	check("type", base.Type, head.Type)
	check("sub", base.Sub, head.Sub)

	if d := diffStringSlice(base.DependsOn, head.DependsOn); d != "" {
		changes = append(changes, FieldChange{Field: "dependsOn", Old: sliceStr(base.DependsOn), New: sliceStr(head.DependsOn)})
	}
	if d := diffStringMap(base.Substitute, head.Substitute); d != "" {
		changes = append(changes, FieldChange{Field: "substitute", Old: d, New: mapStr(head.Substitute)})
	}
	return changes
}

// ── edge diffing ─────────────────────────────────────────────────────────────

func diffEdges(base, head []graph.Edge) []EdgeDiff {
	baseSet := indexEdges(base)
	headSet := indexEdges(head)

	var result []EdgeDiff
	for key, e := range baseSet {
		if _, exists := headSet[key]; !exists {
			result = append(result, EdgeDiff{Edge: e, Kind: Removed})
		} else {
			result = append(result, EdgeDiff{Edge: e, Kind: Same})
		}
	}
	for key, e := range headSet {
		if _, exists := baseSet[key]; !exists {
			result = append(result, EdgeDiff{Edge: e, Kind: Added})
		}
	}

	sort.Slice(result, func(i, j int) bool {
		oi, oj := kindOrder(result[i].Kind), kindOrder(result[j].Kind)
		if oi != oj {
			return oi < oj
		}
		ki := result[i].Edge.S + "|" + result[i].Edge.T + "|" + result[i].Edge.Kind
		kj := result[j].Edge.S + "|" + result[j].Edge.T + "|" + result[j].Edge.Kind
		return ki < kj
	})

	return result
}

// ── helpers ──────────────────────────────────────────────────────────────────

func indexNodes(nodes []graph.Node) map[string]graph.Node {
	m := make(map[string]graph.Node, len(nodes))
	for _, n := range nodes {
		m[n.ID] = n
	}
	return m
}

func edgeKey(e graph.Edge) string {
	return e.S + "|" + e.T + "|" + e.Kind
}

func indexEdges(edges []graph.Edge) map[string]graph.Edge {
	m := make(map[string]graph.Edge, len(edges))
	for _, e := range edges {
		m[edgeKey(e)] = e
	}
	return m
}

func kindOrder(k DiffKind) int {
	switch k {
	case Added:
		return 0
	case Removed:
		return 1
	case Modified:
		return 2
	case Same:
		return 3
	default:
		return 4
	}
}

func diffStringSlice(a, b []string) string {
	if len(a) != len(b) {
		return "changed"
	}
	sa := make([]string, len(a))
	sb := make([]string, len(b))
	copy(sa, a)
	copy(sb, b)
	sort.Strings(sa)
	sort.Strings(sb)
	for i := range sa {
		if sa[i] != sb[i] {
			return "changed"
		}
	}
	return ""
}

func diffStringMap(a, b map[string]string) string {
	if len(a) != len(b) {
		return mapStr(a)
	}
	for k, v := range a {
		if b[k] != v {
			return mapStr(a)
		}
	}
	return ""
}

func sliceStr(s []string) string {
	if len(s) == 0 {
		return ""
	}
	sorted := make([]string, len(s))
	copy(sorted, s)
	sort.Strings(sorted)
	return strings.Join(sorted, ", ")
}

func mapStr(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, len(keys))
	for i, k := range keys {
		pairs[i] = k + "=" + m[k]
	}
	return strings.Join(pairs, ", ")
}
