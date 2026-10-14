# Live Mode

Live mode enriches the static YAML-parsed graph with real-time reconciliation status fetched from the Kubernetes API (client-go, no `kubectl` required).

## Requirements

- Inside Kubernetes: the pod's ServiceAccount, or
- a kubeconfig (`-kubeconfig`, `KUBECONFIG` env, `~/.kube/config`), or
- one kubeconfig per cluster (`-kubeconfig-dir` / `-kubeconfig-map`, see below)
- Cluster API access (read-only on the relevant CRDs)

## Usage

```bash
# Serve with live enrichment
clusterscope -serve :8080 -root ./clusters -live -kubeconfig ~/.kube/prod

# Single-cluster static with live
clusterscope -dir ./clusters/prod -tech flux -live -kubeconfig ~/.kube/prod -out live.html

# Custom refresh interval (default: 30s)
clusterscope -serve :8080 -root ./clusters -live -refresh 60

# Multi-cluster: one kubeconfig per cluster
clusterscope -serve :8080 -root ./clusters/labul/vsphere -live \
  -kubeconfig-dir /kubeconfigs -self-cluster platform-sthings
```

## Multi-cluster live mode

With only `-kubeconfig` (or in-cluster), **every** cluster under `-root` is enriched from that one cluster. This is fine for a single cluster, but in a multi-cluster dashboard all other clusters would show the status of the wrong cluster.

Per-cluster kubeconfigs fix this:

| Flag | Description |
|---|---|
| `-kubeconfig-dir` | Directory with one kubeconfig file per cluster, **named after the cluster directory** (e.g. `/kubeconfigs/infra-sthings`). Fits a mounted Secret with one key per cluster. |
| `-kubeconfig-map` | YAML file for names that do not match (see below). Relative paths are resolved against `-kubeconfig-dir`. |
| `-self-cluster` | The cluster clusterscope runs in. Only this cluster falls back to `-kubeconfig` / in-cluster. |

```yaml
# -kubeconfig-map
clusters:
  platform-sthings: labul-platform.yaml
  homerun2-test1: /kubeconfigs/labul/homerun2-test1
```

Resolution per cluster, as soon as `-kubeconfig-dir` or `-kubeconfig-map` is set:

1. `-kubeconfig-map` entry
2. `-kubeconfig-dir/<cluster>`
3. `-kubeconfig` / in-cluster — **only** for `-self-cluster`
4. otherwise `unconfigured` (no live status, nodes `unknown`)

Clients are cached per kubeconfig file and rebuilt when the file changes (Secret rotation). Clusters are refreshed in parallel, so one unreachable API server does not delay the others.

## Cluster reachability

Before enrichment, clusterscope calls `GET /version` on the cluster's API server.

| Reachability | Meaning |
|---|---|
| `reachable` | API server answered and accepted the credentials |
| `unreachable` | Connection failed, timed out (10s), or credentials were rejected (401/403) |
| `unconfigured` | No kubeconfig resolved for this cluster |

For `unreachable` and `unconfigured` clusters all nodes are reset to `unknown`, so a cluster that goes down does not keep showing its last known status.

Reachability is exposed in:

- `GET /api/clusters` — `reachability` and `liveError` per cluster
- the dashboard sidebar — red ring + `unreachable` / `no kubeconfig`
- `/metrics` — `clusterscope_cluster_reachable{cluster="…"}` (`1` / `0`, absent when unconfigured)

## Status values

| Status | Color | Meaning |
|---|---|---|
| `ready` | 🟢 Green | Resource reconciled successfully |
| `failed` | 🔴 Red | Reconciliation failed |
| `progressing` | 🟠 Orange | Reconciliation in progress |
| `unknown` | Grey | Cluster unreachable / unconfigured, or no status available |

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
- `-kubeconfig` and `-kubeconfig-dir` are validated (must exist) before the server starts
- Live errors degrade to `status: "unknown"` — the server never crashes on live failures
- Only read access is needed on workload clusters (`get`/`list` on the Flux / Argo CD CRDs)
