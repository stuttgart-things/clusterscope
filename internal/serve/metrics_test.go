package serve

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stuttgart-things/clusterscope/internal/graph"
)

func TestUpdateWorkloadMetrics(t *testing.T) {
	p := &graph.ClusterProfile{Graph: graph.Graph{Nodes: []graph.Node{
		{ID: "web", Workloads: &graph.WorkloadHealth{Status: graph.WorkloadDegraded, Drift: true}},
		{ID: "config", Workloads: &graph.WorkloadHealth{Status: graph.WorkloadNA}},
		{ID: "source"},
	}}}
	updateWorkloadMetrics("wl-test", p)

	if got := testutil.ToFloat64(appWorkloadStatus.WithLabelValues("wl-test", "web", "degraded")); got != 1 {
		t.Errorf("web degraded = %v", got)
	}
	if got := testutil.ToFloat64(appWorkloadStatus.WithLabelValues("wl-test", "web", "healthy")); got != 0 {
		t.Errorf("web healthy = %v", got)
	}
	if got := testutil.ToFloat64(appGitOpsDrift.WithLabelValues("wl-test", "web")); got != 1 {
		t.Errorf("web drift = %v", got)
	}

	// Next refresh: web recovered, config gone → its series disappear.
	p.Graph.Nodes = p.Graph.Nodes[:1]
	p.Graph.Nodes[0].Workloads = &graph.WorkloadHealth{Status: graph.WorkloadHealthy}
	updateWorkloadMetrics("wl-test", p)
	if got := testutil.ToFloat64(appGitOpsDrift.WithLabelValues("wl-test", "web")); got != 0 {
		t.Errorf("web drift after recovery = %v", got)
	}
	// Only web's 4 status series are left.
	if n := testutil.CollectAndCount(appWorkloadStatus, "clusterscope_app_workload_status"); n != 4 {
		t.Errorf("status series = %d, want 4", n)
	}
}
