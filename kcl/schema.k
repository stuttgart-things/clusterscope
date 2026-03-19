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
    cpuLimit: str = "200m"
    memoryRequest: str = "64Mi"
    memoryLimit: str = "256Mi"

    # Serve mode port
    port: int = 8080

    # Directory to scan for GitOps manifests (mapped via git-sync in k8s mode)
    dir: str = "/data"

    # Technology: flux | argocd
    tech: str = "flux"

    # Labels and annotations
    labels: {str:str} = {}
    annotations: {str:str} = {}

    check:
        replicas >= 0, "replicas must be >= 0"
        name != "", "name must not be empty"
        namespace != "", "namespace must not be empty"
        tech in ["flux", "argocd"], "tech must be one of: flux, argocd"
