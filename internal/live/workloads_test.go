package live

import (
	"errors"
	"testing"
	"time"

	"github.com/stuttgart-things/clusterscope/internal/graph"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	dynfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

func obj(apiVersion, kind, ns, name string, labels map[string]any, spec, status map[string]any) *unstructured.Unstructured {
	meta := map[string]any{"name": name, "namespace": ns, "creationTimestamp": "2026-10-14T08:00:00Z"}
	if labels != nil {
		meta["labels"] = labels
	}
	o := map[string]any{"apiVersion": apiVersion, "kind": kind, "metadata": meta}
	if spec != nil {
		o["spec"] = spec
	}
	if status != nil {
		o["status"] = status
	}
	return &unstructured.Unstructured{Object: o}
}

func deployment(ns, name string, labels map[string]any, replicas, available int64) *unstructured.Unstructured {
	return obj("apps/v1", "Deployment", ns, name, labels,
		map[string]any{"replicas": replicas, "selector": map[string]any{"matchLabels": map[string]any{"app": name}}},
		map[string]any{"availableReplicas": available})
}

func pod(ns, name, app, waiting string, restarts int64) *unstructured.Unstructured {
	cs := map[string]any{"name": "main", "restartCount": restarts, "state": map[string]any{"running": map[string]any{}}}
	if waiting != "" {
		cs["state"] = map[string]any{"waiting": map[string]any{"reason": waiting}}
	}
	return obj("v1", "Pod", ns, name, map[string]any{"app": app}, nil,
		map[string]any{"phase": "Running", "containerStatuses": []any{cs}})
}

func kustomizationWithInventory(name, ready string, ids ...string) *unstructured.Unstructured {
	entries := []any{}
	for _, id := range ids {
		entries = append(entries, map[string]any{"id": id, "v": "v1"})
	}
	return obj("kustomize.toolkit.fluxcd.io/v1", "Kustomization", "flux-system", name, nil, nil, map[string]any{
		"conditions": []any{map[string]any{"type": "Ready", "status": ready, "message": "Applied revision"}},
		"inventory":  map[string]any{"entries": entries},
	})
}

func TestParseInventoryID(t *testing.T) {
	r, ok := parseInventoryID("demo_web_apps_Deployment")
	if !ok || r != (objRef{Group: "apps", Kind: "Deployment", Namespace: "demo", Name: "web"}) {
		t.Errorf("namespaced: %+v", r)
	}
	r, ok = parseInventoryID("_demo__Namespace")
	if !ok || r.Kind != "Namespace" || r.Name != "demo" || r.Namespace != "" || r.Group != "" {
		t.Errorf("cluster-scoped core: %+v", r)
	}
	if _, ok := parseInventoryID("garbage"); ok {
		t.Error("garbage parsed")
	}
}

func TestEnrich_WorkloadHealthFlux(t *testing.T) {
	dir := t.TempDir()
	prod := writeKubeconfig(t, dir, "prod", "prod")

	helm := map[string]any{labelHelmName: "redis", labelHelmNamespace: "demo"}
	objs := []runtime.Object{
		// GitOps Ready, but web runs 1/2 with a crashlooping pod and the
		// HelmRelease-managed StatefulSet is down → drift.
		kustomizationWithInventory("demo", "True",
			"_demo__Namespace",
			"demo_web_apps_Deployment",
			"demo_web_v1_Service",
			"demo_redis_helm.toolkit.fluxcd.io_HelmRelease"),
		deployment("demo", "web", nil, 2, 1),
		pod("demo", "web-1", "web", "", 0),
		pod("demo", "web-2", "web", "CrashLoopBackOff", 7),
		deployment("demo", "redis-master", helm, 1, 1),
		obj("apps/v1", "StatefulSet", "demo", "redis-replicas", helm,
			map[string]any{"replicas": int64(2), "selector": map[string]any{"matchLabels": map[string]any{"app": "redis-replicas"}}},
			map[string]any{}),
		obj("helm.toolkit.fluxcd.io/v2", "HelmRelease", "demo", "redis", nil, nil, map[string]any{
			"conditions": []any{map[string]any{"type": "Ready", "status": "False", "message": "install retries exhausted"}},
		}),

		// Everything up.
		kustomizationWithInventory("ok", "True", "ok_api_apps_Deployment", "ok_agent_apps_DaemonSet"),
		deployment("ok", "api", nil, 3, 3),
		obj("apps/v1", "DaemonSet", "ok", "agent", nil, nil, map[string]any{"desiredNumberScheduled": int64(4), "numberAvailable": int64(4)}),

		// Config only: no workloads.
		kustomizationWithInventory("config", "True", "config_settings_v1_ConfigMap"),

		// flux-operator sync root with the controllers in its inventory.
		obj("fluxcd.controlplane.io/v1", "FluxInstance", "flux-system", "flux", nil, nil, map[string]any{
			"conditions": []any{map[string]any{"type": "Ready", "status": "True"}},
			"inventory":  map[string]any{"entries": []any{map[string]any{"id": "flux-system_source-controller_apps_Deployment"}}},
		}),
		deployment("flux-system", "source-controller", nil, 1, 1),
	}
	c := fakeClients(ClientConfig{KubeconfigDir: dir}, map[string][]runtime.Object{prod: objs}, nil, nil)

	p := fluxProfile("demo", "ok", "config", "sync-root", "not-deployed")
	st := c.Enrich(p, "prod")
	if st.Reachability != Reachable || st.WorkloadError != "" {
		t.Fatalf("status: %+v", st)
	}
	byID := map[string]graph.Node{}
	for _, n := range p.Graph.Nodes {
		byID[n.ID] = n
	}

	d := byID["demo"].Workloads
	if d == nil || d.Status != graph.WorkloadDegraded || d.Ready != 1 || d.Total != 3 || !d.Drift {
		t.Fatalf("demo: %+v", d)
	}
	if len(d.HelmReleases) != 1 || d.HelmReleases[0].Status != "failed" {
		t.Errorf("demo helm releases: %+v", d.HelmReleases)
	}
	for _, w := range d.Workloads {
		switch w.Name {
		case "web":
			if w.Status != graph.WorkloadDegraded || len(w.Issues) != 1 || w.Issues[0] != "web-2: CrashLoopBackOff" {
				t.Errorf("web: %+v", w)
			}
			if w.Restarts != 0 {
				t.Errorf("first refresh must not report restarts: %+v", w)
			}
		case "redis-replicas":
			if w.Status != graph.WorkloadDown || w.Desired != 2 {
				t.Errorf("redis-replicas: %+v", w)
			}
		case "redis-master":
			if w.Status != graph.WorkloadHealthy {
				t.Errorf("redis-master: %+v", w)
			}
		default:
			t.Errorf("unexpected workload %+v", w)
		}
	}

	if o := byID["ok"].Workloads; o == nil || o.Status != graph.WorkloadHealthy || o.Ready != 2 || o.Drift {
		t.Errorf("ok: %+v", o)
	}
	if cfg := byID["config"].Workloads; cfg == nil || cfg.Status != graph.WorkloadNA || cfg.Drift {
		t.Errorf("config: %+v", cfg)
	}
	sr := byID["sync-root"]
	if sr.Status != "ready" || sr.Workloads == nil || sr.Workloads.Total != 1 {
		t.Errorf("sync-root: status=%q workloads=%+v", sr.Status, sr.Workloads)
	}
	if byID["not-deployed"].Workloads != nil {
		t.Errorf("node without live object must have no workload health")
	}
}

func TestEnrich_WorkloadHealthArgo(t *testing.T) {
	dir := t.TempDir()
	prod := writeKubeconfig(t, dir, "prod", "prod")
	app := obj("argoproj.io/v1alpha1", "Application", "argocd", "shop", nil, nil, map[string]any{
		"health": map[string]any{"status": "Healthy"},
		"sync":   map[string]any{"status": "Synced"},
		"resources": []any{
			map[string]any{"group": "apps", "kind": "Deployment", "namespace": "shop", "name": "front"},
			map[string]any{"kind": "Service", "namespace": "shop", "name": "front"},
		},
	})
	c := fakeClients(ClientConfig{KubeconfigDir: dir},
		map[string][]runtime.Object{prod: {app, deployment("shop", "front", nil, 2, 0)}}, nil, nil)

	p := &graph.ClusterProfile{Technology: "argocd", Graph: graph.Graph{Nodes: []graph.Node{{ID: "shop"}}}}
	c.Enrich(p, "prod")
	w := p.Graph.Nodes[0].Workloads
	if p.Graph.Nodes[0].Status != "ready" || w == nil || w.Status != graph.WorkloadDown || !w.Drift {
		t.Errorf("argo shop: status=%q workloads=%+v", p.Graph.Nodes[0].Status, w)
	}
}

func TestEvalWorkload_RestartsSincePreviousRefresh(t *testing.T) {
	w := &workloadObj{
		ref:     objRef{Group: "apps", Kind: "Deployment", Namespace: "demo", Name: "web"},
		ready:   1,
		desired: 1,
		pods:    []podInfo{{name: "web-1", restarts: 5}},
	}
	ev, total := evalWorkload(w, nil)
	if ev.Restarts != 0 || total != 5 || ev.Status != graph.WorkloadHealthy {
		t.Errorf("first run: %+v total=%d", ev, total)
	}
	w.pods[0].restarts = 8
	ev, _ = evalWorkload(w, map[string]int32{w.ref.key(): total})
	if ev.Restarts != 3 {
		t.Errorf("restarts since previous refresh: %d, want 3", ev.Restarts)
	}
	// pod replaced → counter reset, no negative delta
	w.pods[0].restarts = 0
	if ev, _ = evalWorkload(w, map[string]int32{w.ref.key(): 8}); ev.Restarts != 0 {
		t.Errorf("after pod replacement: %d", ev.Restarts)
	}
}

func TestToPodInfo_PendingGrace(t *testing.T) {
	now := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	p := podItem{}
	p.Metadata.Name = "web-1"
	p.Status.Phase = "Pending"
	p.Status.Conditions = []k8sCondition{{Type: "PodScheduled", Status: "False"}}

	p.Metadata.CreationTimestamp = now.Add(-30 * time.Second)
	if got := toPodInfo(p, now).issues; len(got) != 0 {
		t.Errorf("fresh pending pod reported: %v", got)
	}
	p.Metadata.CreationTimestamp = now.Add(-10 * time.Minute)
	if got := toPodInfo(p, now).issues; len(got) != 1 || got[0] != "Pending (unschedulable)" {
		t.Errorf("old pending pod: %v", got)
	}
}

func TestEnrich_WorkloadListForbidden(t *testing.T) {
	dir := t.TempDir()
	writeKubeconfig(t, dir, "prod", "prod")
	c := fakeClients(ClientConfig{KubeconfigDir: dir}, nil, nil, nil)
	inner := c.newClients
	c.newClients = func(path string) (dynamic.Interface, pingFunc, error) {
		dyn, ping, err := inner(path)
		dyn.(*dynfake.FakeDynamicClient).PrependReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewForbidden(gvrPods.GroupResource(), "", errors.New("rbac"))
		})
		return dyn, ping, err
	}

	p := fluxProfile("apps")
	p.Graph.Nodes[0].Workloads = &graph.WorkloadHealth{Status: graph.WorkloadHealthy}
	st := c.Enrich(p, "prod")
	if st.Reachability != Reachable || st.WorkloadError == "" {
		t.Errorf("status: %+v, want reachable with workload error", st)
	}
	if p.Graph.Nodes[0].Workloads != nil {
		t.Error("stale workload health not cleared")
	}
}

func TestToPodInfo_CrashLoopBetweenBackoffs(t *testing.T) {
	var p podItem
	p.Metadata.Name = "snap-1"
	p.Status.Phase = "Running"
	cs := containerStatus{Name: "main", RestartCount: 12}
	cs.LastState.Terminated = &struct {
		Reason   string `json:"reason"`
		ExitCode int32  `json:"exitCode"`
	}{Reason: "Error", ExitCode: 1}
	p.Status.Containers = []containerStatus{cs}

	if got := toPodInfo(p, time.Now()).issues; len(got) != 1 || got[0] != "restarting (last exit: Error, code 1)" {
		t.Errorf("not-ready restarted container: %v", got)
	}
	p.Status.Containers[0].Ready = true // recovered after earlier restarts
	if got := toPodInfo(p, time.Now()).issues; len(got) != 0 {
		t.Errorf("ready container reported: %v", got)
	}
}
