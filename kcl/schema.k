"""
Schema definition for clusterscope deployment configuration
"""

schema GitSyncRepo:
    """Configuration for a single git-sync sidecar watching one repository"""
    # Git repository URL to sync
    repo: str
    # Branch to sync
    branch: str = "main"
    # Subdirectory under /data/ where the repo is synced (e.g. "harvester", "st-clusters")
    subdir: str
    # Sync interval (e.g. 60s, 5m)
    period: str = "60s"
    # Name of the Kubernetes Secret containing auth credentials (optional)
    # For HTTPS: secret must have keys 'username' and 'password'
    # For SSH:   secret must have key 'ssh' (private key)
    authSecretName: str = ""
    # Auth type: 'https' (token/password) or 'ssh'
    authType: str = "https"

    check:
        repo != "", "repo must not be empty"
        subdir != "", "subdir must not be empty"
        authType in ["https", "ssh"], "authType must be 'https' or 'ssh'"

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

    # Single repo (legacy / simple mode) - mutually exclusive with gitSyncRepos
    # Git repository URL to sync
    gitSyncRepo: str = ""
    # Branch to sync
    gitSyncBranch: str = "main"
    # Sync interval (e.g. 60s, 5m)
    gitSyncPeriod: str = "60s"

    # Multi-repo mode: list of repositories to sync simultaneously.
    # Each entry creates a dedicated git-sync sidecar syncing into /data/<subdir>/
    # When set, gitSyncRepo/gitSyncBranch/gitSyncPeriod are ignored.
    gitSyncRepos: [GitSyncRepo] = []

    # Names of Kubernetes Secrets containing auth credentials for git-sync.
    # Each secret is mounted at /etc/git-sync-secrets/<secret-name>/
    repoAuthSecrets: [str] = []

    # Authentication for private git repositories (single-repo mode)
    # Name of the Kubernetes Secret containing auth credentials
    # For HTTPS: secret must have keys 'username' and 'password'
    # For SSH:   secret must have key 'ssh' (private key)
    gitSyncAuthSecretName: str = ""
    # Auth type: 'https' (token/password) or 'ssh'
    gitSyncAuthType: str = "https"

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
        gitSyncAuthType in ["https", "ssh"], "gitSyncAuthType must be 'https' or 'ssh'"
