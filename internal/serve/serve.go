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
	"github.com/stuttgart-things/clusterscope/internal/graph"
	"github.com/stuttgart-things/clusterscope/internal/render"
	"github.com/stuttgart-things/clusterscope/internal/scan"
	"github.com/stuttgart-things/clusterscope/pkg/argocd"
	"github.com/stuttgart-things/clusterscope/pkg/flux"
)

// ClusterSummary is the JSON payload returned by GET /api/clusters.
type ClusterSummary struct {
	Name      string `json:"name"`
	Tech      string `json:"tech"`
	NodeCount int    `json:"nodeCount"`
	LastScan  string `json:"lastScan"`
}

// Server holds the HTTP mux and cluster cache.
type Server struct {
	root    string
	mu      sync.RWMutex
	cache   map[string]*graph.ClusterProfile
	scanned map[string]time.Time
}

// Start starts the HTTP server on addr, scanning root for cluster directories.
func Start(addr, root string) error {
	if _, err := os.Stat(root); err != nil {
		return err
	}

	s := &Server{
		root:    root,
		cache:   make(map[string]*graph.ClusterProfile),
		scanned: make(map[string]time.Time),
	}

	s.scanAll()
	go s.watch()

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/clusters", s.handleAPIclusters)
	mux.HandleFunc("/clusters/", s.handleCluster)

	log.Printf("clusterscope serving on %s  root=%s", addr, root)
	return http.ListenAndServe(addr, mux) //nolint:gosec
}

// ── scanning ──────────────────────────────────────────────────────────────────

func (s *Server) scanAll() {
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
		return
	}

	s.mu.Lock()
	s.cache[name] = profile
	s.scanned[name] = time.Now()
	s.mu.Unlock()
	log.Printf("scanned %s  tech=%s  nodes=%d", name, tech, len(profile.Graph.Nodes))
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

	summaries := make([]ClusterSummary, 0, len(s.cache))
	for name, p := range s.cache {
		summaries = append(summaries, ClusterSummary{
			Name:      name,
			Tech:      p.Technology,
			NodeCount: len(p.Graph.Nodes),
			LastScan:  s.scanned[name].Format(time.RFC3339),
		})
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
