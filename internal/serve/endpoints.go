package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stuttgart-things/clusterscope/internal/live"
	"github.com/stuttgart-things/clusterscope/internal/probe"
	"github.com/stuttgart-things/clusterscope/internal/render"
)

// endpointState is the last known Gateway API inventory of a cluster.
type endpointState struct {
	ep          *live.Endpoints
	collectedAt time.Time
	// stale is true when the cluster was unreachable on the last refresh and
	// ep is from an earlier one.
	stale bool
}

// updateEndpoints stores the inventory from a live result, updates the route
// metrics and — with -probe — checks every route URL. While a cluster is
// unreachable its last known routes are kept (stale) and still probed: an
// unreachable API server does not mean the applications are down.
func (s *Server) updateEndpoints(name string, st live.ClusterStatus) {
	s.mu.Lock()
	prev, hadPrev := s.endpoints[name]
	var cur endpointState
	switch {
	case st.Endpoints != nil:
		cur = endpointState{ep: st.Endpoints, collectedAt: st.CheckedAt}
	case hadPrev:
		cur = prev
		cur.stale = true
	}
	if cur.ep != nil {
		s.endpoints[name] = cur
	} else {
		delete(s.endpoints, name)
	}
	s.mu.Unlock()

	httprouteAccepted.DeletePartialMatch(prometheus.Labels{"cluster": name})
	if cur.ep != nil && !cur.stale {
		for _, r := range cur.ep.Routes {
			v := 0.0
			if r.Accepted == "True" {
				v = 1
			}
			httprouteAccepted.WithLabelValues(name, r.Namespace, r.Name).Set(v)
		}
	}

	if s.prober == nil {
		return
	}
	var urls []string
	if cur.ep != nil {
		for _, r := range cur.ep.Routes {
			urls = append(urls, r.URLs...)
		}
	}
	results := s.prober.ProbeAll(context.Background(), urls)
	s.mu.Lock()
	s.probes[name] = results
	s.mu.Unlock()
}

// ── API payloads ─────────────────────────────────────────────────────────────

// EndpointsResponse is the JSON payload of GET /api/endpoints.
type EndpointsResponse struct {
	Clusters []EndpointCluster `json:"clusters"`
	Gateways []GatewayRow      `json:"gateways"`
	Routes   []RouteRow        `json:"routes"`
	Probing  bool              `json:"probing"`
}

// EndpointCluster is the per-cluster collection state.
type EndpointCluster struct {
	Name         string `json:"name"`
	Reachability string `json:"reachability,omitempty"`
	GatewayAPI   bool   `json:"gatewayAPI"`
	Stale        bool   `json:"stale,omitempty"`
	CollectedAt  string `json:"collectedAt,omitempty"`
	Error        string `json:"error,omitempty"`
}

// GatewayRow is a Gateway with its cluster.
type GatewayRow struct {
	Cluster string `json:"cluster"`
	live.Gateway
}

// RouteRow is an HTTPRoute with its cluster and probe results.
type RouteRow struct {
	Cluster string `json:"cluster"`
	Stale   bool   `json:"stale,omitempty"`
	live.Route
	Probes []probe.Result `json:"probes,omitempty"`
}

func (s *Server) endpointsSnapshot(cluster string) EndpointsResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	resp := EndpointsResponse{
		Clusters: []EndpointCluster{},
		Gateways: []GatewayRow{},
		Routes:   []RouteRow{},
		Probing:  s.prober != nil,
	}

	names := make([]string, 0, len(s.liveStatus))
	for n := range s.liveStatus {
		if cluster == "" || n == cluster {
			names = append(names, n)
		}
	}
	sort.Strings(names)

	for _, name := range names {
		ec := EndpointCluster{Name: name, Reachability: s.liveStatus[name].Reachability}
		state, ok := s.endpoints[name]
		if ok {
			ec.GatewayAPI = state.ep.GatewayAPI
			ec.Stale = state.stale
			ec.CollectedAt = state.collectedAt.Format(time.RFC3339)
			ec.Error = state.ep.Error
			for _, g := range state.ep.Gateways {
				resp.Gateways = append(resp.Gateways, GatewayRow{Cluster: name, Gateway: g})
			}
			for _, r := range state.ep.Routes {
				row := RouteRow{Cluster: name, Stale: state.stale, Route: r}
				for _, u := range r.URLs {
					if res, ok := s.probes[name][u]; ok {
						row.Probes = append(row.Probes, res)
					}
				}
				resp.Routes = append(resp.Routes, row)
			}
		}
		resp.Clusters = append(resp.Clusters, ec)
	}
	return resp
}

// handleAPIendpoints serves GET /api/endpoints[?cluster=<name>].
func (s *Server) handleAPIendpoints(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.endpointsSnapshot(r.URL.Query().Get("cluster")))
}

// httpSDTarget is one target group in Prometheus http_sd format.
type httpSDTarget struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels"`
}

// handleAPItargets serves GET /api/targets[?cluster=<name>] in Prometheus
// http_sd format: one target group per route URL, for blackbox-exporter.
func (s *Server) handleAPItargets(w http.ResponseWriter, r *http.Request) {
	snap := s.endpointsSnapshot(r.URL.Query().Get("cluster"))
	targets := []httpSDTarget{}
	for _, rt := range snap.Routes {
		for _, u := range rt.URLs {
			labels := map[string]string{
				"cluster":   rt.Cluster,
				"namespace": rt.Namespace,
				"route":     rt.Name,
			}
			if rt.App != "" {
				labels["app"] = rt.App
			}
			targets = append(targets, httpSDTarget{Targets: []string{u}, Labels: labels})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(targets)
}

// handleEndpoints serves the endpoints dashboard page.
func (s *Server) handleEndpoints(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := render.WriteEndpoints(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
