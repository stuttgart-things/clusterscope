package serve

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
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

	scanErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "clusterscope_scan_errors_total",
		Help: "Total number of cluster scan errors.",
	})
)
