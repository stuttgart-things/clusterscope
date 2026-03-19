"""
Main entry point for clusterscope Kubernetes deployment
Imports all resources and exports manifests
"""

import namespace
import serviceaccount
import clusterrole
import service
import configmaprepos as cmrepos
import deploy

# Export all manifests (filter out None values)
manifests = [
    m for m in [
        namespace.namespace
        serviceaccount.serviceAccount
        clusterrole.clusterRole
        clusterrole.clusterRoleBinding
        service.service
        cmrepos.configMapRepos
        deploy.deployment
    ] if m
]
