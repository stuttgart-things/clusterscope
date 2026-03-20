"""
Deployment for clusterscope
"""

import labels

config = labels.config

# git-sync sidecar container (only added when gitSyncEnabled=true)
_gitSyncContainer = {
    name: "git-sync"
    image: config.gitSyncImage
    args: [
        "--repo={}".format(config.gitSyncRepo)
        "--ref={}".format(config.gitSyncBranch)
        "--root=/data"
        "--period={}".format(config.gitSyncPeriod)
        "--depth=1"
    ]
    env: [
        {
            name: "GITSYNC_ONE_TIME"
            value: "false"
        }
    ]
    securityContext: {
        readOnlyRootFilesystem: True
        runAsNonRoot: True
        runAsUser: 65533
        allowPrivilegeEscalation: False
        capabilities: {
            drop: ["ALL"]
        }
    }
    resources: {
        requests: {
            cpu: "10m"
            memory: "32Mi"
        }
        limits: {
            cpu: "100m"
            memory: "128Mi"
        }
    }
    volumeMounts: [
        {
            name: "gitdata"
            mountPath: "/data"
        }
        {
            name: "git-sync-tmp"
            mountPath: "/tmp"
        }
    ] + [
        {
            name: "secret-{}".format(s)
            mountPath: "/etc/git-sync-secrets/{}".format(s)
            readOnly: True
        }
        for s in config.repoAuthSecrets
    ]
}

# extra volumes for git-sync (tmp + auth Secrets)
_gitSyncVolumes = [
    {
        name: "git-sync-tmp"
        emptyDir: {}
    }
] + [
    {
        name: "secret-{}".format(s)
        secret: {
            secretName: s
        }
    }
    for s in config.repoAuthSecrets
]

deployment = {
    apiVersion: "apps/v1"
    kind: "Deployment"
    metadata: {
        name: config.name
        namespace: config.namespace
        labels: labels.commonLabels
    }
    spec: {
        replicas: config.replicas
        selector: {
            matchLabels: labels.selectorLabels
        }
        strategy: {
            type: "RollingUpdate"
            rollingUpdate: {
                maxSurge: 1
                maxUnavailable: 0
            }
        }
        template: {
            metadata: {
                labels: labels.commonLabels
            }
            spec: {
                serviceAccountName: config.name
                containers: [
                    {
                        name: config.name
                        image: config.image
                        imagePullPolicy: config.imagePullPolicy
                        args: [
                            "-root={}".format(config.dir)
                            "-tech={}".format(config.tech)
                            "-serve=:{}".format(config.port)
                        ]
                        ports: [
                            {
                                name: "http"
                                containerPort: config.port
                                protocol: "TCP"
                            }
                        ]
                        resources: {
                            requests: {
                                cpu: config.cpuRequest
                                memory: config.memoryRequest
                            }
                            limits: {
                                cpu: config.cpuLimit
                                memory: config.memoryLimit
                            }
                        }
                        securityContext: {
                            readOnlyRootFilesystem: True
                            runAsNonRoot: True
                            allowPrivilegeEscalation: False
                            capabilities: {
                                drop: ["ALL"]
                            }
                        }
                        livenessProbe: {
                            httpGet: {
                                path: "/"
                                port: config.port
                            }
                            initialDelaySeconds: 10
                            periodSeconds: 30
                        }
                        readinessProbe: {
                            httpGet: {
                                path: "/"
                                port: config.port
                            }
                            initialDelaySeconds: 5
                            periodSeconds: 10
                        }
                        volumeMounts: [
                            {
                                name: "tmp"
                                mountPath: "/tmp"
                            }
                            {
                                name: "gitdata"
                                mountPath: "/data"
                            }
                        ]
                    }
                ] + ([_gitSyncContainer] if config.gitSyncEnabled else [])
                affinity: {
                    podAntiAffinity: {
                        preferredDuringSchedulingIgnoredDuringExecution: [
                            {
                                weight: 100
                                podAffinityTerm: {
                                    labelSelector: {
                                        matchLabels: labels.selectorLabels
                                    }
                                    topologyKey: "kubernetes.io/hostname"
                                }
                            }
                        ]
                    }
                }
                securityContext: {
                    runAsNonRoot: True
                    runAsUser: 65532
                    runAsGroup: 65532
                    fsGroup: 65532
                    seccompProfile: {
                        type: "RuntimeDefault"
                    }
                }
                volumes: [
                    {
                        name: "tmp"
                        emptyDir: {}
                    }
                    {
                        name: "gitdata"
                        emptyDir: {}
                    }
                ] + (_gitSyncVolumes if config.gitSyncEnabled else [])
            }
        }
    }
}
