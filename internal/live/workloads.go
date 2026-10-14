package live

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/stuttgart-things/clusterscope/internal/graph"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// Actual workload health (#52): the GitOps controller's Ready only reflects
// the last reconcile. This evaluates the Deployments, StatefulSets and
// DaemonSets behind each node at refresh time.

var (
	gvrDeployments  = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	gvrStatefulSets = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}
	gvrDaemonSets   = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}
	gvrPods         = schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	gvrHelmReleases = schema.GroupVersionResource{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"}
)

// Helm-controller labels every object it renders with its HelmRelease.
const (
	labelHelmName      = "helm.toolkit.fluxcd.io/name"
	labelHelmNamespace = "helm.toolkit.fluxcd.io/namespace"
)

// pendingGrace is how long a pod may be Pending before it counts as an issue
// (image pulls and scheduling during a normal rollout).
const pendingGrace = 2 * time.Minute

// podIssueReasons are container waiting reasons that mean the pod will not
// become ready on its own.
var podIssueReasons = map[string]bool{
	"CrashLoopBackOff":           true,
	"ImagePullBackOff":           true,
	"ErrImagePull":               true,
	"InvalidImageName":           true,
	"CreateContainerConfigError": true,
	"CreateContainerError":       true,
	"RunContainerError":          true,
}

// objRef identifies an object; Group is "" for the core API group.
type objRef struct {
	Group, Kind, Namespace, Name string
}

func (r objRef) key() string { return r.Kind + "/" + r.Namespace + "/" + r.Name }

// parseInventoryID parses a Flux inventory entry id
// "<namespace>_<name>_<group>_<kind>" (namespace and group may be empty).
func parseInventoryID(id string) (objRef, bool) {
	p := strings.Split(id, "_")
	if len(p) != 4 {
		return objRef{}, false
	}
	return objRef{Namespace: p[0], Name: p[1], Group: p[2], Kind: p[3]}, true
}

// ── cluster snapshot ─────────────────────────────────────────────────────────

type podInfo struct {
	name     string
	labels   map[string]string
	issues   []string
	restarts int32
}

type workloadObj struct {
	ref      objRef
	labels   map[string]string
	ready    int32
	desired  int32
	selector labels.Selector // nil: no pods are matched
	pods     []podInfo
}

// workloadSnapshot is everything listed once per cluster and refresh.
type workloadSnapshot struct {
	workloads    map[objRef]*workloadObj
	helmReleases map[objRef]graph.HelmReleaseStatus
}

type workloadMeta struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	Labels            map[string]string `json:"labels"`
	CreationTimestamp time.Time         `json:"creationTimestamp"`
}

type workloadItem struct {
	Metadata workloadMeta `json:"metadata"`
	Spec     struct {
		Replicas *int32                `json:"replicas"`
		Selector *metav1.LabelSelector `json:"selector"`
	} `json:"spec"`
	Status struct {
		// Deployment
		AvailableReplicas int32 `json:"availableReplicas"`
		// StatefulSet
		ReadyReplicas int32 `json:"readyReplicas"`
		// DaemonSet
		DesiredNumberScheduled int32 `json:"desiredNumberScheduled"`
		NumberAvailable        int32 `json:"numberAvailable"`
	} `json:"status"`
}

type containerStatus struct {
	Name         string `json:"name"`
	Ready        bool   `json:"ready"`
	RestartCount int32  `json:"restartCount"`
	State        struct {
		Waiting *struct {
			Reason string `json:"reason"`
		} `json:"waiting"`
	} `json:"state"`
	LastState struct {
		Terminated *struct {
			Reason   string `json:"reason"`
			ExitCode int32  `json:"exitCode"`
		} `json:"terminated"`
	} `json:"lastState"`
}

type podItem struct {
	Metadata workloadMeta `json:"metadata"`
	Status   struct {
		Phase      string            `json:"phase"`
		Conditions []k8sCondition    `json:"conditions"`
		InitStatus []containerStatus `json:"initContainerStatuses"`
		Containers []containerStatus `json:"containerStatuses"`
	} `json:"status"`
}

func listItems(client dynamic.Interface, gvr schema.GroupVersionResource) ([]unstructured.Unstructured, error) {
	list, err := client.Resource(gvr).Namespace("").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", gvr.Resource, err)
	}
	return list.Items, nil
}

func decode(item unstructured.Unstructured, out any) error {
	raw, err := item.MarshalJSON()
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

// fetchWorkloads lists workloads, pods and (on Flux clusters) HelmReleases.
// Workloads and pods are required; a missing HelmRelease CRD is not an error.
func fetchWorkloads(client dynamic.Interface, technology string, now time.Time) (*workloadSnapshot, error) {
	snap := &workloadSnapshot{
		workloads:    make(map[objRef]*workloadObj),
		helmReleases: make(map[objRef]graph.HelmReleaseStatus),
	}

	kinds := []struct {
		gvr  schema.GroupVersionResource
		kind string
	}{
		{gvrDeployments, "Deployment"},
		{gvrStatefulSets, "StatefulSet"},
		{gvrDaemonSets, "DaemonSet"},
	}
	for _, k := range kinds {
		items, err := listItems(client, k.gvr)
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			var w workloadItem
			if decode(it, &w) != nil {
				continue
			}
			obj := toWorkloadObj(k.kind, w)
			snap.workloads[obj.ref] = obj
		}
	}

	pods, err := listItems(client, gvrPods)
	if err != nil {
		return nil, err
	}
	podsByNS := make(map[string][]podInfo)
	for _, it := range pods {
		var p podItem
		if decode(it, &p) != nil {
			continue
		}
		podsByNS[p.Metadata.Namespace] = append(podsByNS[p.Metadata.Namespace], toPodInfo(p, now))
	}
	for _, w := range snap.workloads {
		if w.selector == nil {
			continue
		}
		for _, p := range podsByNS[w.ref.Namespace] {
			if w.selector.Matches(labels.Set(p.labels)) {
				w.pods = append(w.pods, p)
			}
		}
	}

	if technology != "argocd" {
		if items, err := listItems(client, gvrHelmReleases); err == nil {
			for _, it := range items {
				var hr k8sItem
				if decode(it, &hr) != nil {
					continue
				}
				st := statusFromConditions(hr.Status.Conditions)
				ref := objRef{Group: gvrHelmReleases.Group, Kind: "HelmRelease", Namespace: hr.Metadata.Namespace, Name: hr.Metadata.Name}
				snap.helmReleases[ref] = graph.HelmReleaseStatus{
					Namespace: ref.Namespace, Name: ref.Name, Status: st.Status, Message: st.Message,
				}
			}
		}
	}
	return snap, nil
}

func toWorkloadObj(kind string, w workloadItem) *workloadObj {
	obj := &workloadObj{
		ref:    objRef{Group: "apps", Kind: kind, Namespace: w.Metadata.Namespace, Name: w.Metadata.Name},
		labels: w.Metadata.Labels,
	}
	switch kind {
	case "DaemonSet":
		obj.desired, obj.ready = w.Status.DesiredNumberScheduled, w.Status.NumberAvailable
	default:
		obj.desired = 1
		if w.Spec.Replicas != nil {
			obj.desired = *w.Spec.Replicas
		}
		obj.ready = w.Status.AvailableReplicas
		if kind == "StatefulSet" && obj.ready == 0 {
			obj.ready = w.Status.ReadyReplicas // availableReplicas is newer
		}
	}
	// An empty selector would match every pod in the namespace.
	if w.Spec.Selector != nil && (len(w.Spec.Selector.MatchLabels) > 0 || len(w.Spec.Selector.MatchExpressions) > 0) {
		if sel, err := metav1.LabelSelectorAsSelector(w.Spec.Selector); err == nil {
			obj.selector = sel
		}
	}
	return obj
}

func toPodInfo(p podItem, now time.Time) podInfo {
	info := podInfo{name: p.Metadata.Name, labels: p.Metadata.Labels}
	seen := map[string]bool{}
	add := func(issue string) {
		if !seen[issue] {
			seen[issue] = true
			info.issues = append(info.issues, issue)
		}
	}
	for _, cs := range p.Status.InitStatus {
		info.restarts += cs.RestartCount
		if w := cs.State.Waiting; w != nil && podIssueReasons[w.Reason] {
			add(w.Reason)
		}
	}
	for _, cs := range p.Status.Containers {
		info.restarts += cs.RestartCount
		switch w, t := cs.State.Waiting, cs.LastState.Terminated; {
		case w != nil && podIssueReasons[w.Reason]:
			add(w.Reason)
		case !cs.Ready && cs.RestartCount > 0 && t != nil:
			// Between two crashes the container is briefly running again;
			// a crash loop is only "waiting" during the back-off.
			add(fmt.Sprintf("restarting (last exit: %s, code %d)", t.Reason, t.ExitCode))
		}
	}
	if p.Status.Phase == "Pending" && len(info.issues) == 0 && now.Sub(p.Metadata.CreationTimestamp) > pendingGrace {
		reason := "Pending"
		for _, c := range p.Status.Conditions {
			if c.Type == "PodScheduled" && c.Status == "False" {
				reason = "Pending (unschedulable)"
			}
		}
		info.issues = append(info.issues, reason)
	}
	return info
}

// ── evaluation ───────────────────────────────────────────────────────────────

func isWorkloadKind(r objRef) bool {
	return r.Group == "apps" && (r.Kind == "Deployment" || r.Kind == "StatefulSet" || r.Kind == "DaemonSet")
}

// evalWorkload rates one workload. prev holds the restart totals of the
// previous refresh; the returned total is stored for the next one.
func evalWorkload(w *workloadObj, prev map[string]int32) (graph.Workload, int32) {
	out := graph.Workload{Kind: w.ref.Kind, Namespace: w.ref.Namespace, Name: w.ref.Name, Ready: w.ready, Desired: w.desired}
	var restarts int32
	for _, p := range w.pods {
		restarts += p.restarts
		for _, is := range p.issues {
			out.Issues = append(out.Issues, p.name+": "+is)
		}
	}
	sort.Strings(out.Issues)
	if before, ok := prev[w.ref.key()]; ok && restarts > before {
		out.Restarts = restarts - before
	}

	switch {
	case w.desired == 0:
		out.Status = graph.WorkloadHealthy // scaled to zero on purpose
	case w.ready == 0:
		out.Status = graph.WorkloadDown
	case w.ready < w.desired || len(out.Issues) > 0:
		out.Status = graph.WorkloadDegraded
	default:
		out.Status = graph.WorkloadHealthy
	}
	return out, restarts
}

// nodeHealth evaluates the objects behind one node. refs are the node's
// managed objects (Flux inventory or Argo CD resources); HelmReleases among
// them pull in the workloads labelled with their name.
func (s *workloadSnapshot) nodeHealth(refs []objRef, gitops string, prev, cur map[string]int32) *graph.WorkloadHealth {
	wl := make(map[objRef]*workloadObj)
	var hrs []graph.HelmReleaseStatus
	for _, r := range refs {
		switch {
		case isWorkloadKind(r):
			if w, ok := s.workloads[r]; ok {
				wl[r] = w
			}
		case r.Kind == "HelmRelease" && r.Group == gvrHelmReleases.Group:
			hr, ok := s.helmReleases[r]
			if !ok {
				hr = graph.HelmReleaseStatus{Namespace: r.Namespace, Name: r.Name, Status: "unknown"}
			}
			hrs = append(hrs, hr)
			for ref, w := range s.workloads {
				if w.labels[labelHelmName] == r.Name && w.labels[labelHelmNamespace] == r.Namespace {
					wl[ref] = w
				}
			}
		}
	}

	h := &graph.WorkloadHealth{Status: graph.WorkloadNA, Total: len(wl), HelmReleases: hrs}
	down := 0
	for _, w := range wl {
		ev, total := evalWorkload(w, prev)
		cur[w.ref.key()] = total
		h.Workloads = append(h.Workloads, ev)
		switch ev.Status {
		case graph.WorkloadHealthy:
			h.Ready++
		case graph.WorkloadDown:
			down++
		}
	}
	sort.Slice(h.Workloads, func(i, j int) bool {
		a, b := h.Workloads[i], h.Workloads[j]
		return a.Namespace+"/"+a.Kind+"/"+a.Name < b.Namespace+"/"+b.Kind+"/"+b.Name
	})
	switch {
	case h.Total == 0:
	case h.Ready == h.Total:
		h.Status = graph.WorkloadHealthy
	case down == h.Total:
		h.Status = graph.WorkloadDown
	default:
		h.Status = graph.WorkloadDegraded
	}

	hrNotReady := false
	for _, hr := range hrs {
		if hr.Status != "ready" {
			hrNotReady = true
		}
	}
	h.Drift = gitops == "ready" && (h.Status == graph.WorkloadDegraded || h.Status == graph.WorkloadDown || hrNotReady)
	return h
}

// ── membership ───────────────────────────────────────────────────────────────

type inventoryItem struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Status struct {
		Inventory struct {
			Entries []struct {
				ID string `json:"id"`
			} `json:"entries"`
		} `json:"inventory"`
	} `json:"status"`
}

// inventoryRefs maps object name → objects in its Flux inventory. Objects
// with the same name in different namespaces are merged, like their status.
func inventoryRefs(items []unstructured.Unstructured) map[string][]objRef {
	out := make(map[string][]objRef)
	for _, it := range items {
		var inv inventoryItem
		if decode(it, &inv) != nil {
			continue
		}
		refs := out[inv.Metadata.Name]
		if refs == nil {
			refs = []objRef{} // present, even with an empty inventory
		}
		for _, e := range inv.Status.Inventory.Entries {
			if r, ok := parseInventoryID(e.ID); ok {
				refs = append(refs, r)
			}
		}
		out[inv.Metadata.Name] = refs
	}
	return out
}

type argoResourcesItem struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Status struct {
		Resources []struct {
			Group     string `json:"group"`
			Kind      string `json:"kind"`
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
		} `json:"resources"`
	} `json:"status"`
}

// argoRefs maps Application name → its managed resources.
func argoRefs(items []unstructured.Unstructured) map[string][]objRef {
	out := make(map[string][]objRef)
	for _, it := range items {
		var app argoResourcesItem
		if decode(it, &app) != nil {
			continue
		}
		refs := []objRef{}
		for _, r := range app.Status.Resources {
			refs = append(refs, objRef{Group: r.Group, Kind: r.Kind, Namespace: r.Namespace, Name: r.Name})
		}
		out[app.Metadata.Name] = refs
	}
	return out
}

// applyWorkloads sets Node.Workloads for every node with known membership and
// clears it on all others. It returns the restart totals for the next run.
func applyWorkloads(profile *graph.ClusterProfile, snap *workloadSnapshot, members map[string][]objRef, prev map[string]int32) map[string]int32 {
	cur := make(map[string]int32)
	for i := range profile.Graph.Nodes {
		n := &profile.Graph.Nodes[i]
		refs, ok := members[n.ID]
		if !ok {
			n.Workloads = nil
			continue
		}
		n.Workloads = snap.nodeHealth(refs, n.Status, prev, cur)
	}
	return cur
}

func clearWorkloads(profile *graph.ClusterProfile) {
	for i := range profile.Graph.Nodes {
		profile.Graph.Nodes[i].Workloads = nil
	}
}
