# Configuration

## repos.yaml (git-sync mode)

When deploying clusterscope in Kubernetes with a git-sync sidecar, a `repos.yaml` file configures which Git repositories are synced to the `-root` directory.

Pass the file path via `-repos-config`:

```bash
clusterscope -serve :8080 -root /data -repos-config /etc/git-sync/repos.yaml
```

### Schema

```yaml
repos:
  - name: prod-clusters
    url: https://github.com/my-org/gitops-clusters.git
    branch: main
    path: clusters/prod
    auth:
      secretRef: gitops-repo-secret

  - name: staging-clusters
    url: https://github.com/my-org/gitops-clusters.git
    branch: staging
    path: clusters/staging
```

### Fields

| Field | Required | Description |
|---|---|---|
| `name` | ✓ | Unique identifier for the repository |
| `url` | ✓ | Git repository URL |
| `branch` | ✓ | Branch to sync |
| `path` | — | Subdirectory within the repo to use as cluster root |
| `auth.secretRef` | — | Name of the Kubernetes Secret containing credentials |

!!! note
    The `repos-config` flag only informs clusterscope of which repos are configured. The actual git-sync is performed by the git-sync sidecar container — clusterscope only reads the synced files from disk.
