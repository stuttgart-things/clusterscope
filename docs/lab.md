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

### Several repos, name and lab overrides (`-config`)

When the clusters of a lab live in several repos, or a directory is not named like its cluster, use `-config` instead of `-root` (YAML or JSON):

```yaml
roots:
  - path: /data/stuttgart-things/clusters/labul
  - path: /data/harvester/clusters
    name: harvester          # prefix of the shown path: harvester/edge-test2
    lab: harvester           # lab of this root's clusters (optional)
    include: [edge-test2]    # only these directories (default: all)
clusters:                    # per directory name
  edge-test2: { name: edge-tt-test2, lab: labul }
```

- Lab, in order: the cluster override, the root's `lab`, the path (`clusters/<lab>/…`), `-lab`.
- The cluster **name** is the key for the kubeconfig (`-kubeconfig-dir/<name>`) and the live status, so override it wherever directory and cluster differ.
- A root that does not exist yet (e.g. not yet cloned by git-sync) is skipped and scanned as soon as it appears. Roots are also rescanned every 5 minutes, because git-sync publishes a new checkout by swapping a symlink above the watched directories.
- Unquoted YAML 1.1 booleans (`y`, `yes`, `on`, …) as names are read as `true`, so quote such names.

Cluster names must be unique (they are also the kubeconfig names). A second directory with the same name is skipped and reported as `duplicate name`.

Directories with no GitOps object (no Flux Kustomization / source, no Argo CD Application), and with no reachable cluster behind them, are listed but not counted as clusters. These are typically Crossplane claims or clusterbook data next to the cluster directories.

When a GitOps object of another cluster points at such a directory (a Flux Kustomization's `spec.path`, an Argo CD Application's source path), the row says whose content it is: `machinery-xrs` reads "content of machinery (via xrs)", and the machinery row lists `includes machinery-xrs`. The path is matched as a suffix of the directory's path; the source is not resolved to a repository. A directory nothing points at keeps the plain "not counted" text.

The file watcher covers root, every cluster directory and the directories in between; any change triggers a debounced full rescan, and removed directories disappear from the dashboard and the metrics.

## Sources

| Source | Provides |
|---|---|
| cluster directories (Git) | desired state: clusters, Kustomizations / Applications |
| `-kubeconfig-dir` / `-kubeconfig-map` | which clusters can be reached |
| live mode | GitOps status, [workload health](live.md#workload-health), [endpoints](endpoints.md) |
| clusterbook (`-clusterbook-url`, `clusterbooks:`) | IP / DNS allocation per cluster |
| machinery (`machineries:`) | ClusterStack orders: the desired state of clusters built by Crossplane |

### clusterbook

A [clusterbook](https://github.com/stuttgart-things/clusterbook) REST API (`/api/v1/clusters`) lists the IP / DNS allocations. One clusterbook per lab: `-clusterbook-url` (lab: `-lab`), or several in the `-config` file:

```yaml
clusterbooks:
  - url: https://clusterbook.infra.sthings-vsphere.labul.sva.de
    lab: labul
    names:                   # clusterbook name → cluster name (optional)
      infra: infra-sthings
```

An allocation belongs to a cluster of its lab by, in order: `names`, the same name, the domain — the one cluster whose HTTPRoute hostnames lie under the allocated FQDN (`*.infra.sthings-vsphere.labul.sva.de` → `infra-sthings`). `names` is only needed where a cluster has no routes yet. The lab view shows the addresses and FQDN per cluster.

clusterbook is read at startup and every 5 minutes. When it cannot be read, the lab shows `source unavailable` and keeps the last result.

### machinery

A cluster built from a `ClusterStack` XR ([crossplane-configurations `bootstrap/cluster`](https://github.com/stuttgart-things/crossplane-configurations/tree/main/bootstrap/cluster)) has no cluster directory: its desired state is the order — distribution, size, profiles — and the Argo CD ApplicationSets those profiles select. Without this source such a cluster shows up as `not in Git`.

The orders are read from the machinery cluster's API (`clusterstacks.config.stuttgart-things.com`), through its kubeconfig in `-kubeconfig-dir` / `-kubeconfig-map` — so it needs live mode, and RBAC to list `clusterstacks`. In the `-config` file:

```yaml
machineries:
  - cluster: machinery        # kubeconfig name
    lab: labda
    environments: [labda]     # spec.environmentConfig; one machinery builds for several labs
```

An order belongs to the lab's cluster of the same name. The cluster then shows `ordered on <machinery>` with distribution, size and profiles instead of `kubeconfig only`, and is no longer `not in Git` or counted as unmanaged. Ready means `status.ready` **and** the `Ready` condition, as in cluster-build-watch. Read at startup and every 5 minutes, like clusterbook.

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
| warning | `IP without cluster` | allocation in clusterbook that no cluster of the lab matches (leftover of a torn-down or test cluster) |
| warning | `source unavailable` | clusterbook / machinery not readable (last result kept) |
| warning | `order without cluster` | ClusterStack ready on machinery, but no kubeconfig and no cluster directory |
| warning | `order not synced` | the cluster's ClusterStack is not Synced (composition error) |
| info | `no GitOps yet` | cluster directory and a running cluster, but no Flux / Argo CD objects in either (e.g. k3s set up, bootstrap pending) |
| info | `not in Git` (object) | Kustomization / Application in the cluster without any GitOps manager: applied by hand |
| info | `no IP allocation` | cluster of a lab with a clusterbook, but no allocation for it |
| info | `being built` / `being torn down` | ClusterStack without a cluster yet, with its stage / being deleted |
| info | `order not ready` | the cluster's ClusterStack is not ready (stage shown) |

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
                  "drift": 1, "routes": 5, "routesOK": 5, "findings": 1,
                  "addresses": ["10.31.101.6"], "fqdn": "*.infra.sthings-vsphere.labul.sva.de",
                  "clusterbookName": "infra"}],
    "findings": [{"severity": "error", "kind": "drift", "cluster": "infra-sthings", "object": "nfs-csi",
                  "message": "GitOps ready, workloads degraded 2/3 — snapshot-controller-…: CrashLoopBackOff"}]
  }],
  "sources": [{"kind": "clusterbook", "url": "https://clusterbook.infra.sthings-vsphere.labul.sva.de",
               "ok": true, "records": 7, "fetchedAt": "2026-10-15T04:25:58Z"}]
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
