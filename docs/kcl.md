# KCL Deployment

clusterscope ships with a [KCL](https://kcl-lang.io/) deployment module (`kcl/`) that generates all Kubernetes manifests including an optional git-sync sidecar and HTTPRoute.

## Generated resources

| Resource | Kind |
|---|---|
| `clusterscope` | Namespace |
| `clusterscope` | ServiceAccount |
| `clusterscope` | ClusterRole |
| `clusterscope` | ClusterRoleBinding |
| `clusterscope` | Service |
| `clusterscope` | Deployment |
| `clusterscope` | HTTPRoute *(optional, when `httpRouteEnabled: true`)* |

## Deploy via task

```bash
# Deploy (uses KUBECONFIG env)
task deploy

# Deploy with custom kubeconfig
KUBECONFIG_PATH=~/.kube/my-cluster task deploy

# Deploy with custom profile
PROFILE=tests/my-profile.yaml KUBECONFIG_PATH=~/.kube/my-cluster task deploy

# Undeploy (removes all resources)
KUBECONFIG_PATH=~/.kube/my-cluster task undeploy
```

## Deploy manually

```bash
kcl run kcl/ -Y tests/kcl-deploy-profile.yaml | python3 -c "
import sys, yaml
data = yaml.safe_load(sys.stdin)
for m in data.get('manifests', []):
    print('---')
    print(yaml.dump(m, default_flow_style=False))
" | kubectl apply -f -
```

echo "done"! note
    KCL outputs a single YAML document with a `manifests:` list. `kubectl apply` requires separate YAML documents - the Python snippet splits them correctly.

## Deployment profile

Configuration is passed via a YAML file using KCL's `kcl_options` format:

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
  # git-sync sidecar
  - key: config.gitSyncEnabled
    value: "true"
  - key: config.gitSyncRepo
    value: https://github.com/org/repo
  - key: config.gitSyncBranch
    value: main
  - key: config.gitSyncPeriod
    value: 60s
  # HTTPRoute (Gateway API)
  - key: config.httpRouteEnabled
    value: "true"
  - key: config.gatewayName
    value: my-gateway
  - key: config.gatewayNamespace
    value: default
  - key: config.hostname
    value: clusterscope
  - key: config.domain
    value: example.com
```

## Configuration reference

| Key | Default | Description |
|---|---|---|
| `config.name` | `clusterscope` | Resource name |
| `config.namespace` | `clusterscope` | Kubernetes namespace |
| `config.image` | `ghcr.io/stuttgart-things/clusterscope:latest` | Container image |
| `config.imagePullPolicy` | `IfNotPresent` | Image pull policy |
| `config.replicas` | `1` | Number of replicas |
| `config.port` | `8080` | HTTP serve port |
| `config.dir` | `/data` | Path inside container to scan for GitOps manifests |
| `config.tech` | `flux` | GitOps technology: `flux` or `argocd` |
| `config.gitSyncEnabled` | `false` | Enable git-sync sidecar |
| `config.gitSyncImage` | `registry.k8s.io/git-sync/git-sync:v4.4.0` | git-sync image |
| `config.gitSyncRepo` | `""` | Git repository URL to sync |
| `config.gitSyncBranch` | `main` | Branch/ref to sync |
| `config.gitSyncPeriod` | `60s` | Sync interval |
| `config.httpRouteEnabled` | `false` | Create HTTPRoute (Gateway API) |
| `config.gatewayName` | `""` | Gateway name to attach the HTTPRoute to |
| `config.gatewayNamespace` | `default` | Namespace of the Gateway |
| `config.hostname` | `""` | Hostname prefix (e.g. `clusterscope`) |
| `config.domain` | `""` | Domain (e.g. `example.com`) - URL: `hostname.domain` |

## git-sync sidecar

When `gitSyncEnabled: true`, a git-sync sidecar is added. Both containers share an `emptyDir` volume at `/data`:

```
pod
+-- container: clusterscope  (-root=/data/<repo>/clusters -tech=flux -serve=:8080)
|   +-- volumeMount: gitdata -> /data
+-- container: git-sync      (--repo=... --ref=main --root=/data --period=60s)
    +-- volumeMount: gitdata -> /data
```

git-sync creates: `/data/<repo-name> -> .worktrees/<hash>` (symlink).
Set `config.dir` to the subfolder inside the repo that contains the cluster directories, e.g. `/data/harvester/clusters`.

## HTTPRoute

When `httpRouteEnabled: true`, a [Gateway API](https://gateway-api.sigs.k8s.io/) `HTTPRoute` is generated pointing to the clusterscope Service:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: clusterscope
  namespace: clusterscope
spec:
  parentRefs:
    - name: <gatewayName>
      namespace: <gatewayNamespace>
  hostnames:
    - "<hostname>.<domain>"
  rules:
    - backendRefs:
        - name: clusterscope
          port: 8080
```

## Render only (inspect before applying)

```bash
# Render to stdout
kcl run kcl/ -Y tests/kcl-deploy-profile.yaml

# Non-interactive render via task
task render-manifests-quick
```

---

## Alternative: Deploy via OCI Kustomize artifact

Instead of rendering with KCL locally, you can deploy using the pre-built Kustomize OCI artifact published to `ghcr.io/stuttgart-things/clusterscope-kustomize`.

### Prerequisites

- [`oras`](https://oras.land/) CLI for pulling OCI artifacts

### Overlay structure

```
tests/kustomize-overlay/
├── kustomization.yaml      # references ./base + patches
├── deployment-patch.yaml   # strategic merge patch (git-sync sidecar, args)
└── httproute.yaml          # HTTPRoute (cluster-specific)
    base/                   # populated by task deploy-kustomize (git-ignored)
```

### Deploy via task

```bash
# Deploy (pulls OCI base + applies overlay)
KUBECONFIG_PATH=~/.kube/my-cluster task deploy-kustomize

# Deploy with custom OCI tag
OCI_TAG=v0.6.0 KUBECONFIG_PATH=~/.kube/my-cluster task deploy-kustomize

# Undeploy
KUBECONFIG_PATH=~/.kube/my-cluster task undeploy-kustomize
```

### Deploy manually

```bash
# 1. Pull OCI base
mkdir -p tests/kustomize-overlay/base
oras pull ghcr.io/stuttgart-things/clusterscope-kustomize:v0.6.0 \
  -o tests/kustomize-overlay/base

# 2. Apply overlay (base + deployment-patch + httproute)
kubectl apply -k tests/kustomize-overlay/
```

### Customize the overlay

**`deployment-patch.yaml`** — strategic merge patch to add git-sync sidecar and set `-root`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: clusterscope
  namespace: clusterscope
spec:
  template:
    spec:
      containers:
        - name: clusterscope
          args:
            - '-root=/data/<repo>/clusters'   # path inside synced repo
            - '-tech=flux'
            - '-serve=:8080'
        - name: git-sync
          image: registry.k8s.io/git-sync/git-sync:v4.4.0
          args:
            - '--repo=https://github.com/org/repo'
            - '--ref=main'
            - '--root=/data'
            - '--period=60s'
            - '--depth=1'
          ...
```

**`httproute.yaml`** — HTTPRoute (adapt gateway name and hostname):

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: clusterscope
  namespace: clusterscope
spec:
  parentRefs:
    - name: my-gateway
      namespace: default
  hostnames:
    - clusterscope.example.com
  rules:
    - backendRefs:
        - name: clusterscope
          port: 8080
```
