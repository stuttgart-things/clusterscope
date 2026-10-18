// Package scan discovers cluster directories and auto-detects their technology.
package scan

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/stuttgart-things/clusterscope/internal/graph"
	"github.com/stuttgart-things/clusterscope/pkg/argocd"
	"github.com/stuttgart-things/clusterscope/pkg/flux"
	"sigs.k8s.io/yaml"
)

// ClusterEntry holds a parsed cluster profile together with scan metadata.
type ClusterEntry struct {
	Name    string
	Profile *graph.ClusterProfile
	Err     error
}

// Root walks root, discovers all immediate subdirectories that contain YAML
// files, auto-detects their technology, and returns one ClusterEntry per dir.
// If no subdir-clusters are found but root itself has YAML files, root is
// treated as a single cluster (e.g. pointing directly at a cluster dir).
func Root(root string) ([]ClusterEntry, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var results []ClusterEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if !HasYAML(dir) {
			continue
		}

		tech := DetectTech(dir)
		var profile *graph.ClusterProfile
		var pErr error

		switch tech {
		case "argocd":
			profile, pErr = argocd.ParseDir(dir)
		default:
			profile, pErr = flux.ParseDir(dir)
		}

		results = append(results, ClusterEntry{
			Name:    e.Name(),
			Profile: profile,
			Err:     pErr,
		})
	}

	// No subdir-clusters found — treat root itself as a single cluster.
	if len(results) == 0 && HasYAML(root) {
		tech := DetectTech(root)
		var profile *graph.ClusterProfile
		var pErr error
		switch tech {
		case "argocd":
			profile, pErr = argocd.ParseDir(root)
		default:
			profile, pErr = flux.ParseDir(root)
		}
		return []ClusterEntry{{
			Name:    filepath.Base(root),
			Profile: profile,
			Err:     pErr,
		}}, nil
	}

	return results, nil
}

// DetectTech returns "argocd" when any .yaml file in dir contains ArgoCD
// markers, otherwise returns "flux".
func DetectTech(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "flux"
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		content := string(data)
		if strings.Contains(content, "argoproj.io") ||
			strings.Contains(content, "kind: Application") {
			return "argocd"
		}
	}
	return "flux"
}

// HasYAML returns true if dir contains at least one .yaml/.yml file.
func HasYAML(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			n := e.Name()
			if strings.HasSuffix(n, ".yaml") || strings.HasSuffix(n, ".yml") {
				return true
			}
		}
	}
	return false
}

// ClusterDir is a discovered cluster directory.
type ClusterDir struct {
	Name string // cluster name: the directory name unless overridden
	Dir  string // directory name
	Path string
	Rel  string // display path: [<root name>/]<path relative to the root>
	Lab  string
	// Duplicate is set when another directory with the same cluster name was
	// found first (in root and path order); duplicates are not scanned.
	Duplicate bool
}

// RootSpec is one directory tree to discover clusters in (typically one
// git-synced repository).
type RootSpec struct {
	Path string `json:"path"`
	// Name prefixes the display path of its clusters (e.g. the repo name).
	Name string `json:"name,omitempty"`
	// Lab is the lab of its clusters unless overridden per cluster.
	Lab string `json:"lab,omitempty"`
	// Include limits the root to these directory names (default: all).
	Include []string `json:"include,omitempty"`
}

// Override sets the cluster name and/or lab of one cluster directory, for
// repos whose directory names differ from the cluster names.
type Override struct {
	Name string `json:"name,omitempty"`
	Lab  string `json:"lab,omitempty"`
}

// Config is the -config file: several roots plus per-directory overrides.
//
//	roots:
//	  - path: /data/stuttgart-things/clusters/labul
//	  - path: /data/harvester/clusters
//	    name: harvester
//	    include: [edge-test2]
//	clusters:
//	  edge-test2: {name: edge-tt-test2, lab: labul}
type Config struct {
	Roots    []RootSpec          `json:"roots"`
	Clusters map[string]Override `json:"clusters,omitempty"`
	// Clusterbooks are further cluster sources: IP / DNS allocations (#60).
	Clusterbooks []ClusterbookSpec `json:"clusterbooks,omitempty"`
	// Machineries are further cluster sources: the ClusterStack orders of a
	// machinery cluster (#60).
	Machineries []MachinerySpec `json:"machineries,omitempty"`
}

// MachinerySpec is one machinery cluster whose ClusterStack orders belong to
// a lab. Cluster names its kubeconfig (-kubeconfig-dir / -kubeconfig-map).
// Environments keeps only the orders whose spec.environmentConfig is listed
// -- one machinery builds for several labs; empty keeps every order.
type MachinerySpec struct {
	Cluster      string   `json:"cluster"`
	Lab          string   `json:"lab,omitempty"`
	Environments []string `json:"environments,omitempty"`
}

// ClusterbookSpec is one clusterbook REST API whose allocations belong to a
// lab. Names maps clusterbook names to cluster names where they differ and
// the allocated domain does not identify the cluster (e.g. no routes yet).
type ClusterbookSpec struct {
	URL   string            `json:"url"`
	Lab   string            `json:"lab,omitempty"`
	Names map[string]string `json:"names,omitempty"`
}

// LoadConfig reads a -config file (YAML or JSON).
func LoadConfig(path string) (Config, error) {
	var c Config
	raw, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return c, err
	}
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return c, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(c.Roots) == 0 {
		return c, fmt.Errorf("%s: no roots", path)
	}
	for i, r := range c.Roots {
		if r.Path == "" {
			return c, fmt.Errorf("%s: roots[%d] has no path", path, i)
		}
	}
	for i, b := range c.Clusterbooks {
		if b.URL == "" {
			return c, fmt.Errorf("%s: clusterbooks[%d] has no url", path, i)
		}
	}
	for i, m := range c.Machineries {
		if m.Cluster == "" {
			return c, fmt.Errorf("%s: machineries[%d] has no cluster", path, i)
		}
	}
	return c, nil
}

// MaxDepth limits how deep Discover descends below root.
const MaxDepth = 4

// DefaultLab is used when no lab can be derived from the path.
const DefaultLab = "default"

// Discover finds the cluster directories below one root; see DiscoverAll.
func Discover(root, lab string) []ClusterDir {
	return DiscoverAll(Config{Roots: []RootSpec{{Path: root}}}, lab)
}

// DiscoverAll finds the cluster directories of every root, in root order.
// Per root:
//
//   - its immediate subdirectories contain YAML → each of them is a
//     cluster (flat layout)
//   - the root itself contains YAML → it is the only cluster
//   - otherwise directories are searched recursively (up to MaxDepth): a
//     directory with YAML files is a cluster, one without is descended into.
//     This covers repo layouts like clusters/<lab>/<platform>/<cluster>.
//
// A missing root is skipped (e.g. not yet cloned by git-sync).
//
// Lab, in order: the cluster's override, the root's lab, the path segment
// after "clusters" when a further segment follows before the cluster
// directory (…/clusters/labul/vsphere/platform-sthings → labul), lab (or
// DefaultLab when empty). Hidden directories are skipped.
func DiscoverAll(cfg Config, lab string) []ClusterDir {
	if lab == "" {
		lab = DefaultLab
	}
	seen := make(map[string]bool)
	var out []ClusterDir
	for _, root := range cfg.Roots {
		include := make(map[string]bool, len(root.Include))
		for _, n := range root.Include {
			include[n] = true
		}
		fallback := lab
		abs, err := filepath.Abs(root.Path)
		if err != nil {
			abs = root.Path
		}
		for _, d := range clusterDirs(root.Path) {
			dir := filepath.Base(d)
			if len(include) > 0 && !include[dir] {
				continue
			}
			rel, _ := filepath.Rel(root.Path, d)
			cd := ClusterDir{Name: dir, Dir: dir, Path: d, Rel: rel}
			if root.Name != "" {
				cd.Rel = root.Name + "/" + rel
			}
			ov := cfg.Clusters[dir]
			switch {
			case ov.Lab != "":
				cd.Lab = ov.Lab
			case root.Lab != "":
				cd.Lab = root.Lab
			default:
				cd.Lab = labFromPath(filepath.Join(abs, rel), fallback)
			}
			if ov.Name != "" {
				cd.Name = ov.Name
			}
			cd.Duplicate = seen[cd.Name]
			seen[cd.Name] = true
			out = append(out, cd)
		}
	}
	return out
}

// clusterDirs returns the cluster directories below root, sorted.
func clusterDirs(root string) []string {
	subdirs := visibleSubdirs(root)
	flat := false
	for _, d := range subdirs {
		if HasYAML(d) {
			flat = true
			break
		}
	}

	var dirs []string
	switch {
	case flat:
		dirs = subdirs
	case HasYAML(root):
		dirs = []string{root}
	default:
		var walk func(dir string, depth int)
		walk = func(dir string, depth int) {
			for _, d := range visibleSubdirs(dir) {
				switch {
				case HasYAML(d):
					dirs = append(dirs, d)
				case depth < MaxDepth:
					walk(d, depth+1)
				}
			}
		}
		walk(root, 1)
	}
	sort.Strings(dirs)
	return dirs
}

// labFromPath returns the segment after the last "clusters" segment if at
// least one more segment follows before the cluster directory itself.
func labFromPath(path, fallback string) string {
	parts := strings.Split(filepath.ToSlash(filepath.Clean(path)), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] == "clusters" {
			if i+2 < len(parts) { // clusters/<lab>/…/<cluster>
				return parts[i+1]
			}
			break
		}
	}
	return fallback
}

func visibleSubdirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		// follow symlinks (git-sync publishes the checkout as a symlink)
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			out = append(out, p)
		}
	}
	return out
}
