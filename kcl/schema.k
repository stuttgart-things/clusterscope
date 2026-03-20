"""
Schema definition for clusterscope deployment configuration
"""

schema ClusterScope:
    """Configuration for clusterscope Kubernetes deployment"""

    # Metadata
    name: str = "clusterscope"
    namespace: str = "clusterscope"

    # Image
    image: str = "ghcr.io/stuttgart-things/clusterscope:latest"
    imagePullPolicy: str = "IfNotPresent"

    # Replicas and resources
    replicas: int = 1
    cpuRequest: str = "50m"
    cpuLimit: str = "500m"
    memoryRequest: str = "128Mi"
    memoryLimit: str = "512Mi"

    # Serve mode port
    port: int = 8080

    # Directory to scan for GitOps manifests (mapped via git-sync in k8s mode)
    dir: str = "/data"

    # Technology: flux | argocd
    tech: str = "flux"

    # Git-sync sidecar (Kubernetes serve mode)
    gitSyncEnabled: bool = False
    gitSyncImage: str = "registry.k8s.io/git-sync/git-sync:v4.4.0"

    # Git repository URL to sync
    gitSyncRepo: str = ""
    # Branch to sync
    gitSyncBranch: str = "main"
    # Sync interval (e.g. 60s, 5m)
    gitSyncPeriod: str = "60s"

    # Names of Kubernetes Secrets containing auth credentials for git-sync.
    # Each secret is mounted at /etc/git-sync-secrets/<secret-name>/
    repoAuthSecrets: [str] = []

    # HTTPRoute (Gateway API)
    httpRouteEnabled: bool = False
    gatewayName: str = ""
    gatewayNamespace: str = "default"
    hostname: str = ""
    domain: str = ""

    # Labels and annotations
    labels: {str:str} = {}
    annotations: {str:str} = {}

    check:
        replicas >= 0, "replicas must be >= 0"
        name != "", "name must not be empty"
        namespace != "", "namespace must not be empty"
        tech in ["flux", "argocd"], "tech must be one of: flux, argocd"
