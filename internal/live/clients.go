package live

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/stuttgart-things/clusterscope/internal/graph"
	"gopkg.in/yaml.v3"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// Reachability values reported per cluster.
const (
	Reachable    = "reachable"
	Unreachable  = "unreachable"
	Unconfigured = "unconfigured"
)

// ClusterStatus is the cluster-level result of one live enrichment run.
type ClusterStatus struct {
	Reachability string
	Error        string
	Source       string // kubeconfig path, or "default" for -kubeconfig / in-cluster
	CheckedAt    time.Time
	// Endpoints is the Gateway API inventory; nil unless the cluster is
	// reachable.
	Endpoints *Endpoints
	// WorkloadError is set when workloads or pods could not be listed
	// (typically missing RBAC); node workload health is then absent.
	WorkloadError string
}

// ClientConfig controls how a kubeconfig is resolved for each cluster.
//
// Without KubeconfigDir and Map (legacy mode) every cluster uses the default
// client (Kubeconfig, or in-cluster ServiceAccount). As soon as per-cluster
// kubeconfigs are configured, the default client is only used for the cluster
// named Self — all other clusters without a kubeconfig are "unconfigured"
// instead of being silently looked up in the wrong cluster.
type ClientConfig struct {
	// KubeconfigDir holds one kubeconfig file per cluster, named after the
	// cluster directory (e.g. a mounted Secret with one key per cluster).
	KubeconfigDir string
	// Map overrides the file per cluster: cluster name → kubeconfig path.
	// Relative paths are resolved against KubeconfigDir.
	Map map[string]string
	// Kubeconfig is the default kubeconfig path. Empty means in-cluster
	// ServiceAccount, then KUBECONFIG / ~/.kube/config.
	Kubeconfig string
	// Self names the cluster clusterscope runs in (uses the default client).
	Self string
}

// LoadMapFile reads a kubeconfig mapping file:
//
//	clusters:
//	  platform-sthings: platform-sthings.yaml
//	  homerun2-test1: /kubeconfigs/labul/homerun2-test1
func LoadMapFile(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return nil, err
	}
	var f struct {
		Clusters map[string]string `yaml:"clusters"`
	}
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return f.Clusters, nil
}

// pingFunc checks that the cluster's API server answers.
type pingFunc func(ctx context.Context) error

type kubeClient struct {
	dynamic dynamic.Interface
	ping    pingFunc
	modTime time.Time
	size    int64
}

// clientFactory builds the clients for a kubeconfig path ("" = default rules).
type clientFactory func(path string) (dynamic.Interface, pingFunc, error)

// Clients resolves and caches one Kubernetes client per cluster.
type Clients struct {
	cfg        ClientConfig
	newClients clientFactory

	mu    sync.Mutex
	cache map[string]*kubeClient // key: kubeconfig path ("" = default)
	// restarts holds the container restart totals per cluster and workload
	// from the previous refresh.
	restarts map[string]map[string]int32
}

// NewClients returns a client resolver for cfg.
func NewClients(cfg ClientConfig) *Clients {
	return &Clients{cfg: cfg, newClients: newClientsForPath, cache: make(map[string]*kubeClient), restarts: make(map[string]map[string]int32)}
}

func (c *Clients) perCluster() bool {
	return c.cfg.KubeconfigDir != "" || len(c.cfg.Map) > 0
}

// resolve returns the kubeconfig path for cluster ("" = default client) and
// whether one is configured at all.
func (c *Clients) resolve(cluster string) (path string, ok bool, err error) {
	if p, found := c.cfg.Map[cluster]; found {
		if !filepath.IsAbs(p) && c.cfg.KubeconfigDir != "" {
			p = filepath.Join(c.cfg.KubeconfigDir, p)
		}
		if _, err := os.Stat(p); err != nil {
			return "", false, fmt.Errorf("mapped kubeconfig for %s: %w", cluster, err)
		}
		return p, true, nil
	}
	if c.cfg.KubeconfigDir != "" {
		p := filepath.Join(c.cfg.KubeconfigDir, cluster)
		if _, err := os.Stat(p); err == nil {
			return p, true, nil
		}
	}
	if !c.perCluster() || cluster == c.cfg.Self {
		return c.cfg.Kubeconfig, true, nil
	}
	return "", false, nil
}

// client returns a cached client for path, rebuilding it when the file
// changed (e.g. Secret rotation).
func (c *Clients) client(path string) (*kubeClient, error) {
	var modTime time.Time
	var size int64
	if path != "" {
		fi, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		modTime, size = fi.ModTime(), fi.Size()
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if kc, ok := c.cache[path]; ok && kc.modTime.Equal(modTime) && kc.size == size {
		return kc, nil
	}
	dyn, ping, err := c.newClients(path)
	if err != nil {
		delete(c.cache, path)
		return nil, err
	}
	kc := &kubeClient{dynamic: dyn, ping: ping, modTime: modTime, size: size}
	c.cache[path] = kc
	return kc, nil
}

// Enrich resolves the client for cluster, checks that its API server answers
// and enriches the profile with live reconciliation status. Errors never
// propagate: an unreachable or unconfigured cluster gets all nodes reset to
// "unknown" so stale status from an earlier run does not linger.
func (c *Clients) Enrich(profile *graph.ClusterProfile, cluster string) ClusterStatus {
	st := ClusterStatus{CheckedAt: time.Now()}

	path, ok, err := c.resolve(cluster)
	switch {
	case err != nil:
		st.Reachability, st.Error = Unconfigured, err.Error()
	case !ok:
		st.Reachability, st.Error = Unconfigured, "no kubeconfig for cluster"
	}
	if st.Reachability != "" {
		markUnknown(profile, st.Error)
		return st
	}
	st.Source = path
	if path == "" {
		st.Source = "default"
	}

	kc, err := c.client(path)
	if err == nil {
		err = kc.ping(context.Background())
	}
	if err != nil {
		log.Printf("live: cluster %s unreachable (%s): %v", cluster, st.Source, err)
		st.Reachability, st.Error = Unreachable, err.Error()
		markUnknown(profile, "cluster unreachable: "+err.Error())
		return st
	}

	st.Reachability = Reachable
	var members map[string][]objRef
	switch profile.Technology {
	case "argocd":
		members = enrichArgoCD(profile, kc.dynamic)
	default:
		members = enrichFlux(profile, kc.dynamic)
	}
	c.enrichWorkloads(profile, cluster, kc.dynamic, members, &st)
	ep := fetchEndpoints(kc.dynamic, profile.Technology)
	st.Endpoints = &ep
	return st
}

// enrichWorkloads evaluates the actual workload health per node.
func (c *Clients) enrichWorkloads(profile *graph.ClusterProfile, cluster string, client dynamic.Interface, members map[string][]objRef, st *ClusterStatus) {
	snap, err := fetchWorkloads(client, profile.Technology, time.Now())
	if err != nil {
		log.Printf("live: cluster %s workloads: %v", cluster, err)
		st.WorkloadError = err.Error()
		clearWorkloads(profile)
		return
	}
	c.mu.Lock()
	prev := c.restarts[cluster]
	c.mu.Unlock()
	cur := applyWorkloads(profile, snap, members, prev)
	c.mu.Lock()
	c.restarts[cluster] = cur
	c.mu.Unlock()
}

func markUnknown(profile *graph.ClusterProfile, msg string) {
	for i := range profile.Graph.Nodes {
		profile.Graph.Nodes[i].Workloads = nil
		profile.Graph.Nodes[i].Status = "unknown"
		profile.Graph.Nodes[i].Message = msg
		profile.Graph.Nodes[i].UpdatedAt = ""
	}
}

func newClientsForPath(path string) (dynamic.Interface, pingFunc, error) {
	cfg, err := newRESTConfig(path)
	if err != nil {
		return nil, nil, err
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, nil, err
	}
	httpClient, err := rest.HTTPClientFor(cfg)
	if err != nil {
		return nil, nil, err
	}
	return dyn, versionPing(cfg.Host, httpClient), nil
}

// versionPing returns a ping that GETs <host>/version. Any HTTP response below
// 500 except 401/403 counts as reachable: the API server answered and accepted
// the credentials.
func versionPing(host string, client *http.Client) pingFunc {
	return func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(host, "/")+"/version", nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			return fmt.Errorf("GET /version: %s (credentials rejected)", resp.Status)
		case resp.StatusCode >= 500:
			return fmt.Errorf("GET /version: %s", resp.Status)
		}
		return nil
	}
}
