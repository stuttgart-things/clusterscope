# clusterscope

**clusterscope** is a CLI tool that parses GitOps cluster directories and generates interactive, self-contained HTML visualizations of the resource dependency graph — no running cluster required.

Point it at a Flux or ArgoCD cluster folder and get a standalone HTML file you can open in any browser, commit to Git, or attach to a pull request.

## Features

- Parses **FluxCD** `Kustomization`, `GitRepository`, and `FluxInstance` resources
- Interactive **D3.js** dependency graph with layered DAG layout
- Layer toggles, live search, hover highlighting, and click-to-detail panel
- Cards view alongside the graph for quick scanning
- SOPS-encrypted substitution values are automatically stripped from the output
- Outputs to **stdout** or a file — fully offline, no cluster access needed
- ArgoCD support planned (see [GitHub Issues](https://github.com/stuttgart-things/clusterscope/issues))

## Installation

```bash
git clone https://github.com/stuttgart-things/clusterscope.git
cd clusterscope
go build -o clusterscope ./cmd/clusterscope
```

## Usage

### Generate an HTML profile from a Flux cluster directory

```bash
# Output to stdout
clusterscope -dir ./clusters/labul/vsphere/my-cluster

# Write to a file
clusterscope -dir ./clusters/labul/vsphere/my-cluster -out profile.html

# Specify technology explicitly (default: flux)
clusterscope -dir ./clusters/labul/vsphere/my-cluster -tech flux -out profile.html
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `-dir` | `.` | Path to the cluster directory to parse |
| `-out` | stdout | Output file path |
| `-tech` | `flux` | Technology to parse: `flux` \| `argocd` |

### Open the result

```bash
xdg-open profile.html   # Linux
open profile.html        # macOS
```

## Project structure

```
clusterscope/
├── cmd/clusterscope/main.go      # CLI entrypoint
├── internal/
│   ├── graph/graph.go            # Shared data model (Node, Edge, ClusterProfile)
│   └── render/
│       ├── render.go             # HTML renderer (technology-agnostic)
│       └── template.html         # Interactive D3.js template
├── pkg/
│   ├── flux/parser.go            # FluxCD YAML parser
│   └── argocd/parser.go          # ArgoCD parser (planned)
└── go.mod
```

## Roadmap

| Issue | Feature |
|---|---|
| [#1](https://github.com/stuttgart-things/clusterscope/issues/1) | Multi-cluster mode: scan a `clusters/` root → index dashboard |
| [#2](https://github.com/stuttgart-things/clusterscope/issues/2) | ArgoCD parser: Application, ApplicationSet, AppProject |
| [#3](https://github.com/stuttgart-things/clusterscope/issues/3) | Serve mode: HTTP microservice with scrollable multi-cluster dashboard |
| [#4](https://github.com/stuttgart-things/clusterscope/issues/4) | Live mode: real-time reconciliation status from a running cluster |
| [#5](https://github.com/stuttgart-things/clusterscope/issues/5) | CI integration: Dockerfile + GitHub Actions |
| [#6](https://github.com/stuttgart-things/clusterscope/issues/6) | UI redesign: multi-technology, multi-cluster carousel view |
