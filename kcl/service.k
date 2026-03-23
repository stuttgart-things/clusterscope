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
        annotations: {
            "prometheus.io/scrape": "true"
            "prometheus.io/port": str(config.port)
            "prometheus.io/path": "/metrics"
        }
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
