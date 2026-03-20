# clusterscope — KCL Deployment

This directory contains [KCL](https://kcl-lang.io/) manifests to deploy `clusterscope` to Kubernetes.

The deployment renders the following resources:

| Resource | Kind |
|---|---|
| `clusterscope` | Namespace |
| `clusterscope` | ServiceAccount |
| `clusterscope` | ClusterRole |
| `clusterscope` | ClusterRoleBinding |
| `clusterscope` | Service |
| `clusterscope` | Deployment |

When `gitSyncEnabled: true`, the Deployment includes a `git-sync` sidecar that continuously syncs a Git repository into an `emptyDir` volume shared with the main container.

---

## Directory structure

```
kcl/
├── schema.k           # ClusterScope config schema + defaults
├── main.k             # Entry point, assembles all manifests
├── deploy.k           # Deployment + git-sync sidecar logic
├── clusterrole.k      # ClusterRole + ClusterRoleBinding
├── service.k          # Service
├── namespace.k        # Namespace
├── serviceaccount.k   # ServiceAccount
├── labels.k           # Shared label helpers + option bindings
└── kcl.mod            # KCL module definition
```

---

## Configuration (schema.k)

| Key | Default | Description |
|---|---|---|
| `config.name` | `clusterscope` | Resource name |
| `config.namespace` | `clusterscope` | Kubernetes namespace |
| `config.image` | `ghcr.io/stuttgart-things/clusterscope:latest` | Container image |
| `config.imagePullPolicy` | `IfNotPresent` | Image pull policy |
| `config.replicas` | `1` | Number of replicas |
| `config.port` | `8080` | HTTP serve port |
| `config.dir` | `/data` | Path inside the container to scan for GitOps manifests |
| `config.tech` | `flux` | GitOps technology: `flux` or `argocd` |
| `config.gitSyncEnabled` | `false` | Enable git-sync sidecar |
| `config.gitSyncImage` | `registry.k8s.io/git-sync/git-sync:v4.4.0` | git-sync image |
| `config.gitSyncRepo` | `""` | Git repository URL to sync |
| `config.gitSyncBranch` | `main` | Branch/ref to sync |
| `config.gitSyncPeriod` | `60s` | Sync interval |
| `config.httpRouteEnabled` | `false` | Create HTTPRoute (Gateway API) |
| `config.gatewayName` | `""` | Gateway name |
| `config.gatewayNamespace` | `default` | Namespace of the Gateway |
| `config.hostname` | `""` | Hostname prefix |
| `config.domain` | `""` | Domain → URL: `hostname.domain` |
| `config.cpuRequest` | `50m` | CPU request |
| `config.cpuLimit` | `500m` | CPU limit |
| `config.memoryRequest` | `128Mi` | Memory request |
| `config.memoryLimit` | `512Mi` | Memory limit |

---

## Deploy profile

Configuration is passed via a YAML profile file using KCL's `kcl_options` format:

```yaml
# tests/kcl-deploy-profile.yaml
kcl_options:
  - key: config.image
    value: ghcr.io/stuttgart-things/clusterscope:latest
  - key: config.namespace
    value: clusterscope
  - key: config.tech
    value: flux
  - key: config.dir
    value: /data/harvester/clusters
  - key: config.port
    value: "8080"
  - key: config.replicas
    value: "1"
  - key: config.gitSyncEnabled
    value: "true"
  - key: config.gitSyncRepo
    value: https://github.com/stuttgart-things/harvester
  - key: config.gitSyncBranch
    value: main
  - key: config.gitSyncPeriod
    value: 60s
  - key: config.httpRouteEnabled
    value: "true"
  - key: config.gatewayName
    value: movie-scripts2-gateway
  - key: config.gatewayNamespace
    value: default
  - key: config.hostname
    value: clusterscope
  - key: config.domain
    value: movie-scripts2.sthings-vsphere.labul.sva.de
```

> **Note on `config.dir`**: In Kubernetes serve mode (`-serve` + git-sync), `config.dir` must point to the subfolder *inside* the synced repository that contains the cluster directories.
> git-sync places the repo at `/data/<repo-name>/` — so for the `harvester` repo with a `clusters/` subfolder, use `/data/harvester/clusters`.

---

## Deploy via task

```bash
# Deploy (uses KUBECONFIG env or pass KUBECONFIG_PATH explicitly)
task deploy

# Deploy with custom kubeconfig
KUBECONFIG_PATH=~/.kube/my-cluster task deploy

# Deploy with custom profile
PROFILE=tests/my-profile.yaml KUBECONFIG_PATH=~/.kube/my-cluster task deploy

# Undeploy (removes all resources)
KUBECONFIG_PATH=~/.kube/my-cluster task undeploy
```

---

## Deploy manually

```bash
# Render + apply
kcl run kcl/ -Y tests/kcl-deploy-profile.yaml | python3 -c "
import sys, yaml
data = yaml.safe_load(sys.stdin)
for m in data.get('manifests', []):
    print('---')
    print(yaml.dump(m, default_flow_style=False))
" | kubectl apply -f -

# Render only (inspect before applying)
kcl run kcl/ -Y tests/kcl-deploy-profile.yaml
```

> **Why Python?** KCL outputs a single YAML document with a `manifests:` list.
> `kubectl apply` requires separate YAML documents. The Python snippet splits them correctly.

---

## git-sync sidecar

When `gitSyncEnabled: true`, the Deployment adds a `git-sync` sidecar:

```
clusterscope pod
├── container: clusterscope      (-root=/data/harvester/clusters -tech=flux -serve=:8080)
│   └── volumeMount: gitdata → /data
└── container: git-sync          (--repo=... --ref=main --root=/data --period=60s --depth=1)
    └── volumeMount: gitdata → /data
```

Both containers share the same `gitdata` emptyDir volume mounted at `/data`.
git-sync creates: `/data/harvester → .worktrees/<hash>` (symlink).
clusterscope reads: `/data/harvester/clusters/` via `-root`.

---

## Example: movie-scripts cluster (harvester repo)

Deployed to: `clusterscope.movie-scripts2.sthings-vsphere.labul.sva.de`

```
gitSyncRepo:    https://github.com/stuttgart-things/harvester
gitSyncBranch:  main
config.dir:     /data/harvester/clusters
config.tech:    flux
```

Cluster directories served:
- `infra`
- `platform`
- `xplane`
