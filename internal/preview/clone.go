// Package preview provides on-demand git clone and parse for PR preview.
// It shallow-clones a repo at a given ref into a temp directory, parses
// the cluster profiles, and caches the result with a TTL.
package preview

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/stuttgart-things/clusterscope/internal/graph"
	"github.com/stuttgart-things/clusterscope/internal/scan"
	"github.com/stuttgart-things/clusterscope/pkg/argocd"
	"github.com/stuttgart-things/clusterscope/pkg/flux"
)

// DefaultTTL is how long a cached clone result is valid.
const DefaultTTL = 5 * time.Minute

// CloneTimeout is the maximum time allowed for a git clone operation.
const CloneTimeout = 60 * time.Second

// cacheEntry holds a parsed result with its expiry time.
type cacheEntry struct {
	profiles map[string]*graph.ClusterProfile
	cloneDir string
	expires  time.Time
}

// Cloner manages on-demand git clones with a TTL cache.
type Cloner struct {
	mu    sync.Mutex
	cache map[string]cacheEntry // key: repo+"|"+ref
	ttl   time.Duration
}

// NewCloner creates a Cloner with the given cache TTL.
func NewCloner(ttl time.Duration) *Cloner {
	c := &Cloner{
		cache: make(map[string]cacheEntry),
		ttl:   ttl,
	}
	go c.cleanupLoop()
	return c
}

// FetchProfiles clones the repo at the given ref (or returns cached results)
// and parses all cluster directories found in the clone.
func (c *Cloner) FetchProfiles(repo, ref string) (map[string]*graph.ClusterProfile, error) {
	key := repo + "|" + ref

	c.mu.Lock()
	if entry, ok := c.cache[key]; ok && time.Now().Before(entry.expires) {
		c.mu.Unlock()
		return entry.profiles, nil
	}
	c.mu.Unlock()

	// Clone into a temp dir.
	dir, err := shallowClone(repo, ref)
	if err != nil {
		return nil, fmt.Errorf("git clone: %w", err)
	}

	profiles := parseDir(dir)
	if len(profiles) == 0 {
		// Clean up immediately if nothing was found.
		_ = os.RemoveAll(dir)
		return profiles, nil
	}

	c.mu.Lock()
	// Evict previous entry for this key.
	if old, ok := c.cache[key]; ok {
		_ = os.RemoveAll(old.cloneDir)
	}
	c.cache[key] = cacheEntry{
		profiles: profiles,
		cloneDir: dir,
		expires:  time.Now().Add(c.ttl),
	}
	c.mu.Unlock()

	log.Printf("preview: cloned %s@%s  clusters=%d", repo, ref, len(profiles))
	return profiles, nil
}

// shallowClone performs a depth-1 clone of a repo at a specific ref.
func shallowClone(repo, ref string) (string, error) {
	dir, err := os.MkdirTemp("", "clusterscope-preview-*")
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), CloneTimeout)
	defer cancel()

	// git init + fetch is more reliable for arbitrary refs (e.g. refs/pull/N/head)
	// than git clone --branch which only works for branches/tags.
	cmds := [][]string{
		{"git", "init", dir},
		{"git", "-C", dir, "remote", "add", "origin", repo},
		{"git", "-C", dir, "fetch", "--depth=1", "origin", ref},
		{"git", "-C", dir, "checkout", "FETCH_HEAD"},
	}

	for _, args := range cmds {
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			_ = os.RemoveAll(dir)
			return "", fmt.Errorf("%s: %w\n%s", args[0], err, string(out))
		}
	}

	return dir, nil
}

// parseDir discovers and parses cluster directories in a cloned repo.
func parseDir(root string) map[string]*graph.ClusterProfile {
	profiles := make(map[string]*graph.ClusterProfile)

	// Check if root itself is a single cluster.
	hasSubClusters := false
	if dirEntries, err := os.ReadDir(root); err == nil {
		for _, e := range dirEntries {
			if e.IsDir() && !isHidden(e.Name()) && scan.HasYAML(root+"/"+e.Name()) {
				hasSubClusters = true
				break
			}
		}
	}

	if !hasSubClusters && scan.HasYAML(root) {
		if p := parseCluster(root); p != nil {
			profiles["default"] = p
		}
		return profiles
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return profiles
	}
	for _, e := range entries {
		if !e.IsDir() || isHidden(e.Name()) {
			continue
		}
		dir := root + "/" + e.Name()
		if p := parseCluster(dir); p != nil {
			profiles[e.Name()] = p
		}
	}
	return profiles
}

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

func isHidden(name string) bool {
	return len(name) > 0 && name[0] == '.'
}

// cleanupLoop periodically removes expired cache entries and their temp dirs.
func (c *Cloner) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, entry := range c.cache {
			if now.After(entry.expires) {
				_ = os.RemoveAll(entry.cloneDir)
				delete(c.cache, key)
			}
		}
		c.mu.Unlock()
	}
}
