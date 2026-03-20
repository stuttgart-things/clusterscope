"""
HTTPRoute resource for clusterscope (Gateway API)
Only rendered when config.httpRouteEnabled is True
"""

import labels as l

config = l.config

_httpRoute = None

if config.httpRouteEnabled:
    _httpRoute = {
        apiVersion: "gateway.networking.k8s.io/v1"
        kind: "HTTPRoute"
        metadata: {
            name: config.name
            namespace: config.namespace
            labels: l.commonLabels
        }
        spec: {
            parentRefs: [
                {
                    name: config.gatewayName
                    namespace: config.gatewayNamespace
                }
            ]
            hostnames: [
                "{}.{}".format(config.hostname, config.domain)
            ]
            rules: [
                {
                    matches: [
                        {
                            path: {
                                type: "PathPrefix"
                                value: "/"
                            }
                        }
                    ]
                    backendRefs: [
                        {
                            name: config.name
                            port: config.port
                        }
                    ]
                }
            ]
        }
    }

httpRoute = _httpRoute
