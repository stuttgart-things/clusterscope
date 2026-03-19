# Serve Mode

clusterscope can run as a persistent HTTP dashboard server that watches your cluster directories for changes and serves interactive HTML profiles.

## Start the server

```bash
clusterscope -serve :8080 -root ./clusters
```

Open `http://localhost:8080` in your browser.

## How it works

1. On startup, all subdirectories of `-root` are scanned and parsed
2. Technology is auto-detected per cluster (Flux vs ArgoCD)
3. A file watcher monitors `-root` for YAML changes
4. Changes are debounced (500ms) and trigger a re-scan of the affected cluster
5. The `/api/clusters` endpoint returns a JSON summary of all clusters
6. `/clusters/<name>` returns the HTML profile for a specific cluster

## Endpoints

| Path | Description |
|---|---|
| `/` | HTMX shell with cluster selector |
| `/api/clusters` | JSON list of cluster summaries |
| `/clusters/<name>` | HTML profile for cluster `<name>` |

## Taskfile

```bash
task ui-serve ROOT=./clusters PORT=8080
task ui-serve-live ROOT=./clusters KUBECONFIG=~/.kube/prod
```

## Kubernetes deployment

See [KCL Deployment](kcl.md) for running clusterscope in a cluster with a git-sync sidecar that keeps `-root` synchronized from a Git repository.
