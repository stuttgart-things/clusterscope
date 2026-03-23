package serve

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stuttgart-things/clusterscope/internal/graph"
)

func TestHandleHealthz(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	s.handleHealthz(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if body := w.Body.String(); body != "ok" {
		t.Errorf("expected body %q, got %q", "ok", body)
	}
}

func TestHandleReadyz_NotReady(t *testing.T) {
	s := &Server{ready: false}
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	s.handleReadyz(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleReadyz_Ready(t *testing.T) {
	s := &Server{ready: true}
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	s.handleReadyz(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if body := w.Body.String(); body != "ok" {
		t.Errorf("expected body %q, got %q", "ok", body)
	}
}

func newTestServerWithPreview(profiles map[string]*graph.ClusterProfile, preview map[string]*graph.ClusterProfile) *Server {
	s := newTestServer(profiles, map[string]string{}, false)
	s.previewCache = preview
	s.opts.PreviewRoot = "/tmp/preview" // non-empty to indicate preview is enabled
	return s
}

func newTestServer(profiles map[string]*graph.ClusterProfile, errs map[string]string, liveMode bool) *Server {
	scanned := make(map[string]time.Time)
	for name := range profiles {
		scanned[name] = time.Now()
	}
	for name := range errs {
		scanned[name] = time.Now()
	}
	return &Server{
		cache:     profiles,
		scanErrs:  errs,
		scanned:   scanned,
		refreshed: make(map[string]time.Time),
		opts:      Options{Live: liveMode},
	}
}

func TestHandleAPIclusters_Basic(t *testing.T) {
	profiles := map[string]*graph.ClusterProfile{
		"movie-scripts": {
			Technology: "flux",
			Graph: graph.Graph{
				Nodes: []graph.Node{
					{ID: "src1", Type: "source"},
					{ID: "app1", Type: "apps"},
					{ID: "app2", Type: "apps"},
				},
			},
		},
	}
	s := newTestServer(profiles, map[string]string{}, false)

	req := httptest.NewRequest(http.MethodGet, "/api/clusters", nil)
	w := httptest.NewRecorder()
	s.handleAPIclusters(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var summaries []ClusterSummary
	if err := json.NewDecoder(w.Body).Decode(&summaries); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	cs := summaries[0]
	if cs.Name != "movie-scripts" {
		t.Errorf("name: got %q", cs.Name)
	}
	if cs.Tech != "flux" {
		t.Errorf("tech: got %q", cs.Tech)
	}
	if cs.NodeCount != 3 {
		t.Errorf("nodeCount: got %d", cs.NodeCount)
	}
	if cs.AppCount != 2 {
		t.Errorf("appCount: got %d, want 2", cs.AppCount)
	}
	if cs.ScanError != "" {
		t.Errorf("unexpected scanError: %q", cs.ScanError)
	}
}

func TestHandleAPIclusters_ScanError(t *testing.T) {
	s := newTestServer(map[string]*graph.ClusterProfile{}, map[string]string{
		"broken-cluster": "failed to parse YAML: unexpected EOF",
	}, false)

	req := httptest.NewRequest(http.MethodGet, "/api/clusters", nil)
	w := httptest.NewRecorder()
	s.handleAPIclusters(w, req)

	var summaries []ClusterSummary
	_ = json.NewDecoder(w.Body).Decode(&summaries)
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	if summaries[0].ScanError == "" {
		t.Error("expected scanError to be set")
	}
}

func TestHandleAPIclusters_HealthSummary(t *testing.T) {
	profiles := map[string]*graph.ClusterProfile{
		"prod": {
			Technology: "flux",
			Graph: graph.Graph{
				Nodes: []graph.Node{
					{ID: "a", Type: "apps", Status: "ready"},
					{ID: "b", Type: "apps", Status: "ready"},
					{ID: "c", Type: "apps", Status: "failed"},
				},
			},
		},
	}
	s := newTestServer(profiles, map[string]string{}, true)
	// Manually set mu so RLock works
	s.mu = sync.RWMutex{}

	req := httptest.NewRequest(http.MethodGet, "/api/clusters", nil)
	w := httptest.NewRecorder()
	s.handleAPIclusters(w, req)

	var summaries []ClusterSummary
	_ = json.NewDecoder(w.Body).Decode(&summaries)
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	h := summaries[0].HealthSummary
	if h == nil {
		t.Fatal("healthSummary should be set in live mode")
	}
	if h["ready"] != 2 {
		t.Errorf("ready: got %d, want 2", h["ready"])
	}
	if h["failed"] != 1 {
		t.Errorf("failed: got %d, want 1", h["failed"])
	}
}

func TestMetricsEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	promhttp.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	for _, metric := range []string{
		"clusterscope_clusters_total",
		"clusterscope_scan_duration_seconds",
		"clusterscope_live_enrichment_duration_seconds",
		"clusterscope_scan_errors_total",
	} {
		if !strings.Contains(body, metric) {
			t.Errorf("expected metric %q in /metrics output", metric)
		}
	}
}

// ── Preview endpoint tests ───────────────────────────────────────────────────

func TestHandleAPIpreview_Diff(t *testing.T) {
	base := map[string]*graph.ClusterProfile{
		"prod": {
			Technology: "flux",
			Graph: graph.Graph{
				Nodes: []graph.Node{
					{ID: "cert-manager", Type: "infra", Version: "1.14.0"},
					{ID: "ingress-nginx", Type: "infra"},
				},
				Edges: []graph.Edge{
					{S: "cert-manager", T: "ingress-nginx", Kind: "dependsOn"},
				},
			},
		},
	}
	head := map[string]*graph.ClusterProfile{
		"prod": {
			Technology: "flux",
			Graph: graph.Graph{
				Nodes: []graph.Node{
					{ID: "cert-manager", Type: "infra", Version: "1.15.0"},
					{ID: "ingress-nginx", Type: "infra"},
					{ID: "monitoring", Type: "apps"},
				},
				Edges: []graph.Edge{
					{S: "cert-manager", T: "ingress-nginx", Kind: "dependsOn"},
					{S: "cert-manager", T: "monitoring", Kind: "dependsOn"},
				},
			},
		},
	}
	s := newTestServerWithPreview(base, head)

	req := httptest.NewRequest(http.MethodGet, "/api/preview/prod", nil)
	w := httptest.NewRecorder()
	s.handleAPIpreview(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result struct {
		ClusterName string `json:"clusterName"`
		Summary     struct {
			Added    int `json:"added"`
			Removed  int `json:"removed"`
			Modified int `json:"modified"`
			Same     int `json:"same"`
		} `json:"summary"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.ClusterName != "prod" {
		t.Errorf("clusterName: got %q", result.ClusterName)
	}
	if result.Summary.Added != 1 {
		t.Errorf("added: got %d, want 1 (monitoring)", result.Summary.Added)
	}
	if result.Summary.Modified != 1 {
		t.Errorf("modified: got %d, want 1 (cert-manager version)", result.Summary.Modified)
	}
	if result.Summary.Same != 1 {
		t.Errorf("same: got %d, want 1 (ingress-nginx)", result.Summary.Same)
	}
}

func TestHandleAPIpreview_NotFound(t *testing.T) {
	s := newTestServerWithPreview(
		map[string]*graph.ClusterProfile{},
		map[string]*graph.ClusterProfile{},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/preview/nonexistent", nil)
	w := httptest.NewRecorder()
	s.handleAPIpreview(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleAPIpreview_TraversalBlocked(t *testing.T) {
	s := newTestServerWithPreview(
		map[string]*graph.ClusterProfile{},
		map[string]*graph.ClusterProfile{},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/preview/../etc/passwd", nil)
	w := httptest.NewRecorder()
	s.handleAPIpreview(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for traversal, got %d", w.Code)
	}
}

func TestHandlePreview_RendersHTML(t *testing.T) {
	base := map[string]*graph.ClusterProfile{
		"staging": {
			Technology: "flux",
			Graph: graph.Graph{
				Nodes: []graph.Node{{ID: "app1", Type: "apps"}},
			},
		},
	}
	head := map[string]*graph.ClusterProfile{
		"staging": {
			Technology: "flux",
			Graph: graph.Graph{
				Nodes: []graph.Node{
					{ID: "app1", Type: "apps"},
					{ID: "app2", Type: "apps"},
				},
			},
		},
	}
	s := newTestServerWithPreview(base, head)

	req := httptest.NewRequest(http.MethodGet, "/preview/staging", nil)
	w := httptest.NewRecorder()
	s.handlePreview(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "PR preview") {
		t.Error("expected diff template to contain 'PR preview'")
	}
	if !strings.Contains(body, "staging") {
		t.Error("expected cluster name 'staging' in output")
	}
}
