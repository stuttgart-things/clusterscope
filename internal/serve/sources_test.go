package serve

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stuttgart-things/clusterscope/internal/clusterbook"
	"github.com/stuttgart-things/clusterscope/internal/live"
	"github.com/stuttgart-things/clusterscope/internal/scan"
)

func TestLabClusterbook(t *testing.T) {
	s := labTestServer()
	// platform's routes live under *.platform-dom.lab
	ep := s.endpoints["platform"]
	ep.ep.Routes[0].Hostnames = []string{"vault.platform-dom.lab"}
	s.endpoints["platform"] = ep
	s.books = []*bookState{{
		spec:    scan.ClusterbookSpec{URL: "https://book.lab", Lab: "labul", Names: map[string]string{"edgy": "edge"}},
		fetched: time.Now(),
		records: []clusterbook.Record{
			{Cluster: "plat", FQDN: "*.platform-dom.lab", IPs: []clusterbook.IP{{IP: "10.0.2.6"}}}, // by domain
			{Cluster: "edgy", IPs: []clusterbook.IP{{IP: "10.0.2.7"}}},                            // by mapping
			{Cluster: "edge-test", IPs: []clusterbook.IP{{IP: "10.0.2.9"}}},                       // by name (kubeconfig only)
			{Cluster: "smoke", FQDN: "*.smoke.lab", IPs: []clusterbook.IP{{IP: "10.0.1.10"}}},     // orphan
		},
	}}

	snap := s.labSnapshot()
	var lab *Lab
	for i := range snap.Labs {
		if snap.Labs[i].Name == "labul" {
			lab = &snap.Labs[i]
		}
	}
	if lab == nil {
		t.Fatal("no labul lab")
	}
	byName := map[string]LabCluster{}
	for _, c := range lab.Clusters {
		byName[c.Name] = c
	}
	if c := byName["platform"]; len(c.Addresses) != 1 || c.Addresses[0] != "10.0.2.6" || c.FQDN != "*.platform-dom.lab" || c.ClusterbookName != "plat" {
		t.Errorf("platform = %+v", c)
	}
	if c := byName["edge"]; len(c.Addresses) != 1 || c.ClusterbookName != "edgy" {
		t.Errorf("edge = %+v", c)
	}
	if c := byName["edge-test"]; len(c.Addresses) != 1 || c.ClusterbookName != "" {
		t.Errorf("edge-test = %+v", c)
	}

	kinds := map[string][]string{}
	for _, f := range lab.Findings {
		kinds[f.Kind] = append(kinds[f.Kind], f.Cluster)
	}
	if got := kinds["orphan-allocation"]; len(got) != 1 || got[0] != "smoke" {
		t.Errorf("orphan-allocation = %v", got)
	}
	if got := kinds["no-allocation"]; len(got) != 1 || got[0] != "infra" {
		t.Errorf("no-allocation = %v", got)
	}
	if len(snap.Sources) != 1 || !snap.Sources[0].OK || snap.Sources[0].Records != 4 {
		t.Errorf("sources = %+v", snap.Sources)
	}
	// the labda lab has no clusterbook: no allocation findings there
	for _, l := range snap.Labs {
		if l.Name != "labda" {
			continue
		}
		for _, f := range l.Findings {
			if f.Kind == "no-allocation" {
				t.Errorf("labda finding %+v", f)
			}
		}
	}
}

func TestLabClusterbookAmbiguousAndDown(t *testing.T) {
	s := labTestServer()
	s.liveStatus["infra"] = live.ClusterStatus{Reachability: live.Reachable}
	s.endpoints["infra"] = endpointState{ep: &live.Endpoints{Routes: []live.Route{{Name: "x", Hostnames: []string{"a.shared.lab"}}}}, collectedAt: time.Now()}
	ep := s.endpoints["platform"]
	ep.ep.Routes[0].Hostnames = []string{"b.shared.lab"}
	s.endpoints["platform"] = ep
	s.books = []*bookState{{
		spec:    scan.ClusterbookSpec{URL: "https://book.lab/", Lab: "labul"},
		fetched: time.Now(),
		err:     "GET /api/v1/clusters: 502 Bad Gateway",
		records: []clusterbook.Record{{Cluster: "shared", FQDN: "*.shared.lab"}},
	}}
	snap := s.labSnapshot()
	kinds := map[string]int{}
	for _, l := range snap.Labs {
		for _, f := range l.Findings {
			kinds[f.Kind]++
		}
	}
	if kinds["orphan-allocation"] != 1 {
		t.Errorf("ambiguous domain must not match: %v", kinds)
	}
	if kinds["source-unavailable"] != 1 {
		t.Errorf("source-unavailable missing: %v", kinds)
	}
	if snap.Sources[0].OK {
		t.Error("source with error reported OK")
	}
}

func TestRefreshSources(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/clusters":
			_, _ = w.Write([]byte(`[{"cluster":"platform"}]`))
		default:
			_, _ = w.Write([]byte(`{"cluster":"platform","ips":[{"ip":"10.0.2.6"}]}`))
		}
	}))
	defer srv.Close()
	s := labTestServer()
	s.books = []*bookState{{spec: scan.ClusterbookSpec{URL: srv.URL, Lab: "labul"}, client: clusterbook.New(srv.URL, time.Second)}}
	s.refreshSources()
	if b := s.books[0]; b.err != "" || len(b.records) != 1 {
		t.Fatalf("book = %+v", b)
	}
	srv.Close()
	s.refreshSources()
	if b := s.books[0]; b.err == "" || len(b.records) != 1 {
		t.Fatalf("after outage: err=%q records=%d (want error, last records kept)", b.err, len(b.records))
	}
}
