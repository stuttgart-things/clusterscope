"""
ClusterRole and ClusterRoleBinding for clusterscope
Grants read access to cluster resources and GitOps CRDs
"""

import labels

config = labels.config

clusterRole = {
    apiVersion: "rbac.authorization.k8s.io/v1"
    kind: "ClusterRole"
    metadata: {
        name: config.name
        labels: labels.commonLabels
    }
    rules: [
        # Core resources
        {
            apiGroups: [""]
            resources: ["nodes", "namespaces", "pods", "configmaps"]
            verbs: ["get", "list", "watch"]
        }
        # Apps resources
        {
            apiGroups: ["apps"]
            resources: ["deployments", "replicasets", "statefulsets", "daemonsets"]
            verbs: ["get", "list", "watch"]
        }
        # Flux CRDs (kustomize.toolkit.fluxcd.io)
        {
            apiGroups: ["kustomize.toolkit.fluxcd.io"]
            resources: ["kustomizations"]
            verbs: ["get", "list", "watch"]
        }
        # Flux CRDs (source.toolkit.fluxcd.io)
        {
            apiGroups: ["source.toolkit.fluxcd.io"]
            resources: ["gitrepositories", "ocirepositories", "helmrepositories", "helmcharts"]
            verbs: ["get", "list", "watch"]
        }
        # Flux instance (fluxcd.controlplane.io)
        {
            apiGroups: ["fluxcd.controlplane.io"]
            resources: ["fluxinstances"]
            verbs: ["get", "list", "watch"]
        }
        # ArgoCD CRDs (argoproj.io)
        {
            apiGroups: ["argoproj.io"]
            resources: ["applications", "applicationsets", "appprojects"]
            verbs: ["get", "list", "watch"]
        }
    ]
}

clusterRoleBinding = {
    apiVersion: "rbac.authorization.k8s.io/v1"
    kind: "ClusterRoleBinding"
    metadata: {
        name: config.name
        labels: labels.commonLabels
    }
    roleRef: {
        apiGroup: "rbac.authorization.k8s.io"
        kind: "ClusterRole"
        name: config.name
    }
    subjects: [
        {
            kind: "ServiceAccount"
            name: config.name
            namespace: config.namespace
        }
    ]
}
