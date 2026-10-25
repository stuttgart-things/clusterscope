# clusterscope

**clusterscope** is a CLI tool and HTTP dashboard that parses GitOps cluster directories and generates interactive, self-contained HTML visualizations of the resource dependency graph — no running cluster required.

Point it at a Flux or ArgoCD cluster folder and get a standalone HTML file you can open in any browser, commit to Git, or attach to a pull request.

## Features

- Parses **FluxCD** `Kustomization`, `GitRepository`, and `FluxInstance` resources
- Parses **ArgoCD** `Application`, `ApplicationSet`, and `AppProject` resources
- Interactive **D3.js** dependency graph with layered DAG layout
- Layer toggles, live search, hover highlighting, and click-to-detail panel
- Cards view alongside the graph for quick scanning
- SOPS-encrypted substitution values are automatically stripped from the output
- **PR preview** — visual diff of what a pull request changes in the cluster graph
- **Live mode** — real-time reconciliation status from a running cluster, per-cluster kubeconfigs and reachability, actual workload health per app with GitOps drift detection
- **Lab view** — all clusters of a lab (or several labs) with desired vs. actual state and every mismatch as a finding
- **Endpoint inventory** — HTTPRoutes and Gateways of all clusters with Accepted/ResolvedRefs status, optional reachability probe (routes can opt out with `clusterscope.stuttgart-things.com/probe: "false"`), and Prometheus `http_sd` targets for blackbox-exporter
- HTTP dashboard with HTMX-driven multi-cluster view
- PDF export via headless Chromium
- Prometheus metrics endpoint

## Installation

```bash
git clone https://github.com/stuttgart-things/clusterscope.git
cd clusterscope
go build -o clusterscope ./cmd/clusterscope
```

## Quick Start

### Generate an HTML profile

```bash
clusterscope -dir ./clusters/labul/vsphere/movie-scripts -out profile.html
open profile.html
```

### Run the dashboard

```bash
clusterscope -serve :8080 -root ./clusters/labul/vsphere
# Open http://localhost:8080
```

### PR preview (CLI)

Compare two cluster directories and output a markdown diff:

```bash
clusterscope -diff \
  -base-dir ./clusters/labul/vsphere/movie-scripts \
  -head-dir ./pr-branch/clusters/labul/vsphere/movie-scripts
```

### PR preview (dashboard)

```bash
clusterscope -serve :8080 -root ./clusters/labul/vsphere
# Open http://localhost:8080 → use the "PR Preview" form in the sidebar
# Enter repo URL, PR number, and cluster name
```

See [PR Preview](docs/preview.md) for full documentation.

## Flags

| Flag | Default | Description |
|---|---|---|
| `-dir` | `.` | Path to the cluster directory to parse |
| `-out` | stdout | Output file path |
| `-tech` | `flux` | Technology to parse: `flux` \| `argocd` |
| `-root` | `.` | Root directory: one cluster, a directory of clusters, or a repo tree (`clusters/<lab>/…/<cluster>`) |
| `-config` | — | Several roots (repos) + per-directory cluster name / lab overrides, replaces `-root` (see [Lab View](docs/lab.md)) |
| `-lab` | `default` | Lab name for clusters whose path does not contain `clusters/<lab>/…` |
| `-clusterbook-url` | — | clusterbook REST API: IP / DNS allocations matched against the lab's clusters (see [Lab View](docs/lab.md#clusterbook)) |
| `-serve` | — | Start HTTP dashboard server (e.g. `:8080`) |
| `-live` | `false` | Enrich graph with real-time reconciliation status |
| `-kubeconfig` | — | Path to default kubeconfig file |
| `-kubeconfig-dir` | — | Directory with one kubeconfig per cluster (multi-cluster live mode) |
| `-kubeconfig-map` | — | YAML mapping cluster name → kubeconfig path |
| `-self-cluster` | — | Cluster clusterscope runs in (uses `-kubeconfig` / in-cluster) |
| `-probe` | `false` | Probe every HTTPRoute URL on each live refresh |
| `-probe-timeout` | `5s` | Per-URL probe timeout |
| `-refresh` | `30` | Live status refresh interval in seconds |
| `-diff` | `false` | Compare two cluster directories and output a diff |
| `-base-dir` | — | Base cluster directory (used with `-diff`) |
| `-head-dir` | — | Head cluster directory (used with `-diff`) |
| `-format` | `markdown` | Diff output format: `markdown` \| `json` \| `html` |
| `-preview-root` | — | Directory containing PR head ref cluster files (used with `-serve`) |
| `-base-path` | `$BASE_PATH` | Serve the dashboard under this prefix behind [sthings-portal](https://github.com/stuttgart-things/sthings-portal), e.g. `/clusterscope` (used with `-serve`, see [serve](docs/serve.md#behind-sthings-portal)) |

## Documentation

- [CLI Reference](docs/usage.md)
- [Serve Mode](docs/serve.md)
- [PR Preview](docs/preview.md)
- [Live Mode](docs/live.md)
- [Endpoint Inventory](docs/endpoints.md)
- [Lab View](docs/lab.md)
- [PDF Export](docs/pdf.md)
- [KCL Deployment](docs/kcl.md)
- [Architecture](docs/architecture.md)
- [Configuration](docs/configuration.md)

## Project structure

```
clusterscope/
├── cmd/clusterscope/main.go        # CLI entrypoint
├── internal/
│   ├── diff/                        # Graph diff engine + markdown output
│   ├── graph/graph.go               # Shared data model (Node, Edge, ClusterProfile)
│   ├── live/live.go                 # Kubernetes real-time enrichment
│   ├── preview/clone.go             # On-demand git clone for PR preview
│   ├── render/                      # HTML templates (cluster, diff, shell, index)
│   ├── scan/scan.go                 # Tech detection + cluster discovery
│   └── serve/serve.go               # HTTP server + file watcher
├── pkg/
│   ├── flux/parser.go               # FluxCD YAML parser
│   └── argocd/parser.go             # ArgoCD parser
├── kcl/                             # KCL Kubernetes deployment manifests
└── docs/                            # Documentation
```
