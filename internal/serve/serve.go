// Package serve implements the HTTP dashboard server for clusterscope.
//
// It walks a root directory, discovers cluster subdirectories (auto-detecting
// technology by YAML content), parses each one, and serves an HTMX-driven
// dashboard where each cluster's D3.js visualization loads in a separate panel.
package serve

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stuttgart-things/clusterscope/internal/diff"
	"github.com/stuttgart-things/clusterscope/internal/graph"
	"github.com/stuttgart-things/clusterscope/internal/live"
	"github.com/stuttgart-things/clusterscope/internal/preview"
	"github.com/stuttgart-things/clusterscope/internal/probe"
	"github.com/stuttgart-things/clusterscope/internal/render"
	"github.com/stuttgart-things/clusterscope/internal/scan"
	"github.com/stuttgart-things/clusterscope/pkg/argocd"
	"github.com/stuttgart-things/clusterscope/pkg/flux"
)

// Options configures optional features of the HTTP server.
type Options struct {
	// Live enriches every cluster profile with real-time reconciliation status
	// fetched from the Kubernetes API.
	Live bool
	// Kubeconfig is the path to the default kubeconfig file. Empty means
	// in-cluster ServiceAccount, then KUBECONFIG / ~/.kube/config.
	Kubeconfig string
	// KubeconfigDir holds one kubeconfig file per cluster, named after the
	// cluster directory. Enables per-cluster live mode.
	KubeconfigDir string
	// KubeconfigMap maps cluster name → kubeconfig path (relative to
	// KubeconfigDir). Enables per-cluster live mode.
	KubeconfigMap map[string]string
	// SelfCluster names the cluster clusterscope runs in. In per-cluster
	// mode only this cluster falls back to the default kubeconfig.
	SelfCluster string
	// Probe enables a one-shot HTTP(S) check of every HTTPRoute URL on each
	// live refresh (requires Live).
	Probe bool
	// ProbeTimeout is the per-URL probe timeout. Defaults to 5s.
	ProbeTimeout time.Duration
	// RefreshSeconds is how often (in seconds) live status is re-fetched.
	// Defaults to 30 when Live is true.
	RefreshSeconds int
	// PreviewRoot is the directory containing the PR head ref cluster files.
	// When set, GET /preview/<cluster> and GET /api/preview/<cluster> become
	// available, showing a diff between root (base) and preview-root (head).
	PreviewRoot string
}

// ClusterSummary is the JSON payload returned by GET /api/clusters.
type ClusterSummary struct {
	Name          string         `json:"name"`
	Tech          string         `json:"tech"`
	NodeCount     int            `json:"nodeCount"`
	AppCount      int            `json:"appCount"`
	LastScan      string         `json:"lastScan"`
	LastRefreshed string         `json:"lastRefreshed,omitempty"`
	ScanError     string         `json:"scanError,omitempty"`
	HealthSummary map[string]int `json:"healthSummary,omitempty"`
	// Reachability is "reachable", "unreachable" or "unconfigured" (live mode).
	Reachability string `json:"reachability,omitempty"`
	LiveError    string `json:"liveError,omitempty"`
}

// Server holds the HTTP mux and cluster cache.
type Server struct {
	root      string
	opts      Options
	mu        sync.RWMutex
	cache     map[string]*graph.ClusterProfile
	scanned   map[string]time.Time
	refreshed map[string]time.Time
	scanErrs  map[string]string
	ready     bool

	// clients resolves the kubeconfig per cluster; liveStatus holds the last
	// cluster-level live result (reachability).
	clients    *live.Clients
	liveStatus map[string]live.ClusterStatus

	// endpoints holds the last Gateway API inventory per cluster; it is kept
	// (marked stale) while a cluster is unreachable. probes holds the last
	// probe result per cluster and URL.
	endpoints map[string]endpointState
	prober    *probe.Prober
	probes    map[string]map[string]probe.Result

	// preview holds parsed cluster profiles from the PR head directory.
	previewMu    sync.RWMutex
	previewCache map[string]*graph.ClusterProfile

	// cloner handles on-demand git clone for PR preview via query params.
	cloner *preview.Cloner
}

// Start starts the HTTP server on addr, scanning root for cluster directories.
func Start(addr, root string, opts Options) error {
	if _, err := os.Stat(root); err != nil {
		return err
	}

	if opts.Live && opts.RefreshSeconds <= 0 {
		opts.RefreshSeconds = 30
	}

	s := &Server{
		root:         root,
		opts:         opts,
		cache:        make(map[string]*graph.ClusterProfile),
		scanned:      make(map[string]time.Time),
		refreshed:    make(map[string]time.Time),
		scanErrs:     make(map[string]string),
		liveStatus:   make(map[string]live.ClusterStatus),
		endpoints:    make(map[string]endpointState),
		probes:       make(map[string]map[string]probe.Result),
		previewCache: make(map[string]*graph.ClusterProfile),
		cloner:       preview.NewCloner(preview.DefaultTTL),
	}

	if opts.Live {
		s.clients = live.NewClients(live.ClientConfig{
			KubeconfigDir: opts.KubeconfigDir,
			Map:           opts.KubeconfigMap,
			Kubeconfig:    opts.Kubeconfig,
			Self:          opts.SelfCluster,
		})
		if opts.Probe {
			if opts.ProbeTimeout <= 0 {
				opts.ProbeTimeout = 5 * time.Second
				s.opts.ProbeTimeout = opts.ProbeTimeout
			}
			s.prober = probe.New(opts.ProbeTimeout)
		}
	}

	s.scanAll()
	if opts.PreviewRoot != "" {
		s.scanPreview()
		go s.watchPreview()
	}
	s.mu.Lock()
	s.ready = true
	s.mu.Unlock()
	go s.watch()

	if opts.Live {
		log.Printf("live mode enabled  kubeconfig=%q  kubeconfig-dir=%q  mapped=%d  self=%q  refresh=%ds",
			opts.Kubeconfig, opts.KubeconfigDir, len(opts.KubeconfigMap), opts.SelfCluster, opts.RefreshSeconds)
		s.enrichAll()
		go s.liveRefreshLoop()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/clusters", s.handleAPIclusters)
	mux.HandleFunc("/api/endpoints", s.handleAPIendpoints)
	mux.HandleFunc("/api/targets", s.handleAPItargets)
	mux.HandleFunc("/endpoints", s.handleEndpoints)
	mux.HandleFunc("/clusters/", s.handleCluster)
	mux.HandleFunc("/pdf/", s.handlePDF)
	mux.HandleFunc("/api/preview/", s.handleAPIpreview)
	mux.HandleFunc("/preview/", s.handlePreview)

	log.Printf("clusterscope serving on %s  root=%s", addr, root)
	return http.ListenAndServe(addr, mux) //nolint:gosec
}

// ── scanning ──────────────────────────────────────────────────────────────────

// ── live enrichment ────────────────────────────────────────────────────────────

func (s *Server) enrichAll() {
	start := time.Now()
	defer func() { liveEnrichDuration.Observe(time.Since(start).Seconds()) }()

	s.mu.RLock()
	names := make([]string, 0, len(s.cache))
	for n := range s.cache {
		names = append(names, n)
	}
	s.mu.RUnlock()

	// Clusters are enriched in parallel so one unreachable API server
	// (timeout) does not delay the others.
	var wg sync.WaitGroup
	for _, name := range names {
		s.mu.RLock()
		p := s.cache[name]
		s.mu.RUnlock()
		if p == nil {
			continue
		}
		wg.Add(1)
		go func(name string, p *graph.ClusterProfile) {
			defer wg.Done()
			s.enrichCluster(name, p)
		}(name, p)
	}
	wg.Wait()
}

// enrichCluster runs live enrichment for one cluster and records its
// reachability.
func (s *Server) enrichCluster(name string, p *graph.ClusterProfile) {
	st := s.clients.Enrich(p, name)
	s.mu.Lock()
	s.refreshed[name] = st.CheckedAt
	s.liveStatus[name] = st
	s.mu.Unlock()

	switch st.Reachability {
	case live.Reachable:
		clusterReachable.WithLabelValues(name).Set(1)
	case live.Unreachable:
		clusterReachable.WithLabelValues(name).Set(0)
	default:
		clusterReachable.DeleteLabelValues(name)
	}
	s.updateEndpoints(name, st)
	log.Printf("live enriched %s  reachability=%s", name, st.Reachability)
}

func (s *Server) liveRefreshLoop() {
	ticker := time.NewTicker(time.Duration(s.opts.RefreshSeconds) * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.enrichAll()
	}
}

func (s *Server) scanAll() {
	start := time.Now()
	defer func() { scanDuration.Observe(time.Since(start).Seconds()) }()

	// Check if any immediate subdirectory contains YAML — if yes, multi-cluster mode.
	// Only fall back to treating root as a single cluster if no subdir clusters exist.
	hasSubClusters := false
	if dirEntries, err := os.ReadDir(s.root); err == nil {
		for _, e := range dirEntries {
			if e.IsDir() && scan.HasYAML(filepath.Join(s.root, e.Name())) {
				hasSubClusters = true
				break
			}
		}
	}

	if !hasSubClusters && scan.HasYAML(s.root) {
		s.scanCluster(s.root, filepath.Base(s.root))
		return
	}

	entries, err := os.ReadDir(s.root)
	if err != nil {
		log.Printf("scanAll: %v", err)
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			s.scanCluster(filepath.Join(s.root, e.Name()), e.Name())
		}
	}
}

func (s *Server) scanCluster(path, name string) {
	tech := scan.DetectTech(path)
	var profile *graph.ClusterProfile
	var err error

	switch tech {
	case "argocd":
		profile, err = argocd.ParseDir(path)
	default:
		tech = "flux"
		profile, err = flux.ParseDir(path)
	}
	if err != nil {
		log.Printf("scan %s (%s): %v", name, tech, err)
		scanErrors.Inc()
		s.mu.Lock()
		s.scanErrs[name] = err.Error()
		s.scanned[name] = time.Now()
		s.mu.Unlock()
		return
	}

	s.mu.Lock()
	s.cache[name] = profile
	delete(s.scanErrs, name)
	s.scanned[name] = time.Now()
	s.mu.Unlock()
	log.Printf("scanned %s  tech=%s  nodes=%d", name, tech, len(profile.Graph.Nodes))

	// Update per-cluster metrics
	var appCount int
	for _, n := range profile.Graph.Nodes {
		if n.Type == "apps" || n.Type == "argoapp" {
			appCount++
		}
	}
	clusterNodesTotal.WithLabelValues(name, tech).Set(float64(len(profile.Graph.Nodes)))
	clusterAppsTotal.WithLabelValues(name, tech).Set(float64(appCount))
	s.mu.RLock()
	clustersTotal.Set(float64(len(s.cache)))
	s.mu.RUnlock()

	if s.opts.Live {
		s.enrichCluster(name, profile)
	}
}

// ── file watcher ──────────────────────────────────────────────────────────────

func (s *Server) watch() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("fsnotify: %v", err)
		return
	}
	defer func() { _ = watcher.Close() }()

	if err := watcher.Add(s.root); err != nil {
		log.Printf("fsnotify add: %v", err)
		return
	}

	debounce := make(map[string]*time.Timer)
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			dir := filepath.Dir(event.Name)
			if dir == s.root {
				dir = event.Name
			}
			name := filepath.Base(dir)
			if t, exists := debounce[name]; exists {
				t.Stop()
			}
			debounce[name] = time.AfterFunc(500*time.Millisecond, func() {
				s.scanCluster(dir, name)
			})
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("fsnotify error: %v", err)
		}
	}
}

// ── HTTP handlers ─────────────────────────────────────────────────────────────

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	ready := s.ready
	s.mu.RUnlock()
	w.Header().Set("Content-Type", "text/plain")
	if ready {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("not ready"))
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := render.WriteShell(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleAPIclusters(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Collect names from both cache and scanErrs so failed clusters appear too.
	allNames := make(map[string]struct{})
	for n := range s.cache {
		allNames[n] = struct{}{}
	}
	for n := range s.scanErrs {
		allNames[n] = struct{}{}
	}

	summaries := make([]ClusterSummary, 0, len(allNames))
	for name := range allNames {
		cs := ClusterSummary{
			Name:     name,
			LastScan: s.scanned[name].Format(time.RFC3339),
		}
		if errMsg, ok := s.scanErrs[name]; ok {
			cs.ScanError = errMsg
		}
		if p, ok := s.cache[name]; ok {
			cs.Tech = p.Technology
			cs.NodeCount = len(p.Graph.Nodes)
			for _, n := range p.Graph.Nodes {
				if n.Type == "apps" || n.Type == "argoapp" {
					cs.AppCount++
				}
			}
			if s.opts.Live {
				health := make(map[string]int)
				for _, n := range p.Graph.Nodes {
					if n.Status != "" {
						health[n.Status]++
					}
				}
				if len(health) > 0 {
					cs.HealthSummary = health
				}
			}
		}
		if t, ok := s.refreshed[name]; ok {
			cs.LastRefreshed = t.Format(time.RFC3339)
		}
		if st, ok := s.liveStatus[name]; ok {
			cs.Reachability = st.Reachability
			cs.LiveError = st.Error
		}
		summaries = append(summaries, cs)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(summaries)
}

func (s *Server) handleCluster(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/clusters/")
	name = strings.Trim(name, "/")
	if name == "" {
		http.NotFound(w, r)
		return
	}

	// Sanitize: prevent directory traversal
	if strings.Contains(name, "..") || strings.Contains(name, "/") {
		http.NotFound(w, r)
		return
	}

	s.mu.RLock()
	profile, ok := s.cache[name]
	s.mu.RUnlock()

	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := render.WriteHTML(w, profile); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// ── preview scanning ─────────────────────────────────────────────────────────

func (s *Server) scanPreview() {
	if s.opts.PreviewRoot == "" {
		return
	}
	profiles := scanDir(s.opts.PreviewRoot)
	s.previewMu.Lock()
	s.previewCache = profiles
	s.previewMu.Unlock()
	log.Printf("preview scan complete  clusters=%d", len(profiles))
}

func (s *Server) watchPreview() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("preview fsnotify: %v", err)
		return
	}
	defer func() { _ = watcher.Close() }()

	if err := watcher.Add(s.opts.PreviewRoot); err != nil {
		log.Printf("preview fsnotify add: %v", err)
		return
	}

	var debounceTimer *time.Timer
	for {
		select {
		case _, ok := <-watcher.Events:
			if !ok {
				return
			}
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.AfterFunc(500*time.Millisecond, func() {
				s.scanPreview()
			})
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("preview fsnotify error: %v", err)
		}
	}
}

// scanDir parses all cluster subdirectories (or the root itself) and returns
// a map of cluster name → profile. Shared by both base and preview scanning.
func scanDir(root string) map[string]*graph.ClusterProfile {
	profiles := make(map[string]*graph.ClusterProfile)

	// Check if root itself is a single cluster.
	hasSubClusters := false
	if dirEntries, err := os.ReadDir(root); err == nil {
		for _, e := range dirEntries {
			if e.IsDir() && scan.HasYAML(filepath.Join(root, e.Name())) {
				hasSubClusters = true
				break
			}
		}
	}

	if !hasSubClusters && scan.HasYAML(root) {
		if p := parseCluster(root); p != nil {
			profiles[filepath.Base(root)] = p
		}
		return profiles
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		log.Printf("scanDir: %v", err)
		return profiles
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		if p := parseCluster(dir); p != nil {
			profiles[e.Name()] = p
		}
	}
	return profiles
}

// parseCluster detects technology and parses a single cluster directory.
func parseCluster(dir string) *graph.ClusterProfile {
	tech := scan.DetectTech(dir)
	var profile *graph.ClusterProfile
	var err error
	switch tech {
	case "argocd":
		profile, err = argocd.ParseDir(dir)
	default:
		profile, err = flux.ParseDir(dir)
	}
	if err != nil {
		log.Printf("preview parse %s: %v", dir, err)
		return nil
	}
	return profile
}

// ── preview HTTP handlers ────────────────────────────────────────────────────

func profileNames(profiles map[string]*graph.ClusterProfile) []string {
	names := make([]string, 0, len(profiles))
	for n := range profiles {
		names = append(names, n)
	}
	return names
}

func (s *Server) sanitizeClusterName(path, prefix string) (string, bool) {
	name := strings.TrimPrefix(path, prefix)
	name = strings.Trim(name, "/")
	if name == "" || strings.Contains(name, "..") || strings.Contains(name, "/") {
		return "", false
	}
	return name, true
}

// resolvePreview resolves the base and head profiles for a preview request.
// It supports two modes:
//   - Sidecar mode (no query params): uses previewCache from -preview-root
//   - On-demand mode (?repo=<url>&ref=<ref> or ?repo=<url>&pr=<N>): clones the
//     repo at the given ref, parses it, and returns the matching cluster profile
func (s *Server) resolvePreview(name string, r *http.Request) (base, head *graph.ClusterProfile, err error) {
	// Base profile always comes from the main cache.
	s.mu.RLock()
	base = s.cache[name]
	s.mu.RUnlock()

	repo := r.URL.Query().Get("repo")
	ref := r.URL.Query().Get("ref")
	pr := r.URL.Query().Get("pr")

	// On-demand mode: clone the repo at the requested ref.
	if repo != "" && (ref != "" || pr != "") {
		if pr != "" && ref == "" {
			ref = "refs/pull/" + pr + "/head"
		}

		// Clone the PR head ref.
		headProfiles, cloneErr := s.cloner.FetchProfiles(repo, ref)
		if cloneErr != nil {
			return nil, nil, cloneErr
		}
		log.Printf("preview: cloned %s@%s  found clusters: %v", repo, ref, profileNames(headProfiles))
		head = headProfiles[name]
		if head == nil {
			head = headProfiles["default"]
		}

		// If base wasn't found in the server cache, clone the base branch too.
		// This handles repos with deep nesting where the server's root scan
		// indexes clusters under different names.
		if base == nil {
			baseBranch := r.URL.Query().Get("base")
			if baseBranch == "" {
				baseBranch = "main"
			}
			baseProfiles, baseErr := s.cloner.FetchProfiles(repo, baseBranch)
			if baseErr != nil {
				log.Printf("preview: failed to clone base %s@%s: %v", repo, baseBranch, baseErr)
			} else {
				base = baseProfiles[name]
				if base == nil {
					base = baseProfiles["default"]
				}
				log.Printf("preview: cloned base %s@%s  found clusters: %v", repo, baseBranch, profileNames(baseProfiles))
			}
		}

		if head == nil && base == nil {
			log.Printf("preview: cluster %q not found in cloned profiles", name)
		}
		return base, head, nil
	}

	// Sidecar mode: use the previewCache from -preview-root.
	s.previewMu.RLock()
	head = s.previewCache[name]
	s.previewMu.RUnlock()

	return base, head, nil
}

func (s *Server) handleAPIpreview(w http.ResponseWriter, r *http.Request) {
	name, ok := s.sanitizeClusterName(r.URL.Path, "/api/preview/")
	if !ok {
		http.NotFound(w, r)
		return
	}

	baseProfile, headProfile, err := s.resolvePreview(name, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if baseProfile == nil && headProfile == nil {
		http.NotFound(w, r)
		return
	}

	var baseGraph, headGraph graph.Graph
	if baseProfile != nil {
		baseGraph = baseProfile.Graph
	}
	if headProfile != nil {
		headGraph = headProfile.Graph
	}

	gd := diff.Diff(baseGraph, headGraph)
	gd.ClusterName = name

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(gd)
}

func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	name, ok := s.sanitizeClusterName(r.URL.Path, "/preview/")
	if !ok {
		http.NotFound(w, r)
		return
	}

	baseProfile, headProfile, err := s.resolvePreview(name, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if baseProfile == nil && headProfile == nil {
		http.NotFound(w, r)
		return
	}

	var baseGraph, headGraph graph.Graph
	if baseProfile != nil {
		baseGraph = baseProfile.Graph
	}
	if headProfile != nil {
		headGraph = headProfile.Graph
	}

	gd := diff.Diff(baseGraph, headGraph)
	gd.ClusterName = name

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := render.WriteDiffHTML(w, gd, baseProfile, headProfile); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
