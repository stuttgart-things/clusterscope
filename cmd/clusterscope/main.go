// clusterscope — visualize GitOps cluster profiles as interactive HTML.
//
// Usage:
//
//	clusterscope -dir <cluster-dir> [-out <output.html>] [-tech flux|argocd]
//	clusterscope -serve -root <clusters-root> [-addr :8080]
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/stuttgart-things/clusterscope/internal/render"
	"github.com/stuttgart-things/clusterscope/pkg/argocd"
	"github.com/stuttgart-things/clusterscope/pkg/flux"
)

func main() {
	dir := flag.String("dir", ".", "path to the cluster directory to visualize")
	out := flag.String("out", "", "output file path (default: stdout)")
	tech := flag.String("tech", "flux", "technology to parse: flux | argocd")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `clusterscope — generate an interactive HTML cluster profile from GitOps YAML files

Supported technologies: flux, argocd (argocd: planned, see GitHub issues)

Usage:
  clusterscope -dir <cluster-dir> [-out <output.html>] [-tech flux|argocd]

Flags:
`)
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  clusterscope -dir ./clusters/labul/vsphere/movie-scripts
  clusterscope -dir ./clusters/labul/vsphere/movie-scripts -out profile.html
  clusterscope -dir ./argocd/clusters/prod -tech argocd -out prod.html
`)
	}
	flag.Parse()

	var profile interface { /* graph.ClusterProfile */
	}
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
