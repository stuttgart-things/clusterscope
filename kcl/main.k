"""
Main entry point for clusterscope Kubernetes deployment
Imports all resources and exports manifests
"""

import namespace
import serviceaccount
import clusterrole
import service
import deploy

# Export all manifests (filter out None values)
manifests = [
    m for m in [
        namespace.namespace
        serviceaccount.serviceAccount
        clusterrole.clusterRole
        clusterrole.clusterRoleBinding
        service.service
        deploy.deployment
    ] if m
]
