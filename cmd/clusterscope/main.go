// clusterscope — visualize GitOps cluster profiles as interactive HTML.
//
// Usage:
//
//	clusterscope -dir <cluster-dir> [-out <output.html>] [-tech flux|argocd]
//	clusterscope -serve :8080 -root <clusters-root-dir>
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/stuttgart-things/clusterscope/internal/render"
	"github.com/stuttgart-things/clusterscope/internal/scan"
	"github.com/stuttgart-things/clusterscope/internal/serve"
	"github.com/stuttgart-things/clusterscope/pkg/argocd"
	"github.com/stuttgart-things/clusterscope/pkg/flux"
)

// Build-time variables (injected via -ldflags).
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	render.SetBuildInfo(version, commit, date)

	dir := flag.String("dir", ".", "path to the cluster directory to visualize")
	out := flag.String("out", "", "output file path (default: stdout)")
	tech := flag.String("tech", "flux", "technology to parse: flux | argocd")
	serveAddr := flag.String("serve", "", "start HTTP dashboard server on addr (e.g. :8080); requires -root")
	root := flag.String("root", ".", "root directory containing cluster subdirs (used with -serve)")
	reposConfig := flag.String("repos-config", "", "path to repos.yaml defining git repositories (used with -serve in Kubernetes mode)")
	liveMode := flag.Bool("live", false, "enrich graph with real-time reconciliation status via kubectl")
	kubeconfig := flag.String("kubeconfig", "", "path to kubeconfig file (default: KUBECONFIG env / ~/.kube/config)")
	refreshSeconds := flag.Int("refresh", 30, "live status refresh interval in seconds (used with -live)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `clusterscope — generate an interactive HTML cluster profile from GitOps YAML files

Supported technologies: flux, argocd

Usage:
  clusterscope -dir <cluster-dir> [-out <output.html>] [-tech flux|argocd]
  clusterscope -serve :8080 -root <clusters-root-dir>

Flags:
`)
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  clusterscope -dir ./clusters/labul/vsphere/movie-scripts
  clusterscope -dir ./clusters/labul/vsphere/movie-scripts -out profile.html
  clusterscope -dir ./argocd/clusters/prod -tech argocd -out prod.html
  clusterscope -serve :8080 -root ./clusters/
  clusterscope -serve :8080 -root /data -repos-config /etc/git-sync/repos.yaml
  clusterscope -serve :8080 -root ./clusters/ -live -kubeconfig ~/.kube/prod
  clusterscope -dir ./clusters/prod -tech flux -live -kubeconfig ~/.kube/prod -out live.html
`)
	}
	flag.Parse()

	// ── Serve mode ────────────────────────────────────────────────────────────
	if *serveAddr != "" {
		if *reposConfig != "" {
			fmt.Fprintf(os.Stderr, "ℹ repos-config: %s (git-sync manages data delivery)\n", *reposConfig)
		}
		if *liveMode && *kubeconfig != "" {
			if _, err := os.Stat(*kubeconfig); err != nil {
				fmt.Fprintf(os.Stderr, "kubeconfig not found: %s\n", *kubeconfig)
				os.Exit(1)
			}
		}
		opts := serve.Options{
			Live:           *liveMode,
			Kubeconfig:     *kubeconfig,
			RefreshSeconds: *refreshSeconds,
		}
		if err := serve.Start(*serveAddr, *root, opts); err != nil {
			fmt.Fprintf(os.Stderr, "serve error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// ── Static render mode ────────────────────────────────────────────────────

	// Multi-cluster mode: -root given without -serve
	if *root != "." && *dir == "." {
		outDir := *out
		if outDir == "" {
			outDir = "."
		}
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "error creating output dir: %v\n", err)
			os.Exit(1)
		}
		clusters, err := scan.Root(*root)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error scanning root: %v\n", err)
			os.Exit(1)
		}

		// Write per-cluster HTML files
		for _, c := range clusters {
			if c.Err != nil || c.Profile == nil {
				fmt.Fprintf(os.Stderr, "skip %s: %v\n", c.Name, c.Err)
				continue
			}
			clusterFile := filepath.Join(outDir, c.Name+".html")
			f, err := os.Create(clusterFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error creating %s: %v\n", clusterFile, err)
				continue
			}
			if err := render.WriteHTML(f, c.Profile); err != nil {
				fmt.Fprintf(os.Stderr, "error rendering %s: %v\n", c.Name, err)
			}
			_ = f.Close()
			fmt.Fprintf(os.Stderr, "✓ %s → %s\n", c.Name, clusterFile)
		}

		// Write index.html
		indexFile := filepath.Join(outDir, "index.html")
		idxClusters := make([]render.IndexCluster, len(clusters))
		for i, c := range clusters {
			idxClusters[i] = render.IndexCluster{Name: c.Name, Profile: c.Profile, Err: c.Err}
		}
		fi, err := os.Create(indexFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error creating index.html: %v\n", err)
			os.Exit(1)
		}
		if err := render.WriteIndex(fi, idxClusters); err != nil {
			fmt.Fprintf(os.Stderr, "error rendering index: %v\n", err)
		}
		_ = fi.Close()
		fmt.Fprintf(os.Stderr, "✓ index.html → %s\n", indexFile)
		return
	}

	// Single-cluster mode
	var profile interface{} //nolint:unused
	_ = profile

	switch *tech {
	case "flux":
		p, err := flux.ParseDir(*dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error parsing Flux directory: %v\n", err)
			os.Exit(1)
		}

		w := os.Stdout
		if *out != "" {
			f, err := os.Create(*out)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error creating output file: %v\n", err)
				os.Exit(1)
			}
			defer func() { _ = f.Close() }()
			w = f
		}

		if err := render.WriteHTML(w, p); err != nil {
			fmt.Fprintf(os.Stderr, "error rendering HTML: %v\n", err)
			os.Exit(1)
		}

	case "argocd":
		p, err := argocd.ParseDir(*dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error parsing ArgoCD directory: %v\n", err)
			os.Exit(1)
		}

		w := os.Stdout
		if *out != "" {
			f, err := os.Create(*out)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error creating output file: %v\n", err)
				os.Exit(1)
			}
			defer func() { _ = f.Close() }()
			w = f
		}

		if err := render.WriteHTML(w, p); err != nil {
			fmt.Fprintf(os.Stderr, "error rendering HTML: %v\n", err)
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "unknown technology %q — supported: flux, argocd\n", *tech)
		os.Exit(1)
	}

	if *out != "" {
		abs, _ := filepath.Abs(*out)
		fmt.Fprintf(os.Stderr, "✓ Written to %s\n", abs)
	}
}
