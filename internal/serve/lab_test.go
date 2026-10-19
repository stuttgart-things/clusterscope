package serve

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stuttgart-things/clusterscope/internal/graph"
	"github.com/stuttgart-things/clusterscope/internal/live"
	"github.com/stuttgart-things/clusterscope/internal/probe"
	"github.com/stuttgart-things/clusterscope/internal/scan"
)

func labTestServer() *Server {
	profiles := map[string]*graph.ClusterProfile{
		"platform": {Technology: "flux", Graph: graph.Graph{Nodes: []graph.Node{
			{ID: "flux-system", Type: "source", Status: "ready"},
			{ID: "nfs-csi", Type: "infra", Status: "ready", Workloads: &graph.WorkloadHealth{
				Status: graph.WorkloadDegraded, Ready: 2, Total: 3, Drift: true,
				Workloads: []graph.Workload{{Name: "snap", Issues: []string{"snap-1: CrashLoopBackOff"}}},
			}},
			{ID: "web", Type: "apps", Status: "ready", Workloads: &graph.WorkloadHealth{Status: graph.WorkloadHealthy, Ready: 1, Total: 1}},
			{ID: "broken", Type: "apps", Status: "failed", Message: "kustomize build failed"},
		}}},
		"infra": {Technology: "flux", Graph: graph.Graph{Nodes: []graph.Node{{ID: "apps", Type: "apps", Status: "unknown"}}}},
		"dev": {Technology: "argocd", Graph: graph.Graph{Nodes: []graph.Node{
			{ID: "shop", Type: "argoapp", Technology: "argocd", Status: "unknown"},
			{ID: "orders", Type: "apps", Path: "./clusters/labda/vsphere/xrs", Status: "unknown"},
		}}},
		"xrs":  {Technology: "flux"}, // claims next to the cluster dirs, no GitOps objects
		"edge": {Technology: "flux"}, // k3s set up, no Flux yet
	}
	s := newTestServer(profiles, map[string]string{}, true)
	s.cfg = scan.Config{Roots: []scan.RootSpec{{Path: "/repo/clusters"}}}
	s.dirs = map[string]scan.ClusterDir{
		"platform": {Name: "platform", Path: "/repo/clusters/labul/vsphere/platform", Lab: "labul"},
		"infra":    {Name: "infra", Path: "/repo/clusters/labul/vsphere/infra", Lab: "labul"},
		"dev":      {Name: "dev", Path: "/repo/clusters/labda/vsphere/dev", Lab: "labda"},
		"xrs":      {Name: "xrs", Path: "/repo/clusters/labda/vsphere/xrs", Lab: "labda"},
		"edge":     {Name: "edge", Dir: "edge-test2", Path: "/harvester/clusters/edge-test2", Rel: "harvester/edge-test2", Lab: "labul"},
	}
	s.duplicates = []scan.ClusterDir{{Name: "dev", Path: "/repo/clusters/labul/vsphere/dev", Lab: "labul", Duplicate: true}}
	s.liveStatus = map[string]live.ClusterStatus{
		"platform": {Reachability: live.Reachable, NotDeployed: []string{"monitoring"}, NotInGit: []string{"flux-system/hand-applied"}},
		"infra":    {Reachability: live.Unreachable, Error: "i/o timeout"},
		"dev":      {Reachability: live.Unconfigured, Error: "no kubeconfig for cluster"},
		"xrs":      {Reachability: live.Unconfigured, Error: "no kubeconfig for cluster"},
		"edge":     {Reachability: live.Reachable},
	}
	s.endpoints = map[string]endpointState{"platform": {ep: &live.Endpoints{GatewayAPI: true, Routes: []live.Route{
		{Namespace: "vault", Name: "vault", URLs: []string{"https://vault.lab"}, Accepted: "True", ResolvedRefs: "True"},
		{Namespace: "web", Name: "web", URLs: []string{"https://web.lab"}, Accepted: "True", ResolvedRefs: "True"},
	}}, collectedAt: time.Now()}}
	s.probes = map[string]map[string]probe.Result{"platform": {
		"https://vault.lab": {URL: "https://vault.lab", OK: true},
		"https://web.lab":   {URL: "https://web.lab", Stage: probe.StageTLS, Error: "x509: expired"},
	}}
	s.unmanaged = map[string]live.ClusterStatus{"edge-test": {Reachability: live.Reachable}}
	s.opts.Lab = "labul" // lab for clusters without a directory
	return s
}

func TestLabSnapshot(t *testing.T) {
	s := labTestServer()
	snap := s.labSnapshot()
	if len(snap.Labs) != 2 || snap.Labs[0].Name != "labda" || snap.Labs[1].Name != "labul" {
		t.Fatalf("labs: %+v", snap.Labs)
	}
	labul := snap.Labs[1]

	kinds := map[string]Finding{}
	for _, f := range labul.Findings {
		kinds[f.Kind+"|"+f.Cluster+"|"+f.Object] = f
	}
	for _, want := range []string{
		"drift|platform|nfs-csi",
		"gitops-failed|platform|broken",
		"not-deployed|platform|monitoring",
		"not-in-git|platform|flux-system/hand-applied",
		"endpoint-down|platform|web/web",
		"unreachable|infra|",
		"unmanaged-cluster|edge-test|",
		"duplicate-name|dev|labul/vsphere/dev",
		"no-gitops|edge|",
	} {
		if _, ok := kinds[want]; !ok {
			t.Errorf("missing finding %s in %+v", want, labul.Findings)
		}
	}
	if f := kinds["drift|platform|nfs-csi"]; f.Severity != SevError || f.Message != "GitOps ready, workloads degraded 2/3 — snap-1: CrashLoopBackOff" {
		t.Errorf("drift finding: %+v", f)
	}
	// errors first
	if labul.Findings[0].Severity != SevError || labul.Findings[len(labul.Findings)-1].Severity != SevInfo {
		t.Errorf("finding order: %+v", labul.Findings)
	}

	sm := labul.Summary
	if sm.Clusters != 4 || sm.Unmanaged != 1 || sm.Reachable != 3 || sm.Drift != 1 || sm.Routes != 2 || sm.RoutesOK != 1 || sm.Healthy != 1 || sm.Degraded != 1 {
		t.Errorf("summary: %+v", sm)
	}
	var platform, edge LabCluster
	for _, c := range labul.Clusters {
		switch c.Name {
		case "platform":
			platform = c
		case "edge":
			edge = c
		}
	}
	if !edge.NoGitOpsYet || edge.NoGitOps || edge.Path != "harvester/edge-test2" {
		t.Errorf("edge row: %+v", edge)
	}
	if !platform.Managed || platform.Path != "labul/vsphere/platform" || platform.Apps != 2 || platform.GitOps["failed"] != 1 {
		t.Errorf("platform row: %+v", platform)
	}

	labda := snap.Labs[0]
	if len(labda.Findings) != 1 || labda.Findings[0].Kind != "no-kubeconfig" || labda.Findings[0].Cluster != "dev" {
		t.Errorf("labda findings: %+v", labda.Findings)
	}
	if labda.Summary.Clusters != 1 || len(labda.Clusters) != 2 || !labda.Clusters[1].NoGitOps {
		t.Errorf("directory without GitOps objects must be listed but not counted: %+v", labda)
	}
	if x := labda.Clusters[1]; x.IncludedBy != "dev" || x.IncludedVia != "orders" {
		t.Errorf("xrs is dev's content (#93): %+v", x)
	}
	if d := labda.Clusters[0]; len(d.Includes) != 1 || d.Includes[0] != "xrs" {
		t.Errorf("dev includes xrs: %+v", d)
	}
}

func TestLabMetricsAndAPI(t *testing.T) {
	s := labTestServer()
	s.updateLabMetrics()
	if got := testutil.ToFloat64(labFindings.WithLabelValues("labul", SevError, "drift")); got != 1 {
		t.Errorf("drift findings metric = %v", got)
	}
	if got := testutil.ToFloat64(clusterInfo.WithLabelValues("edge-test", "labul", "", "false")); got != 1 {
		t.Errorf("cluster info for unmanaged = %v", got)
	}

	rec := httptest.NewRecorder()
	s.handleAPIlab(rec, httptest.NewRequest("GET", "/api/lab", nil))
	var resp LabResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || len(resp.Labs) != 2 || !resp.Live {
		t.Errorf("api: err=%v resp=%+v", err, resp)
	}
	rec = httptest.NewRecorder()
	s.handleLab(rec, httptest.NewRequest("GET", "/lab", nil))
	if rec.Code != 200 || rec.Body.Len() == 0 {
		t.Errorf("lab page: %d", rec.Code)
	}
}

func TestLabUnmanagedWithDirectory(t *testing.T) {
	s := labTestServer()
	// last live refresh ran before the directory of "platform" was scanned
	s.unmanaged["platform"] = live.ClusterStatus{Reachability: live.Reachable}
	n := 0
	for _, l := range s.labSnapshot().Labs {
		for _, c := range l.Clusters {
			if c.Name == "platform" {
				n++
				if !c.Managed {
					t.Error("platform listed as kubeconfig only")
				}
			}
		}
	}
	if n != 1 {
		t.Errorf("platform listed %d times", n)
	}
}

func TestUnmanagedLab(t *testing.T) {
	s := labTestServer()
	s.opts.Lab = ""
	if got := s.unmanagedLab(); got != scan.DefaultLab { // labul + labda
		t.Errorf("mixed labs: %q", got)
	}
	delete(s.dirs, "dev")
	delete(s.dirs, "xrs")
	delete(s.dirs, "edge")
	if got := s.unmanagedLab(); got != "labul" {
		t.Errorf("single lab: %q", got)
	}
	s.opts.Lab = "explicit"
	if got := s.unmanagedLab(); got != "explicit" {
		t.Errorf("-lab: %q", got)
	}
}

func TestScanAll_NestedAndPrune(t *testing.T) {
	root := filepath.Join(t.TempDir(), "clusters")
	write := func(rel string) {
		dir := filepath.Join(root, rel)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		ks := "apiVersion: kustomize.toolkit.fluxcd.io/v1\nkind: Kustomization\nmetadata:\n  name: apps\n  namespace: flux-system\nspec:\n  path: ./apps\n"
		if err := os.WriteFile(filepath.Join(dir, "apps.yaml"), []byte(ks), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("labul/vsphere/platform")
	write("labda/vsphere/dev")

	s := newTestServer(map[string]*graph.ClusterProfile{}, map[string]string{}, false)
	s.cfg = scan.Config{Roots: []scan.RootSpec{{Path: root}}}
	s.dirs = map[string]scan.ClusterDir{}
	s.scanAll()
	if len(s.cache) != 2 || s.dirs["platform"].Lab != "labul" || s.dirs["dev"].Lab != "labda" {
		t.Fatalf("scan: cache=%d dirs=%+v", len(s.cache), s.dirs)
	}
	watch := s.watchDirs()
	if len(watch) != 7 { // root, 2×lab, 2×vsphere, 2×cluster
		t.Errorf("watch dirs: %v", watch)
	}

	if err := os.RemoveAll(filepath.Join(root, "labda")); err != nil {
		t.Fatal(err)
	}
	s.scanAll()
	if _, ok := s.cache["dev"]; ok || len(s.cache) != 1 {
		t.Errorf("removed cluster still cached: %v", s.cache)
	}
}

func TestScanAll_MultiRootLateRoot(t *testing.T) {
	base := t.TempDir()
	write := func(dir string) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		ks := "apiVersion: kustomize.toolkit.fluxcd.io/v1\nkind: Kustomization\nmetadata:\n  name: apps\n  namespace: flux-system\nspec:\n  path: ./apps\n"
		if err := os.WriteFile(filepath.Join(dir, "apps.yaml"), []byte(ks), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	st := filepath.Join(base, "st", "clusters", "labul")
	hv := filepath.Join(base, "harvester", "clusters")
	write(filepath.Join(st, "vsphere", "platform-sthings"))

	s := newTestServer(map[string]*graph.ClusterProfile{}, map[string]string{}, false)
	s.dirs = map[string]scan.ClusterDir{}
	s.cfg = scan.Config{
		Roots:    []scan.RootSpec{{Path: st}, {Path: hv, Name: "harvester", Include: []string{"edge-test2"}}},
		Clusters: map[string]scan.Override{"edge-test2": {Name: "edge-tt-test2", Lab: "labul"}},
	}
	s.scanAll()
	before := s.rootsPresent()
	if len(s.cache) != 1 {
		t.Fatalf("first scan: %v", s.cache)
	}

	// git-sync clones the second repo later
	write(filepath.Join(hv, "edge-test2"))
	write(filepath.Join(hv, "homerun2-dev")) // not included
	if s.rootsPresent() == before {
		t.Fatal("appearing root not detected")
	}
	s.scanAll()
	d, ok := s.dirs["edge-tt-test2"]
	if len(s.cache) != 2 || !ok || d.Lab != "labul" || d.Rel != "harvester/edge-test2" {
		t.Errorf("after clone: cache=%d dirs=%+v", len(s.cache), s.dirs)
	}
}
