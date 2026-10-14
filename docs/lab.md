# Lab View

The lab view answers **"what runs where in a lab?"**: Lab → Cluster → App → Endpoint, with the desired state (Git), the GitOps status and the actual state side by side. Every mismatch is listed as a **finding**.

Open it via **Inventory → Lab overview** in the dashboard sidebar (`/#lab`), or directly at `/lab`. Filters (text, lab, severity) are kept in the URL.

## Cluster discovery and labs

`-root` can point at a single cluster, at a directory of clusters, or at a whole repo tree:

| `-root` contains | Clusters |
|---|---|
| subdirectories with YAML files | each subdirectory (flat layout) |
| YAML files itself | `-root` is the only cluster |
| neither | searched recursively (up to 4 levels): a directory with YAML files is a cluster, one without is descended into |

The **lab** is taken from the path: the segment after `clusters/` when at least one more segment follows before the cluster directory.

```
clusters/labul/vsphere/platform-sthings   → lab labul
clusters/labda/vsphere/machinery          → lab labda
harvester/clusters/edge                   → -lab (or "default")
```

So `-root ./clusters` shows all labs of the repo at once, and `-root ./clusters/labul/vsphere` shows one lab. Use `-lab <name>` for clusters whose path does not name a lab, and for clusters that only have a kubeconfig.

Cluster names must be unique (they are also the kubeconfig names). A second directory with the same name is skipped and reported as `duplicate name`.

Directories with no GitOps object (no Flux Kustomization / source, no Argo CD Application), and with no reachable cluster behind them, are listed but not counted as clusters. These are typically Crossplane claims or clusterbook data next to the cluster directories.

The file watcher covers root, every cluster directory and the directories in between; any change triggers a debounced full rescan, and removed directories disappear from the dashboard and the metrics.

## Sources

| Source | Provides |
|---|---|
| cluster directories (Git) | desired state: clusters, Kustomizations / Applications |
| `-kubeconfig-dir` / `-kubeconfig-map` | which clusters can be reached |
| live mode | GitOps status, [workload health](live.md#workload-health), [endpoints](endpoints.md) |

clusterbook (IPs/DNS) and machinery (ordered clusters) are planned as further sources.

## Findings

| Severity | Kind | Meaning |
|---|---|---|
| error | `unreachable` | cluster directory in Git, API server unreachable |
| error | `scan error` | cluster directory could not be parsed |
| error | `GitOps failed` | Kustomization / Application / FluxInstance not ready |
| error | `drift` | GitOps ready, but workloads degraded/down or a HelmRelease not ready |
| error | `workloads down` | all workloads down (GitOps not failed) |
| error | `endpoint down` | an HTTPRoute URL failed the probe (`-probe`) |
| warning | `no kubeconfig` | cluster directory in Git, but no kubeconfig: actual state unknown |
| warning | `not in Git` (cluster) | kubeconfig present, but no cluster directory in any scanned repo |
| warning | `not deployed` | Kustomization / Application in Git, but not in the cluster |
| warning | `workloads degraded` | some replicas missing or pod issues (GitOps not failed) |
| warning | `route` | HTTPRoute not Accepted / ResolvedRefs |
| warning | `workloads unreadable` | workloads/pods could not be listed (RBAC) |
| warning | `duplicate name` | second directory with an existing cluster name |
| info | `not in Git` (object) | Kustomization / Application in the cluster without any GitOps manager: applied by hand |

Kustomizations / Applications in the cluster that are not in the cluster directory but were created by another GitOps object (labels `kustomize.toolkit.fluxcd.io/name`, `helm.toolkit.fluxcd.io/name`, `fluxcd.controlplane.io/name`, `app.kubernetes.io/instance`, annotation `argocd.argoproj.io/tracking-id`, or an owner reference) are **nested**: they are in Git elsewhere. They are counted per cluster (`+N nested`), not reported.

## API

`GET /api/lab`

```json
{
  "generatedAt": "2026-10-14T17:57:45Z",
  "live": true,
  "probing": true,
  "labs": [{
    "name": "labul",
    "summary": {"clusters": 2, "reachable": 2, "unmanaged": 0, "apps": 22, "healthy": 34,
                "degraded": 1, "down": 0, "drift": 1, "routes": 24, "routesOK": 24,
                "errors": 1, "warnings": 0},
    "clusters": [{"name": "infra-sthings", "managed": true, "path": "labul/vsphere/infra-sthings",
                  "tech": "flux", "reachability": "reachable", "apps": 6, "nested": 1,
                  "gitops": {"ready": 17}, "workloads": {"healthy": 10, "degraded": 1, "n/a": 4},
                  "drift": 1, "routes": 5, "routesOK": 5, "findings": 1}],
    "findings": [{"severity": "error", "kind": "drift", "cluster": "infra-sthings", "object": "nfs-csi",
                  "message": "GitOps ready, workloads degraded 2/3 — snapshot-controller-…: CrashLoopBackOff"}]
  }]
}
```

`GET /api/clusters` carries the `lab` of every cluster; the sidebar groups clusters by lab when there is more than one.

## Metrics

| Metric | Labels | Value |
|---|---|---|
| `clusterscope_lab_findings` | `lab`, `severity`, `kind` | number of findings |
| `clusterscope_cluster_info` | `cluster`, `lab`, `tech`, `managed` | always `1`; `managed="false"` for kubeconfig-only clusters |

`clusterscope_cluster_info` lets you group the other per-cluster metrics by lab:

```promql
sum by (lab) (clusterscope_app_gitops_drift * on(cluster) group_left(lab) clusterscope_cluster_info)
```
