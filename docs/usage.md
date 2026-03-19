# CLI Reference

## Synopsis

```
clusterscope [flags]
```

## Flags

| Flag | Default | Description |
|---|---|---|
| `-dir` | `.` | Path to the cluster directory to visualize |
| `-tech` | `flux` | Technology to parse: `flux` or `argocd` |
| `-out` | stdout | Output file path (`.html`) |
| `-root` | `.` | Root directory containing cluster subdirs |
| `-serve` | — | Start HTTP dashboard server (e.g. `:8080`) |
| `-live` | `false` | Enrich graph with real-time kubectl status |
| `-kubeconfig` | — | Path to kubeconfig file |
| `-refresh` | `30` | Live status refresh interval in seconds |
| `-repos-config` | — | Path to repos.yaml (git-sync Kubernetes mode) |

## Examples

### Single cluster — Flux

```bash
clusterscope -dir ./clusters/prod
clusterscope -dir ./clusters/prod -out profile.html
```

### Single cluster — ArgoCD

```bash
clusterscope -dir ./argocd/prod -tech argocd -out prod.html
```

### Multi-cluster static dashboard

```bash
clusterscope -root ./clusters -out ./dist
# Generates: dist/index.html + dist/<cluster-name>.html per cluster
```

### Serve mode

```bash
clusterscope -serve :8080 -root ./clusters
```

See [Serve Mode](serve.md) for details.

### Live mode

```bash
clusterscope -serve :8080 -root ./clusters -live -kubeconfig ~/.kube/prod
```

See [Live Mode](live.md) for details.
