"""
Common labels and config instance for clusterscope resources
"""

import schema

# Get command-line options
_image = option("config.image")
_namespace = option("config.namespace")
_port = option("config.port")
_dir = option("config.dir")
_tech = option("config.tech")
_replicas = option("config.replicas")
_gitSyncEnabled = option("config.gitSyncEnabled")
_gitSyncImage = option("config.gitSyncImage")
_gitSyncRepo = option("config.gitSyncRepo")
_gitSyncBranch = option("config.gitSyncBranch")
_gitSyncPeriod = option("config.gitSyncPeriod")
_httpRouteEnabled = option("config.httpRouteEnabled")
_gatewayName = option("config.gatewayName")
_gatewayNamespace = option("config.gatewayNamespace")
_hostname = option("config.hostname")
_domain = option("config.domain")

# Config instance with command-line overrides
config: schema.ClusterScope = schema.ClusterScope {
    if _image:
        image = _image
    if _namespace:
        namespace = _namespace
    if _port:
        port = int(_port)
    if _dir:
        dir = _dir
    if _tech:
        tech = _tech
    if _replicas:
        replicas = int(_replicas)
    if _gitSyncEnabled:
        gitSyncEnabled = _gitSyncEnabled == True or str(_gitSyncEnabled) in ["true", "True", "1"]
    if _gitSyncImage:
        gitSyncImage = _gitSyncImage
    if _gitSyncRepo:
        gitSyncRepo = _gitSyncRepo
    if _gitSyncBranch:
        gitSyncBranch = _gitSyncBranch
    if _gitSyncPeriod:
        gitSyncPeriod = _gitSyncPeriod
    if _httpRouteEnabled:
        httpRouteEnabled = _httpRouteEnabled == True or str(_httpRouteEnabled) in ["true", "True", "1"]
    if _gatewayName:
        gatewayName = _gatewayName
    if _gatewayNamespace:
        gatewayNamespace = _gatewayNamespace
    if _hostname:
        hostname = _hostname
    if _domain:
        domain = _domain
}

# Common labels applied to all resources
commonLabels = {
    "app.kubernetes.io/name": config.name
    "app.kubernetes.io/instance": config.name
    "app.kubernetes.io/component": "visualizer"
    "app.kubernetes.io/part-of": "clusterscope"
    **config.labels
}

# Selector labels for pods
selectorLabels = {
    "app.kubernetes.io/name": config.name
    "app.kubernetes.io/instance": config.name
}
