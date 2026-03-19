"""
ConfigMap for clusterscope git-sync repos configuration.
Created only when config.reposConfig is set.
The content is the repos.yaml file consumed by the git-sync sidecar.
"""

import labels

config = labels.config

configMapRepos = {
    apiVersion: "v1"
    kind: "ConfigMap"
    metadata: {
        name: "{}-repos".format(config.name)
        namespace: config.namespace
        labels: labels.commonLabels
    }
    data: {
        "repos.yaml": config.reposConfig
    }
} if config.reposConfig else None
