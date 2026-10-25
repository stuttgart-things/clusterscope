package serve

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/stuttgart-things/clusterscope/internal/graph"
)

func TestParseBasePath(t *testing.T) {
	for in, want := range map[string]string{
		"": "", "/": "", "/clusterscope": "/clusterscope", "/clusterscope/": "/clusterscope", " /lab/scope": "/lab/scope",
	} {
		got, err := ParseBasePath(in)
		if err != nil || got != want {
			t.Errorf("ParseBasePath(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"clusterscope", "/a//b", "/a/../b", "/a b", "/a?x", "/a#x", "/a%2f", `/a"b`} {
		if got, err := ParseBasePath(in); err == nil {
			t.Errorf("ParseBasePath(%q) = %q, want an error", in, got)
		}
	}
}

func portalTestServer(basePath string) *Server {
	s := newTestServer(map[string]*graph.ClusterProfile{
		"dev": {Technology: "flux", Graph: graph.Graph{Nodes: []graph.Node{{ID: "app1", Type: "apps"}}}},
	}, map[string]string{}, false)
	s.ready = true
	s.opts.BasePath = basePath
	return s
}

func get(h http.Handler, path string, header ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// absoluteURL finds URLs a served page hands to the browser that would
// bypass <base href>: absolute paths in attributes, fetch(), pushState and
// iframe/location targets built in JavaScript.
var absoluteURL = regexp.MustCompile(`(?:href|src|action)="(/[^"/][^"]*)"|fetch\(["'\x60](/[^"'\x60]*)|pushState\([^,]*,\s*"[^"]*",\s*["'\x60](/[^"'\x60]*)|src="/'|location\.href\s*=\s*["'\x60](/)`)

func TestHandlerWithoutBasePath(t *testing.T) {
	h := portalTestServer("").handler()
	for _, path := range []string{"/", "/health", "/healthz", "/readyz", "/api/clusters", "/clusters/dev", "/lab", "/endpoints"} {
		if w := get(h, path); w.Code != http.StatusOK {
			t.Errorf("GET %s: got %d", path, w.Code)
		}
	}
	if body := get(h, "/").Body.String(); !strings.Contains(body, `<base href="/">`) {
		t.Error(`shell: expected <base href="/">`)
	}
}

// TestHandlerBasePath covers the sthings-portal prefix contract: everything
// answers under the prefix, probes and the API at the root too, and no
// served page hands out a URL that ignores <base href>.
func TestHandlerBasePath(t *testing.T) {
	h := portalTestServer("/clusterscope").handler()

	for _, path := range []string{
		"/clusterscope/", "/clusterscope/clusters/dev", "/clusterscope/lab", "/clusterscope/endpoints",
		"/clusterscope/api/clusters", "/clusterscope/health",
		"/health", "/healthz", "/readyz", "/api/clusters", "/api/targets",
	} {
		if w := get(h, path); w.Code != http.StatusOK {
			t.Errorf("GET %s: got %d", path, w.Code)
		}
	}
	for _, path := range []string{"/", "/clusters/dev", "/lab"} {
		if w := get(h, path); w.Code != http.StatusNotFound {
			t.Errorf("GET %s outside the prefix: got %d, want 404", path, w.Code)
		}
	}
	if w := get(h, "/clusterscope"); w.Code/100 != 3 || w.Header().Get("Location") != "/clusterscope/" {
		t.Errorf("GET /clusterscope: got %d %q, want a redirect to /clusterscope/", w.Code, w.Header().Get("Location"))
	}

	for _, path := range []string{"/clusterscope/", "/clusterscope/clusters/dev", "/clusterscope/lab", "/clusterscope/endpoints"} {
		body := get(h, path).Body.String()
		if !strings.Contains(body, `<head><base href="/clusterscope/">`) {
			t.Errorf("%s: expected <base href> right after <head>", path)
		}
		if m := absoluteURL.FindAllStringSubmatch(strings.Replace(body, `<base href="/clusterscope/">`, "", 1), -1); len(m) > 0 {
			t.Errorf("%s: URLs bypass <base href>: %v", path, m)
		}
		if strings.Contains(body, "homerun2-tokens.css") {
			t.Errorf("%s: links the portal tokens without X-Forwarded-Prefix", path)
		}
	}
}

func TestPortalEmbedding(t *testing.T) {
	h := portalTestServer("/clusterscope").handler()

	shell := get(h, "/clusterscope/", "X-Forwarded-Prefix", "/clusterscope").Body.String()
	for _, want := range []string{
		`<link rel="stylesheet" href="/static/homerun2-tokens.css">`,
		`classList.add('hr-embedded')`,
		".hr-embedded header h1 { display: none; }",
		":where(:root)",
	} {
		if !strings.Contains(shell, want) {
			t.Errorf("shell through the portal: expected %q", want)
		}
	}
}
