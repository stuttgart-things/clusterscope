package live

import (
	"encoding/json"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

func gatewayU(ns, name string, listeners []any, programmed string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "Gateway",
		"metadata":   map[string]any{"name": name, "namespace": ns},
		"spec":       map[string]any{"gatewayClassName": "cilium", "listeners": listeners},
		"status": map[string]any{
			"addresses":  []any{map[string]any{"value": "10.31.103.10"}},
			"conditions": []any{map[string]any{"type": "Programmed", "status": programmed}, map[string]any{"type": "Accepted", "status": "True"}},
		},
	}}
}

func listener(name, protocol, hostname string, port int64) map[string]any {
	l := map[string]any{"name": name, "protocol": protocol, "port": port}
	if hostname != "" {
		l["hostname"] = hostname
	}
	return l
}

func routeU(ns, name string, labels map[string]any, spec map[string]any, parents []any) *unstructured.Unstructured {
	meta := map[string]any{"name": name, "namespace": ns}
	if labels != nil {
		meta["labels"] = labels
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "gateway.networking.k8s.io/v1",
		"kind":       "HTTPRoute",
		"metadata":   meta,
		"spec":       spec,
		"status":     map[string]any{"parents": parents},
	}}
}

func parentStatus(gwNS, gw, section, accepted, resolved, msg string) map[string]any {
	ref := map[string]any{"name": gw, "namespace": gwNS}
	if section != "" {
		ref["sectionName"] = section
	}
	return map[string]any{
		"parentRef": ref,
		"conditions": []any{
			map[string]any{"type": "Accepted", "status": accepted, "message": msg},
			map[string]any{"type": "ResolvedRefs", "status": resolved, "message": msg},
		},
	}
}

func TestFetchEndpoints(t *testing.T) {
	gw := gatewayU("default", "lab-gw", []any{
		listener("http", "HTTP", "", 80),
		listener("https", "HTTPS", "*.sthings.lab", 443),
	}, "True")

	ok := routeU("vault", "vault",
		map[string]any{"kustomize.toolkit.fluxcd.io/name": "vault", "kustomize.toolkit.fluxcd.io/namespace": "flux-system"},
		map[string]any{
			"parentRefs": []any{map[string]any{"name": "lab-gw", "namespace": "default"}},
			"hostnames":  []any{"vault.sthings.lab", "*.vault.sthings.lab"},
			"rules": []any{map[string]any{
				"matches":     []any{map[string]any{"path": map[string]any{"type": "PathPrefix", "value": "/"}}},
				"backendRefs": []any{map[string]any{"name": "vault", "port": int64(8200)}},
			}},
		},
		[]any{parentStatus("default", "lab-gw", "", "True", "True", "")})

	broken := routeU("demo", "api",
		map[string]any{"helm.toolkit.fluxcd.io/name": "demo-api", "kustomize.toolkit.fluxcd.io/name": "apps"},
		map[string]any{
			"parentRefs": []any{map[string]any{"name": "lab-gw", "namespace": "default", "sectionName": "http"}},
			"hostnames":  []any{"demo.other.lab"},
			"rules": []any{map[string]any{
				"matches":     []any{map[string]any{"path": map[string]any{"type": "PathPrefix", "value": "/api"}}},
				"backendRefs": []any{map[string]any{"name": "api", "namespace": "backend", "port": int64(8080)}},
			}},
		},
		[]any{parentStatus("default", "lab-gw", "http", "True", "False", `Service "backend/api" not found`)})

	unattached := routeU("demo", "orphan", nil,
		map[string]any{
			"parentRefs": []any{map[string]any{"name": "missing-gw"}},
			"hostnames":  []any{"orphan.sthings.lab"},
		}, nil)

	// Objects are added with an explicit GVR: the fake tracker would guess
	// "gatewaies" from kind Gateway.
	client := dynfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvrGateways: "GatewayList", gvrHTTPRoutes: "HTTPRouteList"})
	if err := client.Tracker().Create(gvrGateways, gw, "default"); err != nil {
		t.Fatal(err)
	}
	for _, r := range []*unstructured.Unstructured{ok, broken, unattached} {
		if err := client.Tracker().Create(gvrHTTPRoutes, r, r.GetNamespace()); err != nil {
			t.Fatal(err)
		}
	}

	ep := fetchEndpoints(client, "flux")
	if !ep.GatewayAPI || ep.Error != "" {
		t.Fatalf("unexpected endpoints state: %+v", ep)
	}
	if len(ep.Gateways) != 1 || ep.Gateways[0].Programmed != "True" || len(ep.Gateways[0].Listeners) != 2 {
		t.Errorf("gateway: %+v", ep.Gateways)
	}
	if len(ep.Routes) != 3 {
		t.Fatalf("routes: got %d, want 3", len(ep.Routes))
	}
	byName := map[string]Route{}
	for _, r := range ep.Routes {
		byName[r.Namespace+"/"+r.Name] = r
	}

	v := byName["vault/vault"]
	if v.Accepted != "True" || v.ResolvedRefs != "True" {
		t.Errorf("vault conditions: %+v", v)
	}
	if v.App != "vault" {
		t.Errorf("vault app: %q", v.App)
	}
	// wildcard hostname is not probe-able; HTTPS listener matches → https
	if len(v.URLs) != 1 || v.URLs[0] != "https://vault.sthings.lab" {
		t.Errorf("vault urls: %v", v.URLs)
	}
	if len(v.Backends) != 1 || v.Backends[0] != "vault:8200" {
		t.Errorf("vault backends: %v", v.Backends)
	}

	a := byName["demo/api"]
	if a.Accepted != "True" || a.ResolvedRefs != "False" {
		t.Errorf("api conditions: accepted=%s resolved=%s", a.Accepted, a.ResolvedRefs)
	}
	if a.Parents[0].Gateway != "default/lab-gw/http" || a.Parents[0].Message == "" {
		t.Errorf("api parent: %+v", a.Parents[0])
	}
	if a.App != "demo-api" {
		t.Errorf("helm label should win: %q", a.App)
	}
	// sectionName "http" → plain HTTP listener, path prefix appended
	if len(a.URLs) != 1 || a.URLs[0] != "http://demo.other.lab/api" {
		t.Errorf("api urls: %v", a.URLs)
	}
	if a.Backends[0] != "backend/api:8080" {
		t.Errorf("cross-namespace backend: %v", a.Backends)
	}

	o := byName["demo/orphan"]
	if o.Accepted != "Unknown" || o.Parents[0].Gateway != "demo/missing-gw" {
		t.Errorf("orphan: %+v", o)
	}
}

func TestFetchEndpoints_NoGatewayAPI(t *testing.T) {
	client := dynfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvrGateways: "GatewayList", gvrHTTPRoutes: "HTTPRouteList"})
	client.PrependReactor("list", "gateways", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(gvrGateways.GroupResource(), "")
	})

	ep := fetchEndpoints(client, "flux")
	if ep.GatewayAPI || ep.Error != "" || len(ep.Routes) != 0 {
		t.Errorf("expected GatewayAPI=false without error, got %+v", ep)
	}
}

func TestOwningApp_Argo(t *testing.T) {
	m := k8sMeta{Annotations: map[string]string{"argocd.argoproj.io/tracking-id": "schmetterpause:gateway.networking.k8s.io/HTTPRoute:sp/web"}}
	if got := owningApp(m, "argocd"); got != "schmetterpause" {
		t.Errorf("tracking-id: %q", got)
	}
	m = k8sMeta{Labels: map[string]string{"app.kubernetes.io/instance": "kargo"}}
	if got := owningApp(m, "argocd"); got != "kargo" {
		t.Errorf("argo instance label: %q", got)
	}
	if got := owningApp(m, "flux"); got != "" {
		t.Errorf("instance label must be ignored on flux clusters: %q", got)
	}
}

func TestHostMatches(t *testing.T) {
	tests := []struct {
		pattern, host string
		want          bool
	}{
		{"", "a.lab", true},
		{"*.lab", "a.lab", true},
		{"*.lab", "a.b.lab", true},
		{"*.lab", "lab", false},
		{"a.lab", "a.lab", true},
		{"a.lab", "b.lab", false},
	}
	for _, tt := range tests {
		if got := hostMatches(tt.pattern, tt.host); got != tt.want {
			t.Errorf("hostMatches(%q, %q) = %v", tt.pattern, tt.host, got)
		}
	}
}

func TestToRoute_ProbesRootWhenMatched(t *testing.T) {
	// Harbor-style route: specific API paths first, "/" last.
	var r httpRouteObj
	raw := `{"metadata":{"name":"harbor","namespace":"harbor"},"spec":{"hostnames":["harbor.lab"],"rules":[
		{"matches":[{"path":{"type":"PathPrefix","value":"/api/"}},{"path":{"type":"PathPrefix","value":"/v2/"}}]},
		{"matches":[{"path":{"type":"PathPrefix","value":"/"}}]}]}}`
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		t.Fatal(err)
	}
	rt := toRoute(r, nil, "flux")
	if len(rt.URLs) != 1 || rt.URLs[0] != "https://harbor.lab" {
		t.Errorf("urls: %v, want root", rt.URLs)
	}
}

func TestToRoute_ProbeAnnotation(t *testing.T) {
	for _, tt := range []struct {
		annotations string
		want        bool
	}{
		{``, false},
		{`,"annotations":{"clusterscope.stuttgart-things.com/probe":"false"}`, true},
		{`,"annotations":{"clusterscope.stuttgart-things.com/probe":" False "}`, true},
		{`,"annotations":{"clusterscope.stuttgart-things.com/probe":"true"}`, false},
	} {
		var r httpRouteObj
		raw := `{"metadata":{"name":"acme","namespace":"esp"` + tt.annotations + `},"spec":{"hostnames":["dev.lab"],"rules":[
			{"matches":[{"path":{"type":"PathPrefix","value":"/.well-known/acme-challenge/"}}]}]}}`
		if err := json.Unmarshal([]byte(raw), &r); err != nil {
			t.Fatal(err)
		}
		rt := toRoute(r, nil, "flux")
		if rt.NoProbe != tt.want {
			t.Errorf("%s: NoProbe = %v, want %v", tt.annotations, rt.NoProbe, tt.want)
		}
		// The URL stays listed either way.
		if len(rt.URLs) != 1 {
			t.Errorf("%s: urls %v", tt.annotations, rt.URLs)
		}
	}
}
