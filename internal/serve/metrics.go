package serve

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/stuttgart-things/clusterscope/internal/graph"
)

var (
	clustersTotal = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "clusterscope_clusters_total",
		Help: "Number of discovered clusters.",
	})

	clusterNodesTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "clusterscope_cluster_nodes_total",
		Help: "Number of graph nodes per cluster.",
	}, []string{"cluster", "tech"})

	clusterAppsTotal = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "clusterscope_cluster_apps_total",
		Help: "Number of app nodes per cluster.",
	}, []string{"cluster", "tech"})

	scanDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "clusterscope_scan_duration_seconds",
		Help:    "Time taken for a full directory scan.",
		Buckets: prometheus.DefBuckets,
	})

	liveEnrichDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "clusterscope_live_enrichment_duration_seconds",
		Help:    "Time taken for live status enrichment.",
		Buckets: prometheus.DefBuckets,
	})

	clusterReachable = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "clusterscope_cluster_reachable",
		Help: "1 if the cluster's API server answered during the last live refresh, 0 if not. Absent when no kubeconfig is configured for the cluster.",
	}, []string{"cluster"})

	httprouteAccepted = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "clusterscope_httproute_accepted",
		Help: "1 if all parent Gateways accepted the HTTPRoute, 0 if not (False or Unknown). Only for reachable clusters.",
	}, []string{"cluster", "namespace", "route"})

	appWorkloadStatus = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "clusterscope_app_workload_status",
		Help: "1 for the current actual workload health of a node (healthy, degraded, down, n/a), 0 for the others. Only for reachable clusters.",
	}, []string{"cluster", "app", "status"})

	appGitOpsDrift = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "clusterscope_app_gitops_drift",
		Help: "1 if the GitOps controller reports the node ready while its workloads or HelmReleases are not, else 0.",
	}, []string{"cluster", "app"})

	labFindings = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "clusterscope_lab_findings",
		Help: "Number of lab view findings (desired vs. actual mismatches) per lab, severity and kind.",
	}, []string{"lab", "severity", "kind"})

	clusterInfo = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "clusterscope_cluster_info",
		Help: "Always 1; maps a cluster to its lab. managed=false for clusters with a kubeconfig but no cluster directory.",
	}, []string{"cluster", "lab", "tech", "managed"})

	scanErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "clusterscope_scan_errors_total",
		Help: "Total number of cluster scan errors.",
	})
)

var workloadStatuses = []string{graph.WorkloadHealthy, graph.WorkloadDegraded, graph.WorkloadDown, graph.WorkloadNA}

// updateWorkloadMetrics replaces the workload series of one cluster.
func updateWorkloadMetrics(cluster string, p *graph.ClusterProfile) {
	appWorkloadStatus.DeletePartialMatch(prometheus.Labels{"cluster": cluster})
	appGitOpsDrift.DeletePartialMatch(prometheus.Labels{"cluster": cluster})
	for _, n := range p.Graph.Nodes {
		if n.Workloads == nil {
			continue
		}
		for _, st := range workloadStatuses {
			v := 0.0
			if n.Workloads.Status == st {
				v = 1
			}
			appWorkloadStatus.WithLabelValues(cluster, n.ID, st).Set(v)
		}
		drift := 0.0
		if n.Workloads.Drift {
			drift = 1
		}
		appGitOpsDrift.WithLabelValues(cluster, n.ID).Set(drift)
	}
}

// deleteClusterMetrics removes all per-cluster series of a removed cluster.
func deleteClusterMetrics(cluster string) {
	l := prometheus.Labels{"cluster": cluster}
	clusterNodesTotal.DeletePartialMatch(l)
	clusterAppsTotal.DeletePartialMatch(l)
	clusterReachable.DeletePartialMatch(l)
	httprouteAccepted.DeletePartialMatch(l)
	appWorkloadStatus.DeletePartialMatch(l)
	appGitOpsDrift.DeletePartialMatch(l)
}
