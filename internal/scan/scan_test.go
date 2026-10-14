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

func TestDiscoverAll_RootsIncludeOverrides(t *testing.T) {
	base := t.TempDir()
	st := filepath.Join(base, "stuttgart-things", "clusters", "labul")
	hv := filepath.Join(base, "harvester", "clusters")
	mkCluster(t, filepath.Join(st, "vsphere", "platform-sthings"))
	mkCluster(t, filepath.Join(hv, "edge-test2"))
	mkCluster(t, filepath.Join(hv, "homerun2-dev"))
	mkCluster(t, filepath.Join(hv, "platform-sthings")) // same name as in st

	cfg := Config{
		Roots: []RootSpec{
			{Path: st},
			{Path: hv, Name: "harvester", Lab: "harvester", Include: []string{"edge-test2", "platform-sthings"}},
			{Path: filepath.Join(base, "not-cloned-yet")},
		},
		Clusters: map[string]Override{"edge-test2": {Name: "edge-tt-test2", Lab: "labul"}},
	}
	dirs := DiscoverAll(cfg, "")
	if len(dirs) != 3 {
		t.Fatalf("got %+v", dirs)
	}
	if d := dirs[0]; d.Name != "platform-sthings" || d.Lab != "labul" || d.Rel != "vsphere/platform-sthings" || d.Duplicate {
		t.Errorf("stuttgart-things cluster: %+v", d)
	}
	if d := dirs[1]; d.Name != "edge-tt-test2" || d.Dir != "edge-test2" || d.Lab != "labul" || d.Rel != "harvester/edge-test2" {
		t.Errorf("override: %+v", d)
	}
	if d := dirs[2]; d.Name != "platform-sthings" || d.Lab != "harvester" || !d.Duplicate {
		t.Errorf("duplicate across roots with root lab: %+v", d)
	}
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	y := write("c.yaml", "roots:\n  - path: /a\n  - path: /b\n    name: b\n    include: [x]\nclusters:\n  x: {name: edge-tt-test2, lab: l}\n")
	c, err := LoadConfig(y)
	if err != nil || len(c.Roots) != 2 || c.Roots[1].Include[0] != "x" || c.Clusters["x"].Name != "edge-tt-test2" {
		t.Errorf("yaml: %+v %v", c, err)
	}
	// JSON on one line, as substituted by Flux into a ConfigMap
	j := write("c.json", `{"roots":[{"path":"/a","lab":"labul"}],"clusters":{"x":{"lab":"l"}}}`)
	if c, err := LoadConfig(j); err != nil || c.Roots[0].Lab != "labul" {
		t.Errorf("json: %+v %v", c, err)
	}
	for _, bad := range []string{"roots: []\n", "roots:\n  - name: x\n", "roots: [\n"} {
		if _, err := LoadConfig(write("bad.yaml", bad)); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}
