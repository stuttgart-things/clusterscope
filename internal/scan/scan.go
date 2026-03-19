// Package scan discovers cluster directories and auto-detects their technology.
package scan

import (
	"os"
	"path/filepath"
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
