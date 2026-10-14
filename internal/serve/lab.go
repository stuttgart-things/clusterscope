package serve

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/stuttgart-things/clusterscope/internal/graph"
	"github.com/stuttgart-things/clusterscope/internal/live"
	"github.com/stuttgart-things/clusterscope/internal/render"
	"github.com/stuttgart-things/clusterscope/internal/scan"
)

// Lab view (#54): Lab → Cluster → App → Endpoint, desired (Git) vs. GitOps
// status vs. actual state, with the mismatches as findings.

// Finding severities.
const (
	SevError   = "error"
	SevWarning = "warning"
	SevInfo    = "info"
)

// Finding is one mismatch between desired and actual state.
type Finding struct {
	Severity string `json:"severity"`
	Kind     string `json:"kind"`
	Cluster  string `json:"cluster"`
	Object   string `json:"object,omitempty"`
	Message  string `json:"message"`
}

// LabCluster is one cluster row of the lab view.
type LabCluster struct {
	Name string `json:"name"`
	// Managed is false for clusters that only have a kubeconfig.
	Managed      bool           `json:"managed"`
	Path         string         `json:"path,omitempty"` // relative to -root
	Tech         string         `json:"tech,omitempty"`
	Reachability string         `json:"reachability,omitempty"`
	Error        string         `json:"error,omitempty"`
	Apps         int            `json:"apps"`
	GitOps       map[string]int `json:"gitops,omitempty"`    // node status → count
	Workloads    map[string]int `json:"workloads,omitempty"` // workload status → count
	Drift        int            `json:"drift"`
	Routes       int            `json:"routes"`
	RoutesOK     int            `json:"routesOK"`
	NotDeployed  []string       `json:"notDeployed,omitempty"`
	NotInGit     []string       `json:"notInGit,omitempty"`
	// Nested counts live Kustomizations / Applications created by other
	// GitOps objects (in Git, but not in this cluster directory).
	Nested int `json:"nested,omitempty"`
	// NoGitOps marks a directory without any GitOps object and without a
	// reachable cluster (e.g. Crossplane claims next to the cluster dirs);
	// it is listed but not counted as a cluster.
	NoGitOps bool `json:"noGitOps,omitempty"`
	// NoGitOpsYet marks a reachable cluster whose directory has no GitOps
	// objects and which runs no Flux / Argo CD objects either (e.g. k3s set
	// up, Flux bootstrap still to come).
	NoGitOpsYet bool `json:"noGitOpsYet,omitempty"`
	Findings    int  `json:"findings"`
}

// LabSummary aggregates a lab.
type LabSummary struct {
	Clusters  int `json:"clusters"`
	Reachable int `json:"reachable"`
	Unmanaged int `json:"unmanaged"`
	Apps      int `json:"apps"`
	Healthy   int `json:"healthy"`
	Degraded  int `json:"degraded"`
	Down      int `json:"down"`
	Drift     int `json:"drift"`
	Routes    int `json:"routes"`
	RoutesOK  int `json:"routesOK"`
	Errors    int `json:"errors"`
	Warnings  int `json:"warnings"`
}

// Lab groups the clusters of one lab.
type Lab struct {
	Name     string       `json:"name"`
	Summary  LabSummary   `json:"summary"`
	Clusters []LabCluster `json:"clusters"`
	Findings []Finding    `json:"findings"`
}

// LabResponse is the JSON payload of GET /api/lab.
type LabResponse struct {
	GeneratedAt string `json:"generatedAt"`
	Live        bool   `json:"live"`
	Probing     bool   `json:"probing"`
	Labs        []Lab  `json:"labs"`
}

var severityRank = map[string]int{SevError: 0, SevWarning: 1, SevInfo: 2}

// labSnapshot builds the lab view from the cached scan and live results.
func (s *Server) labSnapshot() LabResponse {
	eps := s.endpointsSnapshot("")

	s.mu.RLock()
	defer s.mu.RUnlock()

	labs := make(map[string]*Lab)
	labOf := func(name string) *Lab {
		if labs[name] == nil {
			labs[name] = &Lab{Name: name, Clusters: []LabCluster{}, Findings: []Finding{}}
		}
		return labs[name]
	}

	routesByCluster := make(map[string][]RouteRow)
	for _, r := range eps.Routes {
		routesByCluster[r.Cluster] = append(routesByCluster[r.Cluster], r)
	}

	// Clusters with a directory (scanned or failed to scan).
	nameSet := make(map[string]bool)
	for n := range s.dirs {
		nameSet[n] = true
	}
	for n := range s.cache {
		nameSet[n] = true
	}
	for n := range s.scanErrs {
		nameSet[n] = true
	}
	names := make([]string, 0, len(nameSet))
	for n := range nameSet {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		dir, ok := s.dirs[name]
		if !ok {
			dir = scan.ClusterDir{Name: name, Lab: s.unmanagedLab()}
		}
		lab := labOf(dir.Lab)
		lc, findings := s.labCluster(name, dir, routesByCluster[name])
		lab.Clusters = append(lab.Clusters, lc)
		lab.Findings = append(lab.Findings, findings...)
	}

	for _, d := range s.duplicates {
		lab := labOf(d.Lab)
		lab.Findings = append(lab.Findings, Finding{
			Severity: SevWarning, Kind: "duplicate-name", Cluster: d.Name, Object: s.displayPath(d),
			Message: fmt.Sprintf("cluster name already used by %s — directory not scanned", s.displayPath(s.dirs[d.Name])),
		})
	}

	// Kubeconfig without cluster directory.
	unmanagedLab := s.unmanagedLab()
	unames := make([]string, 0, len(s.unmanaged))
	for n := range s.unmanaged {
		unames = append(unames, n)
	}
	sort.Strings(unames)
	for _, name := range unames {
		st := s.unmanaged[name]
		lab := labOf(unmanagedLab)
		lab.Clusters = append(lab.Clusters, LabCluster{
			Name: name, Reachability: st.Reachability, Error: st.Error, Findings: 1,
		})
		msg := "kubeconfig present, but no cluster directory in any repo"
		if st.Reachability != live.Reachable {
			msg += " — cluster " + st.Reachability
		}
		lab.Findings = append(lab.Findings, Finding{Severity: SevWarning, Kind: "unmanaged-cluster", Cluster: name, Message: msg})
	}

	out := LabResponse{
		GeneratedAt: time.Now().Format(time.RFC3339),
		Live:        s.opts.Live,
		Probing:     s.prober != nil,
		Labs:        []Lab{},
	}
	for _, lab := range labs {
		sort.SliceStable(lab.Findings, func(i, j int) bool {
			a, b := lab.Findings[i], lab.Findings[j]
			if severityRank[a.Severity] != severityRank[b.Severity] {
				return severityRank[a.Severity] < severityRank[b.Severity]
			}
			if a.Cluster != b.Cluster {
				return a.Cluster < b.Cluster
			}
			return a.Object < b.Object
		})
		summarize(lab)
		out.Labs = append(out.Labs, *lab)
	}
	sort.Slice(out.Labs, func(i, j int) bool { return out.Labs[i].Name < out.Labs[j].Name })
	return out
}

// labCluster builds the row and findings of a cluster with a directory.
// The caller holds s.mu.
func (s *Server) labCluster(name string, dir scan.ClusterDir, routes []RouteRow) (LabCluster, []Finding) {
	lc := LabCluster{Name: name, Managed: true, Path: s.displayPath(dir)}
	var fs []Finding
	add := func(sev, kind, object, msg string) {
		fs = append(fs, Finding{Severity: sev, Kind: kind, Cluster: name, Object: object, Message: msg})
	}

	if msg, ok := s.scanErrs[name]; ok {
		add(SevError, "scan-error", lc.Path, msg)
	}
	p := s.cache[name]
	if p != nil {
		lc.Tech = p.Technology
	}

	st, hasLive := s.liveStatus[name]
	if p != nil && len(p.Graph.Nodes) == 0 && st.Reachability != live.Reachable {
		lc.NoGitOps = true
		lc.Reachability = st.Reachability
		return lc, nil
	}
	if hasLive {
		lc.Reachability, lc.Error = st.Reachability, st.Error
		lc.Nested = len(st.Nested)
		switch st.Reachability {
		case live.Unreachable:
			add(SevError, "unreachable", "", "API server unreachable: "+st.Error)
		case live.Unconfigured:
			add(SevWarning, "no-kubeconfig", "", "cluster directory in Git, but no kubeconfig — actual state unknown")
		}
		if st.WorkloadError != "" {
			add(SevWarning, "workloads-unreadable", "", st.WorkloadError)
		}
		lc.NotDeployed, lc.NotInGit = st.NotDeployed, st.NotInGit
		for _, n := range st.NotDeployed {
			add(SevWarning, "not-deployed", n, "in Git, but not in the cluster")
		}
		for _, n := range st.NotInGit {
			add(SevInfo, "not-in-git", n, "in the cluster, but not in the cluster directory")
		}
		if p != nil && len(p.Graph.Nodes) == 0 && st.Reachability == live.Reachable && len(st.NotInGit)+len(st.Nested) == 0 {
			lc.NoGitOpsYet = true
			add(SevInfo, "no-gitops", "", "cluster runs, but no GitOps yet: no Flux / Argo CD objects in the directory or in the cluster")
		}
	}

	if p != nil {
		lc.GitOps = map[string]int{}
		lc.Workloads = map[string]int{}
		for _, n := range p.Graph.Nodes {
			if n.Type == "apps" || n.Type == "argoapp" {
				lc.Apps++
			}
			if n.Status != "" {
				lc.GitOps[n.Status]++
			}
			if n.Status == "failed" {
				add(SevError, "gitops-failed", n.ID, n.Message)
			}
			w := n.Workloads
			if w == nil {
				continue
			}
			lc.Workloads[w.Status]++
			switch {
			case w.Drift:
				lc.Drift++
				add(SevError, "drift", n.ID, fmt.Sprintf("GitOps ready, workloads %s %d/%d%s", w.Status, w.Ready, w.Total, firstIssue(w)))
			case n.Status != "failed" && w.Status == graph.WorkloadDown:
				add(SevError, "workloads-down", n.ID, fmt.Sprintf("workloads down %d/%d%s", w.Ready, w.Total, firstIssue(w)))
			case n.Status != "failed" && w.Status == graph.WorkloadDegraded:
				add(SevWarning, "workloads-degraded", n.ID, fmt.Sprintf("workloads degraded %d/%d%s", w.Ready, w.Total, firstIssue(w)))
			}
		}
		if len(lc.GitOps) == 0 {
			lc.GitOps = nil
		}
		if len(lc.Workloads) == 0 {
			lc.Workloads = nil
		}
	}

	for _, r := range routes {
		lc.Routes++
		obj := r.Namespace + "/" + r.Name
		ok := r.Accepted == "True" && r.ResolvedRefs == "True"
		if !ok {
			add(SevWarning, "route-not-accepted", obj, fmt.Sprintf("HTTPRoute Accepted=%s ResolvedRefs=%s", r.Accepted, r.ResolvedRefs))
		}
		for _, pr := range r.Probes {
			if !pr.OK {
				ok = false
				add(SevError, "endpoint-down", obj, fmt.Sprintf("%s: %s failed — %s", pr.URL, pr.Stage, pr.Error))
			}
		}
		if ok {
			lc.RoutesOK++
		}
	}

	lc.Findings = len(fs)
	return lc, fs
}

func firstIssue(w *graph.WorkloadHealth) string {
	for _, it := range w.Workloads {
		if len(it.Issues) > 0 {
			return " — " + it.Issues[0]
		}
	}
	for _, hr := range w.HelmReleases {
		if hr.Status != "ready" {
			return " — HelmRelease " + hr.Name + " " + hr.Status
		}
	}
	return ""
}

func summarize(lab *Lab) {
	sm := LabSummary{}
	for _, c := range lab.Clusters {
		if c.NoGitOps {
			continue
		}
		sm.Clusters++
		if !c.Managed {
			sm.Unmanaged++
		}
		if c.Reachability == live.Reachable {
			sm.Reachable++
		}
		sm.Apps += c.Apps
		sm.Healthy += c.Workloads[graph.WorkloadHealthy]
		sm.Degraded += c.Workloads[graph.WorkloadDegraded]
		sm.Down += c.Workloads[graph.WorkloadDown]
		sm.Drift += c.Drift
		sm.Routes += c.Routes
		sm.RoutesOK += c.RoutesOK
	}
	for _, f := range lab.Findings {
		switch f.Severity {
		case SevError:
			sm.Errors++
		case SevWarning:
			sm.Warnings++
		}
	}
	lab.Summary = sm
}

// unmanagedLab is the lab for clusters without a directory: -lab, else the
// lab all directories share, else the default lab. The caller holds s.mu.
func (s *Server) unmanagedLab() string {
	if s.opts.Lab != "" {
		return s.opts.Lab
	}
	lab := ""
	for _, d := range s.dirs {
		if lab != "" && d.Lab != lab {
			return scan.DefaultLab
		}
		lab = d.Lab
	}
	if lab == "" {
		return scan.DefaultLab
	}
	return lab
}

// displayPath is the cluster directory as shown: [<root name>/]<relative>.
func (s *Server) displayPath(d scan.ClusterDir) string {
	if d.Rel != "" {
		return d.Rel
	}
	for _, r := range s.cfg.Roots {
		if rel, err := filepath.Rel(r.Path, d.Path); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	return d.Path
}

// updateLabMetrics refreshes the lab-level series.
func (s *Server) updateLabMetrics() {
	snap := s.labSnapshot()
	labFindings.Reset()
	clusterInfo.Reset()
	for _, lab := range snap.Labs {
		counts := map[[2]string]int{}
		for _, f := range lab.Findings {
			counts[[2]string{f.Severity, f.Kind}]++
		}
		for k, n := range counts {
			labFindings.WithLabelValues(lab.Name, k[0], k[1]).Set(float64(n))
		}
		for _, c := range lab.Clusters {
			clusterInfo.WithLabelValues(c.Name, lab.Name, c.Tech, fmt.Sprint(c.Managed)).Set(1)
		}
	}
}

// handleAPIlab serves GET /api/lab.
func (s *Server) handleAPIlab(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.labSnapshot())
}

// handleLab serves the lab dashboard page.
func (s *Server) handleLab(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := render.WriteLab(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
