// Package scan discovers cluster directories and auto-detects their technology.
package scan

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/stuttgart-things/clusterscope/internal/graph"
	"github.com/stuttgart-things/clusterscope/pkg/argocd"
	"github.com/stuttgart-things/clusterscope/pkg/flux"
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
	Name string // directory name, used as cluster name
	Path string
	Lab  string
	// Duplicate is set when another directory with the same name was found
	// first (in path order); duplicates are not scanned.
	Duplicate bool
}

// MaxDepth limits how deep Discover descends below root.
const MaxDepth = 4

// DefaultLab is used when no lab can be derived from the path.
const DefaultLab = "default"

// Discover finds cluster directories below root:
//
//   - root's immediate subdirectories contain YAML → each of them is a
//     cluster (flat layout, unchanged behaviour)
//   - root itself contains YAML → root is the only cluster
//   - otherwise directories are searched recursively (up to MaxDepth): a
//     directory with YAML files is a cluster, one without is descended into.
//     This covers repo layouts like clusters/<lab>/<platform>/<cluster>.
//
// The lab of a cluster is the path segment after a "clusters" segment when a
// further segment follows before the cluster directory
// (…/clusters/labul/vsphere/platform-sthings → labul), else lab (or
// DefaultLab when empty). Hidden directories are skipped.
func Discover(root, lab string) []ClusterDir {
	if lab == "" {
		lab = DefaultLab
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}

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

	seen := make(map[string]bool)
	out := make([]ClusterDir, 0, len(dirs))
	for _, d := range dirs {
		name := filepath.Base(d)
		rel, _ := filepath.Rel(root, d)
		cd := ClusterDir{Name: name, Path: d, Lab: labFromPath(filepath.Join(abs, rel), lab), Duplicate: seen[name]}
		seen[name] = true
		out = append(out, cd)
	}
	return out
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
