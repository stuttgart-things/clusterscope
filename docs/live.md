# Live Mode

Live mode enriches the static YAML-parsed graph with real-time reconciliation status fetched from a running cluster via `kubectl`.

## Requirements

- `kubectl` on `PATH`
- A valid `KUBECONFIG` (environment variable or `-kubeconfig` flag)
- Cluster API access (read-only on relevant CRDs)

## Usage

```bash
# Serve with live enrichment
clusterscope -serve :8080 -root ./clusters -live -kubeconfig ~/.kube/prod

# Single-cluster static with live
clusterscope -dir ./clusters/prod -tech flux -live -kubeconfig ~/.kube/prod -out live.html

# Custom refresh interval (default: 30s)
clusterscope -serve :8080 -root ./clusters -live -refresh 60
```

## Status values

| Status | Color | Meaning |
|---|---|---|
| `ready` | 🟢 Green | Resource reconciled successfully |
| `failed` | 🔴 Red | Reconciliation failed |
| `progressing` | 🟠 Orange | Reconciliation in progress |
| `unknown` | Grey | kubectl unreachable or no status available |

## Data sources

### Flux

| Resource | API Group |
|---|---|
| Kustomizations | `kustomize.toolkit.fluxcd.io` |
| GitRepositories | `source.toolkit.fluxcd.io` |

### ArgoCD

| Resource | API Group |
|---|---|
| Applications | `argoproj.io` |

## UI changes in live mode

- Node border color reflects live status (instead of type color)
- Clicking a node shows `Status`, `Message`, and `UpdatedAt` in the detail panel
- A `🟢 Live · <timestamp>` badge appears in the cluster profile header

## Security

- No credentials are written to HTML output
- `KUBECONFIG` follows standard kubectl conventions
- kubeconfig path is validated (file must exist) before the server starts
- Errors from `kubectl` degrade silently to `status: "unknown"` — the server never crashes on live failures
