package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func mkCluster(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "kustomization.yaml"), []byte("kind: Kustomization\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func names(dirs []ClusterDir) map[string]ClusterDir {
	m := map[string]ClusterDir{}
	for _, d := range dirs {
		if !d.Duplicate {
			m[d.Name] = d
		}
	}
	return m
}

func TestDiscover_Nested(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo", "clusters")
	mkCluster(t, filepath.Join(root, "labul", "vsphere", "platform-sthings"))
	mkCluster(t, filepath.Join(root, "labul", "vsphere", "infra-sthings"))
	mkCluster(t, filepath.Join(root, "labda", "vsphere", "sthings-infra"))
	mkCluster(t, filepath.Join(root, "labda", "vsphere", "platform-sthings")) // same name, other lab
	mkCluster(t, filepath.Join(root, "labul", ".git", "hooks"))               // hidden

	dirs := Discover(root, "")
	if len(dirs) != 4 {
		t.Fatalf("got %d dirs: %+v", len(dirs), dirs)
	}
	m := names(dirs)
	if m["infra-sthings"].Lab != "labul" || m["sthings-infra"].Lab != "labda" {
		t.Errorf("labs: %+v", m)
	}
	// labda sorts first → labul/…/platform-sthings is the duplicate
	if p := m["platform-sthings"]; p.Lab != "labda" {
		t.Errorf("first platform-sthings: %+v", p)
	}
	dups := 0
	for _, d := range dirs {
		if d.Duplicate {
			dups++
			if d.Lab != "labul" {
				t.Errorf("duplicate: %+v", d)
			}
		}
	}
	if dups != 1 {
		t.Errorf("duplicates: %d", dups)
	}
}

func TestDiscover_FlatLabFromRootPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "clusters", "labul", "vsphere")
	mkCluster(t, filepath.Join(root, "platform-sthings"))
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	m := names(Discover(root, "fallback"))
	if len(m) != 2 { // flat layout keeps every subdirectory, like before
		t.Fatalf("got %+v", m)
	}
	if m["platform-sthings"].Lab != "labul" {
		t.Errorf("lab from root path: %+v", m["platform-sthings"])
	}
}

func TestDiscover_FallbackLab(t *testing.T) {
	root := filepath.Join(t.TempDir(), "harvester", "clusters")
	mkCluster(t, filepath.Join(root, "edge")) // clusters/<cluster>: no lab segment
	m := names(Discover(root, "lab-x"))
	if m["edge"].Lab != "lab-x" {
		t.Errorf("fallback lab: %+v", m)
	}
	if m := names(Discover(root, "")); m["edge"].Lab != DefaultLab {
		t.Errorf("default lab: %+v", m)
	}
}

func TestDiscover_RootIsCluster(t *testing.T) {
	root := filepath.Join(t.TempDir(), "prod")
	mkCluster(t, root)
	dirs := Discover(root, "")
	if len(dirs) != 1 || dirs[0].Name != "prod" || dirs[0].Path != root {
		t.Errorf("got %+v", dirs)
	}
}

func TestDiscover_MaxDepth(t *testing.T) {
	root := t.TempDir()
	mkCluster(t, filepath.Join(root, "a", "b", "c", "d", "too-deep"))
	mkCluster(t, filepath.Join(root, "a", "b", "c", "ok"))
	m := names(Discover(root, ""))
	if _, ok := m["ok"]; !ok || len(m) != 1 {
		t.Errorf("got %+v", m)
	}
}
