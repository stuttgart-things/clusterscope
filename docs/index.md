# clusterscope

**Technology-agnostic GitOps cluster visualizer CLI + server.**

clusterscope parses your GitOps YAML files (Flux or ArgoCD) and generates interactive HTML dashboards showing the dependency graph, sources, kustomizations, and applications of your Kubernetes clusters — with no cluster access required.

## Features

- **Flux & ArgoCD support** — auto-detects technology from YAML content
- **Interactive D3.js graph** — dependency visualization with hover and click
- **Multi-cluster dashboard** — index page linking all cluster profiles
- **Serve mode** — live HTTP server with file watcher and auto-reload
- **Live mode** — real-time reconciliation status via `kubectl`
- **PDF export** — print-optimized CSS layout via browser print dialog
- **KCL deployment** — Kubernetes manifests with git-sync sidecar
- **Backstage integration** — `catalog-info.yaml` for discovery

## Quickstart

```bash
# Single cluster (Flux)
clusterscope -dir ./clusters/prod -out profile.html

# Single cluster (ArgoCD)
clusterscope -dir ./argocd/prod -tech argocd -out profile.html

# Multi-cluster static dashboard
clusterscope -root ./clusters -out ./dist

# Live HTTP dashboard
clusterscope -serve :8080 -root ./clusters

# Live mode with kubectl enrichment
clusterscope -serve :8080 -root ./clusters -live -kubeconfig ~/.kube/prod
```

## Supported Technologies

| Technology | Parser | Resources |
|---|---|---|
| Flux | `pkg/flux` | GitRepository, Kustomization, FluxInstance |
| ArgoCD | `pkg/argocd` | Application, ApplicationSet, AppProject |

## Task shortcuts

```bash
task ui-serve ROOT=./clusters PORT=8080
task ui-static ROOT=./clusters OUT=./dist
task ui-serve-live ROOT=./clusters KUBECONFIG=~/.kube/prod
```
