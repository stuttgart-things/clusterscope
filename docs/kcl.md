# KCL Deployment

clusterscope ships with a [KCL](https://kcl-lang.io/) deployment module that generates the Kubernetes manifests for running it as a server with an optional git-sync sidecar.

## Render manifests

```bash
# Interactive (uses gum for prompts)
task render-manifests

# Non-interactive with defaults
task render-manifests-quick

# With custom profile
task render-manifests-quick PROFILE=tests/kcl-deploy-profile.yaml
```

## Deployment profile

```yaml
# tests/kcl-deploy-profile.yaml
config:
  image: ghcr.io/stuttgart-things/clusterscope:latest
  replicas: 1
  namespace: clusterscope
  serviceType: ClusterIP
  rootPath: /data/clusters
  servePort: "8080"

  # git-sync sidecar
  gitSyncEnabled: true
  gitSyncImage: registry.k8s.io/git-sync/git-sync:v4.4.0
  reposConfig: /etc/git-sync/repos.yaml

  repoAuthSecrets:
    - gitops-repo-secret

labels:
  app: clusterscope
  version: latest
```

## Generated resources

| Resource | Description |
|---|---|
| `Deployment` | clusterscope server + optional git-sync sidecar |
| `Service` | Exposes the HTTP dashboard |
| `ConfigMap` | repos.yaml configuration |

## git-sync sidecar

When `gitSyncEnabled: true`, a git-sync container is added alongside the clusterscope container:

- Mounts a shared `emptyDir` volume at `/data/clusters`
- Reads `repos.yaml` from the ConfigMap mounted at `/etc/git-sync/`
- Periodically syncs the configured Git repositories into the shared volume
- clusterscope reads cluster YAML files from the synced directories

## Kustomize OCI push

```bash
task push-kustomize-oci
```

Pushes the rendered manifests as an OCI artifact to `ghcr.io/stuttgart-things/clusterscope-kustomize`.
