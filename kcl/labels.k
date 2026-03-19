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
