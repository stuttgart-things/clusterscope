"""Common labels and config instance for clusterscope resources"""

import schema

# Get command-line options
_image = option("config.image")
_namespace = option("config.namespace")
_port = option("config.port")
_basePath = option("config.basePath")
_dir = option("config.dir")
_tech = option("config.tech")
_replicas = option("config.replicas")
_gitSyncEnabled = option("config.gitSyncEnabled")
_gitSyncImage = option("config.gitSyncImage")
_gitSyncRepo = option("config.gitSyncRepo")
_gitSyncBranch = option("config.gitSyncBranch")
_gitSyncPeriod = option("config.gitSyncPeriod")
_gitSyncAuthSecretName = option("config.gitSyncAuthSecretName")
_gitSyncAuthType = option("config.gitSyncAuthType")
_gitSyncRepos = option("config.gitSyncRepos")
_liveMode = option("config.liveMode")
_previewEnabled = option("config.previewEnabled")
_previewRepo = option("config.previewRepo")
_previewRef = option("config.previewRef")
_previewPeriod = option("config.previewPeriod")
_previewAuthSecretName = option("config.previewAuthSecretName")
_previewAuthType = option("config.previewAuthType")
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
    if _basePath:
        basePath = _basePath
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
    if _gitSyncAuthSecretName:
        gitSyncAuthSecretName = _gitSyncAuthSecretName
    if _gitSyncAuthType:
        gitSyncAuthType = _gitSyncAuthType
    if _gitSyncRepos:
        gitSyncRepos = _gitSyncRepos
    if _liveMode:
        liveMode = _liveMode == True or str(_liveMode) in ["true", "True", "1"]
    if _previewEnabled:
        previewEnabled = _previewEnabled == True or str(_previewEnabled) in ["true", "True", "1"]
    if _previewRepo:
        previewRepo = _previewRepo
    if _previewRef:
        previewRef = _previewRef
    if _previewPeriod:
        previewPeriod = _previewPeriod
    if _previewAuthSecretName:
        previewAuthSecretName = _previewAuthSecretName
    if _previewAuthType:
        previewAuthType = _previewAuthType
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
