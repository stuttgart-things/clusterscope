"""
Service for clusterscope HTTP dashboard
"""

import labels

config = labels.config

service = {
    apiVersion: "v1"
    kind: "Service"
    metadata: {
        name: config.name
        namespace: config.namespace
        labels: labels.commonLabels
    }
    spec: {
        selector: labels.selectorLabels
        ports: [
            {
                name: "http"
                port: config.port
                targetPort: config.port
                protocol: "TCP"
            }
        ]
        type: "ClusterIP"
    }
}
