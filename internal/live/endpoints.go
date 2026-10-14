package live

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var (
	gvrGateways   = schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways"}
	gvrHTTPRoutes = schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "httproutes"}
)

// Endpoints is the Gateway API inventory of one cluster.
type Endpoints struct {
	// GatewayAPI is false when the Gateway API CRDs are not installed.
	GatewayAPI bool      `json:"gatewayAPI"`
	Gateways   []Gateway `json:"gateways"`
	Routes     []Route   `json:"routes"`
	Error      string    `json:"error,omitempty"`
}

// Gateway is a Gateway API Gateway with its listeners and status.
type Gateway struct {
	Namespace  string     `json:"namespace"`
	Name       string     `json:"name"`
	Class      string     `json:"class"`
	Listeners  []Listener `json:"listeners"`
	Addresses  []string   `json:"addresses,omitempty"`
	Accepted   string     `json:"accepted"`   // condition status: True | False | Unknown
	Programmed string     `json:"programmed"` // condition status: True | False | Unknown
	Message    string     `json:"message,omitempty"`
}

// Listener is one Gateway listener.
type Listener struct {
	Name     string `json:"name"`
	Hostname string `json:"hostname,omitempty"`
	Protocol string `json:"protocol"`
	Port     int64  `json:"port"`
}

// Route is an HTTPRoute with the information needed to answer "which URL
// exists, where does it go, and is it attached?".
type Route struct {
	Namespace string        `json:"namespace"`
	Name      string        `json:"name"`
	Hostnames []string      `json:"hostnames"`
	Paths     []string      `json:"paths,omitempty"`
	Parents   []RouteParent `json:"parents"`
	Backends  []string      `json:"backends,omitempty"`
	// URLs are the probe-able URLs (no wildcard hostnames), scheme taken
	// from the parent Gateway's listener.
	URLs []string `json:"urls,omitempty"`
	// App is the owning GitOps unit, from Flux / Argo CD labels.
	App string `json:"app,omitempty"`
	// Accepted / ResolvedRefs aggregate all parents: "True" only if every
	// parent reports True, "False" if any reports False, else "Unknown".
	Accepted     string `json:"accepted"`
	ResolvedRefs string `json:"resolvedRefs"`
}

// RouteParent is one parentRef of a route with its status conditions.
type RouteParent struct {
	Gateway      string `json:"gateway"` // namespace/name[/section]
	Accepted     string `json:"accepted"`
	ResolvedRefs string `json:"resolvedRefs"`
	Message      string `json:"message,omitempty"`
}

// ── k8s shapes ───────────────────────────────────────────────────────────────

type k8sMeta struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
}

type gatewayObj struct {
	Metadata k8sMeta `json:"metadata"`
	Spec     struct {
		GatewayClassName string `json:"gatewayClassName"`
		Listeners        []struct {
			Name     string `json:"name"`
			Hostname string `json:"hostname"`
			Protocol string `json:"protocol"`
			Port     int64  `json:"port"`
		} `json:"listeners"`
	} `json:"spec"`
	Status struct {
		Addresses []struct {
			Value string `json:"value"`
		} `json:"addresses"`
		Conditions []k8sCondition `json:"conditions"`
	} `json:"status"`
}

type parentRef struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	SectionName string `json:"sectionName"`
}

type httpRouteObj struct {
	Metadata k8sMeta `json:"metadata"`
	Spec     struct {
		ParentRefs []parentRef `json:"parentRefs"`
		Hostnames  []string    `json:"hostnames"`
		Rules      []struct {
			Matches []struct {
				Path *struct {
					Type  string `json:"type"`
					Value string `json:"value"`
				} `json:"path"`
			} `json:"matches"`
			BackendRefs []struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
				Kind      string `json:"kind"`
				Port      *int64 `json:"port"`
			} `json:"backendRefs"`
		} `json:"rules"`
	} `json:"spec"`
	Status struct {
		Parents []struct {
			ParentRef  parentRef      `json:"parentRef"`
			Conditions []k8sCondition `json:"conditions"`
		} `json:"parents"`
	} `json:"status"`
}

// ── collection ───────────────────────────────────────────────────────────────

func fetchEndpoints(client dynamic.Interface, technology string) Endpoints {
	ep := Endpoints{GatewayAPI: true, Gateways: []Gateway{}, Routes: []Route{}}

	gwList, err := client.Resource(gvrGateways).Namespace("").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			ep.GatewayAPI = false
			return ep
		}
		ep.Error = fmt.Sprintf("list gateways: %v", err)
		return ep
	}
	gateways := make(map[string]gatewayObj)
	for _, item := range gwList.Items {
		raw, _ := item.MarshalJSON()
		var g gatewayObj
		if json.Unmarshal(raw, &g) != nil {
			continue
		}
		gateways[g.Metadata.Namespace+"/"+g.Metadata.Name] = g
		ep.Gateways = append(ep.Gateways, toGateway(g))
	}

	rtList, err := client.Resource(gvrHTTPRoutes).Namespace("").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ep
		}
		ep.Error = fmt.Sprintf("list httproutes: %v", err)
		return ep
	}
	for _, item := range rtList.Items {
		raw, _ := item.MarshalJSON()
		var r httpRouteObj
		if json.Unmarshal(raw, &r) != nil {
			continue
		}
		ep.Routes = append(ep.Routes, toRoute(r, gateways, technology))
	}

	sort.Slice(ep.Gateways, func(i, j int) bool {
		return ep.Gateways[i].Namespace+"/"+ep.Gateways[i].Name < ep.Gateways[j].Namespace+"/"+ep.Gateways[j].Name
	})
	sort.Slice(ep.Routes, func(i, j int) bool {
		return ep.Routes[i].Namespace+"/"+ep.Routes[i].Name < ep.Routes[j].Namespace+"/"+ep.Routes[j].Name
	})
	return ep
}

func toGateway(g gatewayObj) Gateway {
	gw := Gateway{
		Namespace:  g.Metadata.Namespace,
		Name:       g.Metadata.Name,
		Class:      g.Spec.GatewayClassName,
		Accepted:   conditionStatus(g.Status.Conditions, "Accepted"),
		Programmed: conditionStatus(g.Status.Conditions, "Programmed"),
		Message:    conditionMessage(g.Status.Conditions, "Programmed"),
	}
	for _, l := range g.Spec.Listeners {
		gw.Listeners = append(gw.Listeners, Listener{Name: l.Name, Hostname: l.Hostname, Protocol: l.Protocol, Port: l.Port})
	}
	for _, a := range g.Status.Addresses {
		gw.Addresses = append(gw.Addresses, a.Value)
	}
	return gw
}

func toRoute(r httpRouteObj, gateways map[string]gatewayObj, technology string) Route {
	rt := Route{
		Namespace: r.Metadata.Namespace,
		Name:      r.Metadata.Name,
		Hostnames: r.Spec.Hostnames,
		App:       owningApp(r.Metadata, technology),
	}
	if rt.Hostnames == nil {
		rt.Hostnames = []string{}
	}

	for _, rule := range r.Spec.Rules {
		for _, m := range rule.Matches {
			if m.Path != nil && m.Path.Value != "" && m.Path.Type != "RegularExpression" {
				rt.Paths = appendUnique(rt.Paths, m.Path.Value)
			}
		}
		for _, b := range rule.BackendRefs {
			ref := b.Name
			if b.Kind != "" && b.Kind != "Service" {
				ref = b.Kind + "/" + ref
			}
			if b.Namespace != "" && b.Namespace != rt.Namespace {
				ref = b.Namespace + "/" + ref
			}
			if b.Port != nil {
				ref = fmt.Sprintf("%s:%d", ref, *b.Port)
			}
			rt.Backends = appendUnique(rt.Backends, ref)
		}
	}

	// Parents: spec order, status matched by gateway reference.
	for _, pr := range r.Spec.ParentRefs {
		ns := pr.Namespace
		if ns == "" {
			ns = rt.Namespace
		}
		p := RouteParent{Gateway: parentKey(ns, pr.Name, pr.SectionName), Accepted: "Unknown", ResolvedRefs: "Unknown"}
		for _, sp := range r.Status.Parents {
			sns := sp.ParentRef.Namespace
			if sns == "" {
				sns = rt.Namespace
			}
			if sns == ns && sp.ParentRef.Name == pr.Name && sp.ParentRef.SectionName == pr.SectionName {
				p.Accepted = conditionStatus(sp.Conditions, "Accepted")
				p.ResolvedRefs = conditionStatus(sp.Conditions, "ResolvedRefs")
				for _, t := range []string{"Accepted", "ResolvedRefs"} {
					if conditionStatus(sp.Conditions, t) == "False" {
						p.Message = conditionMessage(sp.Conditions, t)
						break
					}
				}
			}
		}
		rt.Parents = append(rt.Parents, p)
	}
	rt.Accepted = aggregate(rt.Parents, func(p RouteParent) string { return p.Accepted })
	rt.ResolvedRefs = aggregate(rt.Parents, func(p RouteParent) string { return p.ResolvedRefs })

	// URLs: one per concrete hostname. The root is probed when any rule
	// matches "/" (or has no path match); otherwise the first path.
	path := ""
	if len(rt.Paths) > 0 && !contains(rt.Paths, "/") {
		path = rt.Paths[0]
	}
	for _, h := range rt.Hostnames {
		if strings.Contains(h, "*") {
			continue
		}
		rt.URLs = append(rt.URLs, schemeFor(h, rt.Namespace, r.Spec.ParentRefs, gateways)+"://"+h+path)
	}
	return rt
}

// owningApp derives the GitOps unit that manages an object from its labels.
func owningApp(m k8sMeta, technology string) string {
	if n := m.Labels["helm.toolkit.fluxcd.io/name"]; n != "" {
		return n
	}
	if id := m.Annotations["argocd.argoproj.io/tracking-id"]; id != "" {
		if i := strings.Index(id, ":"); i > 0 {
			return id[:i]
		}
	}
	if n := m.Labels["kustomize.toolkit.fluxcd.io/name"]; n != "" {
		return n
	}
	// Argo CD's default label tracking; on Flux clusters this label usually
	// is the Helm release name, which helm.toolkit.fluxcd.io/name covers.
	if technology == "argocd" {
		return m.Labels["app.kubernetes.io/instance"]
	}
	return ""
}

// schemeFor picks https when a matching HTTPS listener exists on one of the
// route's parent gateways, http when only HTTP listeners match, https if
// nothing is known.
func schemeFor(host, routeNS string, parents []parentRef, gateways map[string]gatewayObj) string {
	hasHTTP := false
	for _, pr := range parents {
		ns := pr.Namespace
		if ns == "" {
			ns = routeNS
		}
		g, ok := gateways[ns+"/"+pr.Name]
		if !ok {
			continue
		}
		for _, l := range g.Spec.Listeners {
			if pr.SectionName != "" && l.Name != pr.SectionName {
				continue
			}
			if !hostMatches(l.Hostname, host) {
				continue
			}
			switch l.Protocol {
			case "HTTPS":
				return "https"
			case "HTTP":
				hasHTTP = true
			}
		}
	}
	if hasHTTP {
		return "http"
	}
	return "https"
}

func hostMatches(pattern, host string) bool {
	switch {
	case pattern == "":
		return true
	case strings.HasPrefix(pattern, "*."):
		return strings.HasSuffix(host, pattern[1:])
	default:
		return pattern == host
	}
}

func parentKey(ns, name, section string) string {
	k := ns + "/" + name
	if section != "" {
		k += "/" + section
	}
	return k
}

func aggregate(parents []RouteParent, get func(RouteParent) string) string {
	if len(parents) == 0 {
		return "Unknown"
	}
	all := true
	for _, p := range parents {
		switch get(p) {
		case "False":
			return "False"
		case "True":
		default:
			all = false
		}
	}
	if all {
		return "True"
	}
	return "Unknown"
}

func conditionStatus(conds []k8sCondition, t string) string {
	for _, c := range conds {
		if c.Type == t {
			return c.Status
		}
	}
	return "Unknown"
}

func conditionMessage(conds []k8sCondition, t string) string {
	for _, c := range conds {
		if c.Type == t {
			return c.Message
		}
	}
	return ""
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func appendUnique(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}
