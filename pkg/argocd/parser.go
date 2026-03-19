// Package argocd parses ArgoCD Application, ApplicationSet and AppProject
// YAML files from a cluster directory and returns a graph.ClusterProfile.
package argocd

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/stuttgart-things/clusterscope/internal/graph"
)

// ── Internal ArgoCD structs ───────────────────────────────────────────────

type application struct {
	Name       string
	Namespace  string
	Project    string
	RepoURL    string
	Path       string
	Revision   string
	DestServer string
	DestNS     string
	// multi-source: first source wins for display
	Sources []appSource
}

type appSource struct {
	RepoURL  string
	Path     string
	Revision string
	Chart    string
}

type applicationSet struct {
	Name      string
	Namespace string
	Project   string
	// generator repoURL (git generator)
	RepoURL  string
	Revision string
	Paths    []string
}

type appProject struct {
	Name        string
	Namespace   string
	Description string
}

type clusterData struct {
	ClusterName     string
	ClusterPath     string
	Applications    []application
	ApplicationSets []applicationSet
	AppProjects     []appProject
}

// ── Public API ────────────────────────────────────────────────────────────

// ParseDir walks an ArgoCD cluster directory and returns a ClusterProfile.
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
			if strings.HasPrefix(base, ".") && path != dir {
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

// ── Profile builder ───────────────────────────────────────────────────────

func buildProfile(data *clusterData) *graph.ClusterProfile {
	g := buildGraph(data)
	meta := map[string]string{
		"applications":    intStr(len(data.Applications)),
		"applicationSets": intStr(len(data.ApplicationSets)),
		"appProjects":     intStr(len(data.AppProjects)),
	}
	return &graph.ClusterProfile{
		ClusterName: data.ClusterName,
		ClusterPath: data.ClusterPath,
		Technology:  "argocd",
		Graph:       g,
		Meta:        meta,
	}
}

func buildGraph(data *clusterData) graph.Graph {
	var g graph.Graph
	nodeIndex := map[string]bool{}

	// Layer 0: AppProjects (or a default "default" project node if none found)
	projects := data.AppProjects
	if len(projects) == 0 && (len(data.Applications) > 0 || len(data.ApplicationSets) > 0) {
		// synthesise a virtual "default" project node
		projects = []appProject{{Name: "default"}}
	}
	sort.Slice(projects, func(i, j int) bool { return projects[i].Name < projects[j].Name })
	for _, p := range projects {
		sub := p.Description
		if sub == "" {
			sub = "AppProject"
		}
		g.Nodes = append(g.Nodes, graph.Node{
			ID:         "project:" + p.Name,
			Layer:      0,
			Type:       "argoproject",
			Sub:        sub,
			Technology: "argocd",
		})
		nodeIndex["project:"+p.Name] = true
	}

	// Layer 1: ApplicationSets
	sort.Slice(data.ApplicationSets, func(i, j int) bool {
		return data.ApplicationSets[i].Name < data.ApplicationSets[j].Name
	})
	for _, as := range data.ApplicationSets {
		repoURL := as.RepoURL
		pathsStr := strings.Join(as.Paths, ", ")
		sub := pathsStr
		if sub == "" {
			sub = as.Revision
		}
		g.Nodes = append(g.Nodes, graph.Node{
			ID:         as.Name,
			Layer:      1,
			Type:       "argoappset",
			Sub:        sub,
			Path:       repoURL,
			Version:    as.Revision,
			Technology: "argocd",
		})
		nodeIndex[as.Name] = true

		projKey := "project:" + as.Project
		if as.Project == "" {
			projKey = "project:default"
		}
		if nodeIndex[projKey] {
			g.Edges = append(g.Edges, graph.Edge{S: projKey, T: as.Name, Kind: "manages"})
		}
	}

	// Layer 2: Applications
	sort.Slice(data.Applications, func(i, j int) bool {
		return data.Applications[i].Name < data.Applications[j].Name
	})
	for _, app := range data.Applications {
		repoURL := app.RepoURL
		path := app.Path
		revision := app.Revision
		if len(app.Sources) > 0 {
			repoURL = app.Sources[0].RepoURL
			path = app.Sources[0].Path
			revision = app.Sources[0].Revision
			if path == "" {
				path = app.Sources[0].Chart
			}
		}
		sub := path
		if sub == "" {
			sub = revision
		}
		g.Nodes = append(g.Nodes, graph.Node{
			ID:         app.Name,
			Layer:      2,
			Type:       "argoapp",
			Sub:        sub,
			Source:     repoURL,
			Path:       path,
			Version:    revision,
			Domain:     app.DestNS,
			Technology: "argocd",
		})
		nodeIndex[app.Name] = true

		projKey := "project:" + app.Project
		if app.Project == "" {
			projKey = "project:default"
		}
		if nodeIndex[projKey] {
			g.Edges = append(g.Edges, graph.Edge{S: projKey, T: app.Name, Kind: "manages"})
		}
	}

	return g
}

// ── YAML file parser ──────────────────────────────────────────────────────

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
		// Only handle argoproj.io resources
		apiVersion := nodeStr(doc, "apiVersion")
		if !strings.Contains(apiVersion, "argoproj.io") {
			continue
		}
		switch nodeStr(doc, "kind") {
		case "Application":
			if a := parseApplication(doc); a != nil {
				data.Applications = append(data.Applications, *a)
			}
		case "ApplicationSet":
			if as := parseApplicationSet(doc); as != nil {
				data.ApplicationSets = append(data.ApplicationSets, *as)
			}
		case "AppProject":
			if p := parseAppProject(doc); p != nil {
				data.AppProjects = append(data.AppProjects, *p)
			}
		}
	}
	return nil
}

func parseApplication(raw map[string]yaml.Node) *application {
	meta := extractMeta(raw)
	if meta.Name == "" {
		return nil
	}
	spec := nodeMap(raw, "spec")
	if spec == nil {
		return nil
	}
	app := &application{
		Name:      meta.Name,
		Namespace: meta.Namespace,
		Project:   nodeStr(spec, "project"),
	}
	if src := nodeMap(spec, "source"); src != nil {
		app.RepoURL = nodeStr(src, "repoURL")
		app.Path = nodeStr(src, "path")
		app.Revision = nodeStr(src, "targetRevision")
	}
	if srcs, ok := spec["sources"]; ok && srcs.Kind == yaml.SequenceNode {
		for _, item := range srcs.Content {
			m := mappingToMap(item)
			app.Sources = append(app.Sources, appSource{
				RepoURL:  nodeStr(m, "repoURL"),
				Path:     nodeStr(m, "path"),
				Revision: nodeStr(m, "targetRevision"),
				Chart:    nodeStr(m, "chart"),
			})
		}
	}
	if dst := nodeMap(spec, "destination"); dst != nil {
		app.DestServer = nodeStr(dst, "server")
		app.DestNS = nodeStr(dst, "namespace")
	}
	return app
}

func parseApplicationSet(raw map[string]yaml.Node) *applicationSet {
	meta := extractMeta(raw)
	if meta.Name == "" {
		return nil
	}
	spec := nodeMap(raw, "spec")
	if spec == nil {
		return nil
	}
	as := &applicationSet{
		Name:      meta.Name,
		Namespace: meta.Namespace,
	}
	// Extract project from template
	if tmpl := nodeMap(spec, "template"); tmpl != nil {
		if tSpec := nodeMap(tmpl, "spec"); tSpec != nil {
			as.Project = nodeStr(tSpec, "project")
		}
	}
	// Extract repoURL/revision/paths from git generator
	if gens, ok := spec["generators"]; ok && gens.Kind == yaml.SequenceNode {
		for _, gen := range gens.Content {
			m := mappingToMap(gen)
			if git := nodeMap(m, "git"); git != nil {
				as.RepoURL = nodeStr(git, "repoURL")
				as.Revision = nodeStr(git, "revision")
				if dirs, ok := git["directories"]; ok && dirs.Kind == yaml.SequenceNode {
					for _, d := range dirs.Content {
						dm := mappingToMap(d)
						if p := nodeStr(dm, "path"); p != "" {
							as.Paths = append(as.Paths, p)
						}
					}
				}
			}
		}
	}
	return as
}

func parseAppProject(raw map[string]yaml.Node) *appProject {
	meta := extractMeta(raw)
	if meta.Name == "" {
		return nil
	}
	spec := nodeMap(raw, "spec")
	desc := ""
	if spec != nil {
		desc = nodeStr(spec, "description")
	}
	return &appProject{
		Name:        meta.Name,
		Namespace:   meta.Namespace,
		Description: desc,
	}
}

// ── YAML helpers ──────────────────────────────────────────────────────────

type meta struct{ Name, Namespace string }

func extractMeta(doc map[string]yaml.Node) meta {
	m := nodeMap(doc, "metadata")
	if m == nil {
		return meta{}
	}
	return meta{
		Name:      nodeStr(m, "name"),
		Namespace: nodeStr(m, "namespace"),
	}
}

func nodeStr(m map[string]yaml.Node, key string) string {
	if n, ok := m[key]; ok && n.Kind == yaml.ScalarNode {
		return n.Value
	}
	return ""
}

func nodeMap(m map[string]yaml.Node, key string) map[string]yaml.Node {
	n, ok := m[key]
	if !ok || n.Kind != yaml.MappingNode {
		return nil
	}
	return mappingToMap(&n)
}

func mappingToMap(n *yaml.Node) map[string]yaml.Node {
	out := make(map[string]yaml.Node)
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Kind == yaml.ScalarNode {
			out[n.Content[i].Value] = *n.Content[i+1]
		}
	}
	return out
}

func intStr(n int) string {
	return fmt.Sprintf("%d", n)
}
