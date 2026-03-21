"""
Deployment for clusterscope
"""

import labels

config = labels.config

# ── Single-repo helpers ──────────────────────────────────────────────────────

# Conditional SSH arg for single-repo git-sync
_sshArgs = ["--ssh-key-file=/etc/git-sync-secrets/{}/ssh".format(config.gitSyncAuthSecretName)] if config.gitSyncAuthSecretName != "" and config.gitSyncAuthType == "ssh" else []

# Conditional HTTPS auth env vars for single-repo git-sync
_authEnv = [
    {
        name: "GITSYNC_USERNAME"
        valueFrom: {
            secretKeyRef: {
                name: config.gitSyncAuthSecretName
                key: "username"
            }
        }
    }
    {
        name: "GITSYNC_PASSWORD"
        valueFrom: {
            secretKeyRef: {
                name: config.gitSyncAuthSecretName
                key: "password"
            }
        }
    }
] if config.gitSyncAuthSecretName != "" and config.gitSyncAuthType == "https" else []

# Conditional auth volume mount (single-repo)
_authVolumeMount = [
    {
        name: "secret-auth"
        mountPath: "/etc/git-sync-secrets/{}".format(config.gitSyncAuthSecretName)
        readOnly: True
    }
] if config.gitSyncAuthSecretName != "" else []

# Conditional auth volume (single-repo)
_authVolume = [
    {
        name: "secret-auth"
        secret: {
            secretName: config.gitSyncAuthSecretName
        }
    }
] if config.gitSyncAuthSecretName != "" else []

# Single git-sync sidecar container (used when gitSyncEnabled=true and gitSyncRepos is empty)
_gitSyncContainer = {
    name: "git-sync"
    image: config.gitSyncImage
    args: [
        "--repo={}".format(config.gitSyncRepo)
        "--ref={}".format(config.gitSyncBranch)
        "--root=/data"
        "--period={}".format(config.gitSyncPeriod)
        "--depth=1"
    ] + _sshArgs
    env: [
        {
            name: "GITSYNC_ONE_TIME"
            value: "false"
        }
    ] + _authEnv
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
    ] + _authVolumeMount
}

# extra volumes for single git-sync (tmp + auth Secrets)
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
] + _authVolume

# ── Multi-repo helpers ───────────────────────────────────────────────────────

# Build one git-sync container per entry in gitSyncRepos
_multiGitSyncContainers = [
    {
        name: "git-sync-{}".format(r.subdir)
        image: config.gitSyncImage
        args: [
            "--repo={}".format(r.repo)
            "--ref={}".format(r.branch)
            "--root=/data/{}".format(r.subdir)
            "--period={}".format(r.period)
            "--depth=1"
        ] + (["--ssh-key-file=/etc/git-sync-secrets/{}/ssh".format(r.authSecretName)] if r.authSecretName != "" and r.authType == "ssh" else [])
        env: [
            {
                name: "GITSYNC_ONE_TIME"
                value: "false"
            }
        ] + ([
            {
                name: "GITSYNC_USERNAME"
                valueFrom: {
                    secretKeyRef: {
                        name: r.authSecretName
                        key: "username"
                    }
                }
            }
            {
                name: "GITSYNC_PASSWORD"
                valueFrom: {
                    secretKeyRef: {
                        name: r.authSecretName
                        key: "password"
                    }
                }
            }
        ] if r.authSecretName != "" and r.authType == "https" else [])
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
                name: "git-sync-tmp-{}".format(r.subdir)
                mountPath: "/tmp"
            }
        ] + ([
            {
                name: "secret-repo-{}".format(r.subdir)
                mountPath: "/etc/git-sync-secrets/{}".format(r.authSecretName)
                readOnly: True
            }
        ] if r.authSecretName != "" else [])
    }
    for r in config.gitSyncRepos
]

# Build volumes for each entry in gitSyncRepos
_multiGitSyncVolumes = [
    {
        name: "git-sync-tmp-{}".format(r.subdir)
        emptyDir: {}
    }
    for r in config.gitSyncRepos
] + [
    {
        name: "secret-repo-{}".format(r.subdir)
        secret: {
            secretName: r.authSecretName
        }
    }
    for r in config.gitSyncRepos if r.authSecretName != ""
]

# Choose between single-repo and multi-repo mode
_isMultiRepo = len(config.gitSyncRepos) > 0
_activeSyncContainers = _multiGitSyncContainers if _isMultiRepo else ([_gitSyncContainer] if config.gitSyncEnabled else [])
_activeSyncVolumes = _multiGitSyncVolumes if _isMultiRepo else (_gitSyncVolumes if config.gitSyncEnabled else [])


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
                ] + _activeSyncContainers
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
                ] + _activeSyncVolumes
            }
        }
    }
}
