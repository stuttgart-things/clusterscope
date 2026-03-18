// Package argocd will parse ArgoCD Application, ApplicationSet and AppProject
// YAML files from a cluster directory and return a graph.ClusterProfile.
//
// This package is a placeholder — implementation is tracked in GitHub issues.
package argocd

import "github.com/stuttgart-things/clusterscope/internal/graph"

// ParseDir walks an ArgoCD cluster directory and returns a ClusterProfile.
// Currently returns an empty profile; full implementation is planned.
func ParseDir(dir string) (*graph.ClusterProfile, error) {
	return &graph.ClusterProfile{
		ClusterName: dir,
		ClusterPath: dir,
		Technology:  "argocd",
		Graph:       graph.Graph{},
		Meta:        map[string]string{},
	}, nil
}
