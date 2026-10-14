package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stuttgart-things/clusterscope/internal/graph"
	"github.com/stuttgart-things/clusterscope/internal/live"
	"github.com/stuttgart-things/clusterscope/internal/probe"
)

func testEndpoints() *live.Endpoints {
	return &live.Endpoints{
		GatewayAPI: true,
		Gateways:   []live.Gateway{{Namespace: "default", Name: "lab-gw", Programmed: "True"}},
		Routes: []live.Route{
			{Namespace: "vault", Name: "vault", Hostnames: []string{"vault.lab"}, URLs: []string{"https://vault.lab"}, App: "vault", Accepted: "True", ResolvedRefs: "True"},
			{Namespace: "demo", Name: "api", Hostnames: []string{"*.demo.lab"}, Accepted: "False", ResolvedRefs: "True"},
		},
	}
}

func TestUpdateEndpoints_KeepsStaleWhenUnreachable(t *testing.T) {
	s := newTestServer(map[string]*graph.ClusterProfile{"prod": {}}, map[string]string{}, true)

	s.updateEndpoints("prod", live.ClusterStatus{Reachability: live.Reachable, Endpoints: testEndpoints(), CheckedAt: time.Now()})
	if got := testutil.ToFloat64(httprouteAccepted.WithLabelValues("prod", "vault", "vault")); got != 1 {
		t.Errorf("accepted metric vault: %v", got)
	}
	if got := testutil.ToFloat64(httprouteAccepted.WithLabelValues("prod", "demo", "api")); got != 0 {
		t.Errorf("accepted metric api: %v", got)
	}

	s.updateEndpoints("prod", live.ClusterStatus{Reachability: live.Unreachable})
	st, ok := s.endpoints["prod"]
	if !ok || !st.stale || len(st.ep.Routes) != 2 {
		t.Fatalf("expected last known routes kept as stale, got %+v (ok=%v)", st, ok)
	}
	if strings.Contains(metricDump(t), `clusterscope_httproute_accepted{cluster="prod"`) {
		t.Error("route metrics of an unreachable cluster must be removed")
	}
}

func metricDump(t *testing.T) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	promhttp.Handler().ServeHTTP(w, req)
	return w.Body.String()
}

func TestHandleAPIendpoints(t *testing.T) {
	s := newTestServer(map[string]*graph.ClusterProfile{"prod": {}, "dev": {}}, map[string]string{}, true)
	s.liveStatus["prod"] = live.ClusterStatus{Reachability: live.Reachable}
	s.liveStatus["dev"] = live.ClusterStatus{Reachability: live.Unconfigured}
	s.endpoints["prod"] = endpointState{ep: testEndpoints(), collectedAt: time.Now()}
	s.prober = probe.New(time.Second)
	s.probes["prod"] = map[string]probe.Result{"https://vault.lab": {URL: "https://vault.lab", OK: true, StatusCode: 200}}

	req := httptest.NewRequest(http.MethodGet, "/api/endpoints", nil)
	w := httptest.NewRecorder()
	s.handleAPIendpoints(w, req)

	var resp EndpointsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Clusters) != 2 || len(resp.Routes) != 2 || len(resp.Gateways) != 1 || !resp.Probing {
		t.Fatalf("unexpected response: %+v", resp)
	}
	var vault RouteRow
	for _, r := range resp.Routes {
		if r.Cluster != "prod" {
			t.Errorf("route row without cluster: %+v", r)
		}
		if r.Name == "vault" {
			vault = r
		}
	}
	if len(vault.Probes) != 1 || !vault.Probes[0].OK {
		t.Errorf("probe not attached: %+v", vault)
	}

	// cluster filter
	req = httptest.NewRequest(http.MethodGet, "/api/endpoints?cluster=dev", nil)
	w = httptest.NewRecorder()
	s.handleAPIendpoints(w, req)
	resp = EndpointsResponse{}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Clusters) != 1 || resp.Clusters[0].Reachability != live.Unconfigured || len(resp.Routes) != 0 {
		t.Errorf("filtered response: %+v", resp)
	}
}

func TestHandleAPItargets(t *testing.T) {
	s := newTestServer(map[string]*graph.ClusterProfile{"prod": {}}, map[string]string{}, true)
	s.liveStatus["prod"] = live.ClusterStatus{Reachability: live.Reachable}
	s.endpoints["prod"] = endpointState{ep: testEndpoints(), collectedAt: time.Now()}

	req := httptest.NewRequest(http.MethodGet, "/api/targets", nil)
	w := httptest.NewRecorder()
	s.handleAPItargets(w, req)

	var targets []httpSDTarget
	if err := json.NewDecoder(w.Body).Decode(&targets); err != nil {
		t.Fatal(err)
	}
	// wildcard-only route has no URL → exactly one target
	if len(targets) != 1 {
		t.Fatalf("targets: %+v", targets)
	}
	tg := targets[0]
	if tg.Targets[0] != "https://vault.lab" || tg.Labels["cluster"] != "prod" || tg.Labels["route"] != "vault" || tg.Labels["app"] != "vault" {
		t.Errorf("target: %+v", tg)
	}
}
