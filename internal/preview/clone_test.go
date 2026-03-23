package preview

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestShallowClone_PublicRepo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}

	dir, err := shallowClone("https://github.com/stuttgart-things/clusterscope.git", "main")
	if err != nil {
		t.Fatalf("shallowClone: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	// Verify the clone contains expected files.
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		t.Errorf("expected go.mod in clone, got: %v", err)
	}
}

func TestCloner_CacheHit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}

	c := NewCloner(5 * time.Minute)

	// First call: clone.
	profiles1, err := c.FetchProfiles("https://github.com/stuttgart-things/clusterscope.git", "main")
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}

	// Second call: should be cached (fast).
	start := time.Now()
	profiles2, err := c.FetchProfiles("https://github.com/stuttgart-things/clusterscope.git", "main")
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	elapsed := time.Since(start)

	if elapsed > 100*time.Millisecond {
		t.Errorf("second fetch took %v, expected cache hit", elapsed)
	}

	// Both should return the same result (pointers may differ but content same).
	if len(profiles1) != len(profiles2) {
		t.Errorf("cache returned different profile count: %d vs %d", len(profiles1), len(profiles2))
	}
}

func TestParseDir_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	profiles := parseDir(dir)
	if len(profiles) != 0 {
		t.Errorf("expected 0 profiles from empty dir, got %d", len(profiles))
	}
}

func TestIsHidden(t *testing.T) {
	tests := []struct {
		name   string
		hidden bool
	}{
		{".git", true},
		{".hidden", true},
		{"visible", false},
		{"clusters", false},
	}
	for _, tt := range tests {
		if got := isHidden(tt.name); got != tt.hidden {
			t.Errorf("isHidden(%q) = %v, want %v", tt.name, got, tt.hidden)
		}
	}
}
