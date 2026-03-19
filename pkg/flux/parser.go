// Package flux parses FluxCD YAML files (Kustomization, GitRepository,
// FluxInstance) from a cluster directory and returns a graph.ClusterProfile.
package flux

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/stuttgart-things/clusterscope/internal/graph"
)

// ── Internal Flux structs ──────────────────────────────────────────────────

type kustomization struct {
	Name       string
	Namespace  string
	Source     string
	Path       string
	Interval   string
	DependsOn  []string
	Substitute map[string]string
	Domain     string
	Version    string
	Layer      string
}

type gitRepository struct {
	Name     string
	URL      string
	Branch   string
	Interval string
}

type fluxInstance struct {
	Version    string
	Components []string
	SyncURL    string
	SyncPath   string
	SyncRef    string
	SOPS       bool
}

type clusterData struct {
	ClusterName    string
	ClusterPath    string
	Kustomizations []kustomization
	GitRepos       []gitRepository
	FluxInstance   *fluxInstance
}

// ── Public API ─────────────────────────────────────────────────────────────

// ParseDir walks a Flux cluster directory and returns a ClusterProfile.
func ParseDir(dir string) (*graph.ClusterProfile, error) {
	data := &clusterData{
		ClusterName: filepath.Base(dir),
		ClusterPath: dir,
	}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if (strings.HasPrefix(base, ".") || base == "vault") && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}
		return parseFile(path, data)
	})
	if err != nil {
		return nil, err
	}
	return buildProfile(data), nil
}

// ── Profile builder ────────────────────────────────────────────────────────

func buildProfile(data *clusterData) *graph.ClusterProfile {
	g := buildGraph(data)

	meta := map[string]string{}
	if data.FluxInstance != nil {
		meta["fluxVersion"] = data.FluxInstance.Version
		meta["fluxSyncURL"] = data.FluxInstance.SyncURL
		meta["fluxSyncPath"] = data.FluxInstance.SyncPath
		meta["fluxSyncRef"] = data.FluxInstance.SyncRef
		if data.FluxInstance.SOPS {
			meta["sops"] = "true"
		}
		if len(data.FluxInstance.Components) > 0 {
			if b, err := json.Marshal(data.FluxInstance.Components); err == nil {
				meta["fluxComponents"] = string(b)
			}
		}
	}

	return &graph.ClusterProfile{
		ClusterName: data.ClusterName,
		ClusterPath: data.ClusterPath,
		Technology:  "flux",
		Graph:       g,
		Meta:        meta,
	}
}

func buildGraph(data *clusterData) graph.Graph {
	var g graph.Graph
	nodeIndex := map[string]bool{}

	// Git source nodes (layer 0)
	for _, gr := range data.GitRepos {
		g.Nodes = append(g.Nodes, graph.Node{
			ID:         gr.Name,
			Layer:      0,
			Type:       "source",
			Sub:        gr.Branch + " · " + gr.Interval,
			Path:       gr.URL,
			Technology: "flux",
		})
		nodeIndex[gr.Name] = true
	}
	if data.FluxInstance != nil {
		g.Nodes = append(g.Nodes, graph.Node{
			ID:         "sync-root",
			Layer:      0,
			Type:       "source",
			Sub:        data.FluxInstance.SyncRef,
			Path:       data.FluxInstance.SyncURL,
			Technology: "flux",
		})
		nodeIndex["sync-root"] = true
	}

	// Split infra into base (no deps) and dep infra
	var baseInfra, depInfra, appNodes, homerunNodes []kustomization
	for _, k := range data.Kustomizations {
		switch k.Layer {
		case "infra":
			if len(k.DependsOn) == 0 {
				baseInfra = append(baseInfra, k)
			} else {
				depInfra = append(depInfra, k)
			}
		case "homerun2", "profile":
			homerunNodes = append(homerunNodes, k)
		default:
			appNodes = append(appNodes, k)
		}
	}

	addNodes := func(ks []kustomization, layer int) {
		sorted := make([]kustomization, len(ks))
		copy(sorted, ks)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

		for _, k := range sorted {
			sub := k.Version
			if sub == "" {
				sub = k.Source
			}
			g.Nodes = append(g.Nodes, graph.Node{
				ID:         k.Name,
				Layer:      layer,
				Type:       k.Layer,
				Sub:        sub,
				Source:     k.Source,
				Path:       k.Path,
				Version:    k.Version,
				Interval:   k.Interval,
				Domain:     k.Domain,
				DependsOn:  k.DependsOn,
				Substitute: sanitizeSubstitute(k.Substitute),
				Technology: "flux",
			})
			nodeIndex[k.Name] = true

			if k.Source != "" && nodeIndex[k.Source] {
				g.Edges = append(g.Edges, graph.Edge{S: k.Source, T: k.Name, Kind: "watches"})
			}
			for _, dep := range k.DependsOn {
				if nodeIndex[dep] {
					g.Edges = append(g.Edges, graph.Edge{S: dep, T: k.Name, Kind: "dependsOn"})
				}
			}
		}
	}

	addNodes(baseInfra, 1)
	addNodes(depInfra, 2)
	addNodes(appNodes, 3)
	addNodes(homerunNodes, 4)

	// Second pass: add any missing edges
	knownEdges := map[string]bool{}
	for _, e := range g.Edges {
		knownEdges[e.S+"|"+e.T] = true
	}
	for _, n := range g.Nodes {
		if n.Source != "" && nodeIndex[n.Source] {
			key := n.Source + "|" + n.ID
			if !knownEdges[key] {
				g.Edges = append(g.Edges, graph.Edge{S: n.Source, T: n.ID, Kind: "watches"})
				knownEdges[key] = true
			}
		}
		for _, dep := range n.DependsOn {
			key := dep + "|" + n.ID
			if nodeIndex[dep] && !knownEdges[key] {
				g.Edges = append(g.Edges, graph.Edge{S: dep, T: n.ID, Kind: "dependsOn"})
				knownEdges[key] = true
			}
		}
	}

	return g
}

func sanitizeSubstitute(sub map[string]string) map[string]string {
	clean := make(map[string]string)
	for k, v := range sub {
		if !strings.HasPrefix(v, "ENC[") {
			clean[k] = v
		}
	}
	return clean
}

// ── YAML file parser ───────────────────────────────────────────────────────

func parseFile(path string, data *clusterData) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	for {
		var doc map[string]yaml.Node
		if err := decoder.Decode(&doc); err != nil {
			break
		}
		if doc == nil {
			continue
		}
		switch nodeStr(doc, "kind") {
		case "Kustomization":
			if k := parseKustomization(doc); k != nil {
				data.Kustomizations = append(data.Kustomizations, *k)
			}
		case "GitRepository":
			if g := parseGitRepo(doc); g != nil {
				data.GitRepos = append(data.GitRepos, *g)
			}
		case "FluxInstance":
			if fi := parseFluxInstance(doc); fi != nil {
				data.FluxInstance = fi
			}
		}
	}
	return nil
}

func parseKustomization(raw map[string]yaml.Node) *kustomization {
	meta := extractMeta(raw)
	spec := nodeMap(raw, "spec")
	if spec == nil {
		return nil
	}
	k := &kustomization{
		Name:       meta.Name,
		Namespace:  meta.Namespace,
		Interval:   nodeStr(spec, "interval"),
		Path:       nodeStr(spec, "path"),
		Substitute: make(map[string]string),
	}
	if sr := nodeMap(spec, "sourceRef"); sr != nil {
		k.Source = nodeStr(sr, "name")
	}
	if deps, ok := spec["dependsOn"]; ok && deps.Kind == yaml.SequenceNode {
		for _, item := range deps.Content {
			m := mappingToMap(item)
			if name := nodeStr(m, "name"); name != "" {
				k.DependsOn = append(k.DependsOn, name)
			}
		}
	}
	if pb := nodeMap(spec, "postBuild"); pb != nil {
		if sub := nodeMap(pb, "substitute"); sub != nil {
			for key, val := range sub {
				k.Substitute[key] = val.Value
			}
		}
	}
	k.Domain = k.Substitute["DOMAIN"]
	for _, vkey := range []string{
		"VERSION", "CLAIM_MACHINERY_API_VERSION", "VAULT_VERSION",
		"PROMETHEUS_VERSION", "CERT_MANAGER_VERSION", "NFS_CSI_VERSION",
		"TRUST_MANAGER_VERSION", "UPTIME_KUMA_VERSION", "FLUX_WEB_VERSION",
		"HEADLAMP_VERSION", "HOMEPAGE_VERSION", "CLUSTERBOOK_VERSION",
		"MACHINERY_VERSION", "HOMERUN2_OMNI_PITCHER_VERSION",
	} {
		if v := k.Substitute[vkey]; v != "" {
			k.Version = v
			break
		}
	}
	k.Layer = classifyLayer(k.Path, k.Name)
	return k
}

func parseGitRepo(raw map[string]yaml.Node) *gitRepository {
	meta := extractMeta(raw)
	spec := nodeMap(raw, "spec")
	if spec == nil {
		return nil
	}
	g := &gitRepository{
		Name:     meta.Name,
		Interval: nodeStr(spec, "interval"),
		URL:      nodeStr(spec, "url"),
	}
	if ref := nodeMap(spec, "ref"); ref != nil {
		g.Branch = nodeStr(ref, "branch")
	}
	return g
}

func parseFluxInstance(raw map[string]yaml.Node) *fluxInstance {
	spec := nodeMap(raw, "spec")
	if spec == nil {
		return nil
	}
	fi := &fluxInstance{}
	if dist := nodeMap(spec, "distribution"); dist != nil {
		fi.Version = nodeStr(dist, "version")
	}
	if sync := nodeMap(spec, "sync"); sync != nil {
		fi.SyncURL = nodeStr(sync, "url")
		fi.SyncPath = nodeStr(sync, "path")
		fi.SyncRef = nodeStr(sync, "ref")
	}
	if comps, ok := spec["components"]; ok && comps.Kind == yaml.SequenceNode {
		for _, c := range comps.Content {
			fi.Components = append(fi.Components, c.Value)
		}
	}
	if kust := nodeMap(spec, "kustomize"); kust != nil {
		if patches, ok := kust["patches"]; ok {
			b, _ := yaml.Marshal(patches)
			fi.SOPS = strings.Contains(string(b), "sops")
		}
	}
	return fi
}

func classifyLayer(path, name string) string {
	switch {
	case strings.Contains(path, "/infra/") || strings.Contains(path, "/infra"):
		return "infra"
	case strings.Contains(path, "/apps/homerun") || strings.Contains(name, "homerun"):
		return "homerun2"
	case strings.Contains(name, "profile") || strings.Contains(name, "alert"):
		return "profile"
	default:
		return "apps"
	}
}

// ── yaml.Node helpers ──────────────────────────────────────────────────────

type metaResult struct{ Name, Namespace string }

func extractMeta(raw map[string]yaml.Node) metaResult {
	m := nodeMap(raw, "metadata")
	if m == nil {
		return metaResult{}
	}
	return metaResult{Name: nodeStr(m, "name"), Namespace: nodeStr(m, "namespace")}
}

func nodeStr(m map[string]yaml.Node, key string) string {
	if n, ok := m[key]; ok {
		return n.Value
	}
	return ""
}

func nodeMap(m map[string]yaml.Node, key string) map[string]yaml.Node {
	n, ok := m[key]
	if !ok {
		return nil
	}
	return mappingToMap(&n)
}

func mappingToMap(n *yaml.Node) map[string]yaml.Node {
	if n.Kind == yaml.AliasNode {
		return mappingToMap(n.Alias)
	}
	if n.Kind != yaml.MappingNode {
		return nil
	}
	result := make(map[string]yaml.Node)
	for i := 0; i+1 < len(n.Content); i += 2 {
		result[n.Content[i].Value] = *n.Content[i+1]
	}
	return result
}
