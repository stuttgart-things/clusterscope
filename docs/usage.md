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
| `-live` | `false` | Enrich graph with real-time status from the Kubernetes API |
| `-kubeconfig` | — | Path to default kubeconfig file |
| `-kubeconfig-dir` | — | Directory with one kubeconfig per cluster (see [Live Mode](live.md#multi-cluster-live-mode)) |
| `-kubeconfig-map` | — | YAML mapping cluster name → kubeconfig path |
| `-self-cluster` | — | Cluster clusterscope runs in (uses `-kubeconfig` / in-cluster) |
| `-refresh` | `30` | Live status refresh interval in seconds |
| `-repos-config` | — | Path to repos.yaml (git-sync Kubernetes mode) |
| `-diff` | `false` | Compare two cluster directories and output a diff |
| `-base-dir` | — | Base cluster directory (used with `-diff`) |
| `-head-dir` | — | Head cluster directory (used with `-diff`) |
| `-format` | `markdown` | Diff output format: `markdown` \| `json` \| `html` |
| `-preview-root` | — | PR head ref cluster files (used with `-serve`) |

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

### PR preview diff

```bash
# Markdown (for PR comments)
clusterscope -diff -base-dir ./clusters/labul/vsphere/movie-scripts -head-dir ./pr/clusters/labul/vsphere/movie-scripts

# JSON
clusterscope -diff -base-dir ./base -head-dir ./head -format json

# HTML
clusterscope -diff -base-dir ./base -head-dir ./head -format html -out diff.html
```

See [PR Preview](preview.md) for details.
