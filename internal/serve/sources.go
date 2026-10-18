package serve

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/stuttgart-things/clusterscope/internal/clusterbook"
	"github.com/stuttgart-things/clusterscope/internal/machinery"
	"github.com/stuttgart-things/clusterscope/internal/scan"
)

// Further cluster sources for the lab view (#60): a source that fails shows
// up as a lab-level warning and never blocks the view.

// SourceRefresh is how often the clusterbooks are re-read.
const SourceRefresh = 5 * time.Minute

// bookState is the last result of one clusterbook. The fields after spec
// are guarded by Server.mu.
type bookState struct {
	spec    scan.ClusterbookSpec
	client  *clusterbook.Client
	records []clusterbook.Record
	err     string
	fetched time.Time
}

// machineryState is the last result of one machinery cluster. The fields
// after list are guarded by Server.mu.
type machineryState struct {
	spec    scan.MachinerySpec
	list    func(ctx context.Context) ([]machinery.Record, error)
	records []machinery.Record
	err     string
	fetched time.Time
}

// SourceStatus is one cluster source as shown in the lab view.
type SourceStatus struct {
	Kind      string `json:"kind"` // clusterbook, machinery
	URL       string `json:"url"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	Records   int    `json:"records"`
	FetchedAt string `json:"fetchedAt,omitempty"`
}

// startSources reads every clusterbook once in the background and then
// every SourceRefresh.
func (s *Server) startSources() {
	for _, spec := range s.cfg.Clusterbooks {
		s.books = append(s.books, &bookState{spec: spec, client: clusterbook.New(spec.URL, 15*time.Second)})
	}
	for _, spec := range s.cfg.Machineries {
		s.machineries = append(s.machineries, &machineryState{spec: spec, list: s.machineryLister(spec.Cluster)})
	}
	if len(s.books) == 0 && len(s.machineries) == 0 {
		return
	}
	go func() {
		for {
			s.refreshSources()
			time.Sleep(SourceRefresh)
		}
	}()
}

func (s *Server) refreshSources() {
	for _, b := range s.books {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		recs, err := b.client.List(ctx)
		cancel()
		s.mu.Lock()
		b.fetched = time.Now()
		if err != nil {
			// Keep the last records: a short outage should not turn every
			// cluster into "no allocation".
			b.err = err.Error()
			log.Printf("clusterbook %s: %v", b.spec.URL, err)
		} else {
			b.err = ""
			b.records = recs
		}
		s.mu.Unlock()
	}
	for _, m := range s.machineries {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		recs, err := m.list(ctx)
		cancel()
		s.mu.Lock()
		m.fetched = time.Now()
		if err != nil {
			// Keep the last orders, for the clusterbooks' reason.
			m.err = err.Error()
			log.Printf("machinery %s: %v", m.spec.Cluster, err)
		} else {
			m.err = ""
			m.records = recs
		}
		s.mu.Unlock()
	}
	s.updateLabMetrics()
}

// machineryLister reads the orders through the live clients: the machinery
// cluster's kubeconfig is one of -kubeconfig-dir / -kubeconfig-map.
func (s *Server) machineryLister(cluster string) func(ctx context.Context) ([]machinery.Record, error) {
	return func(ctx context.Context) ([]machinery.Record, error) {
		if s.clients == nil {
			return nil, fmt.Errorf("needs live mode (-live) to read cluster %s", cluster)
		}
		client, err := s.clients.Dynamic(cluster)
		if err != nil {
			return nil, err
		}
		return machinery.List(ctx, client)
	}
}

// applyMachineries matches the ClusterStack orders of every machinery against
// the clusters of its lab, by name. A matched cluster gets its order; a
// cluster that only had a kubeconfig is then not "unmanaged" any more -- the
// order IS its desired state. An order without a cluster becomes a finding:
// a warning once it is ready (the kubeconfig never arrived), info while it
// is still being built. The caller holds s.mu.
func (s *Server) applyMachineries(labOf func(string) *Lab) []SourceStatus {
	out := []SourceStatus{}
	for _, m := range s.machineries {
		st := SourceStatus{Kind: "machinery", URL: m.spec.Cluster, OK: m.err == "" && !m.fetched.IsZero(), Error: m.err}
		if !m.fetched.IsZero() {
			st.FetchedAt = m.fetched.Format(time.RFC3339)
		}

		labName := m.spec.Lab
		if labName == "" {
			labName = s.unmanagedLab()
		}
		lab := labOf(labName)
		if m.err != "" {
			msg := "machinery not readable: " + m.err
			if m.records != nil {
				msg += " — showing the last result from " + m.fetched.Format(time.RFC3339)
			}
			lab.Findings = append(lab.Findings, Finding{Severity: SevWarning, Kind: "source-unavailable", Object: m.spec.Cluster, Message: msg})
		}
		envs := make(map[string]bool, len(m.spec.Environments))
		for _, e := range m.spec.Environments {
			envs[e] = true
		}

		idx := make(map[string]int, len(lab.Clusters))
		for i, c := range lab.Clusters {
			if !c.NoGitOps {
				idx[c.Name] = i
			}
		}
		ordered := make(map[string]bool)
		for _, rec := range m.records {
			if len(envs) > 0 && !envs[rec.Environment] {
				continue
			}
			st.Records++
			stage := rec.Stage
			if stage == "" {
				stage = "pending"
			}
			i, ok := idx[rec.Name]
			if !ok {
				f := Finding{Severity: SevInfo, Kind: "order-building", Cluster: rec.Name, Object: m.spec.Cluster,
					Message: fmt.Sprintf("ordered on %s, being built (stage %s)", m.spec.Cluster, stage)}
				switch {
				case rec.Deleting:
					f.Kind, f.Message = "order-deleting", fmt.Sprintf("being torn down on %s", m.spec.Cluster)
				case rec.Ready:
					f.Severity, f.Kind = SevWarning, "order-without-cluster"
					f.Message = fmt.Sprintf("ordered on %s and ready, but no kubeconfig and no cluster directory", m.spec.Cluster)
				}
				lab.Findings = append(lab.Findings, f)
				continue
			}
			c := &lab.Clusters[i]
			r := rec
			c.Order = &r
			c.OrderSource = m.spec.Cluster
			ordered[c.Name] = true
			if !rec.Synced {
				c.Findings++
				lab.Findings = append(lab.Findings, Finding{Severity: SevWarning, Kind: "order-not-synced", Cluster: c.Name, Object: m.spec.Cluster,
					Message: "ClusterStack not synced: " + rec.SyncMessage})
			}
			if !rec.Ready && !rec.Deleting {
				c.Findings++
				lab.Findings = append(lab.Findings, Finding{Severity: SevInfo, Kind: "order-not-ready", Cluster: c.Name, Object: m.spec.Cluster,
					Message: fmt.Sprintf("ClusterStack not ready (stage %s)", stage)})
			}
		}
		// The order is the desired state of a cluster without a directory.
		kept := lab.Findings[:0]
		for _, f := range lab.Findings {
			if f.Kind == "unmanaged-cluster" && ordered[f.Cluster] {
				if i, ok := idx[f.Cluster]; ok && lab.Clusters[i].Findings > 0 {
					lab.Clusters[i].Findings--
				}
				continue
			}
			kept = append(kept, f)
		}
		lab.Findings = kept
		out = append(out, st)
	}
	return out
}

// applyClusterbooks matches the allocations of every clusterbook against the
// clusters of its lab: by name mapping, by name, then by domain — a cluster
// whose route hostnames lie under the allocated FQDN. Matched clusters get
// their addresses and FQDN; allocations without a cluster and clusters
// without an allocation become findings. The caller holds s.mu.
func (s *Server) applyClusterbooks(labOf func(string) *Lab, routesByCluster map[string][]RouteRow) []SourceStatus {
	out := []SourceStatus{}
	for _, b := range s.books {
		st := SourceStatus{Kind: "clusterbook", URL: b.spec.URL, OK: b.err == "" && !b.fetched.IsZero(),
			Error: b.err, Records: len(b.records)}
		if !b.fetched.IsZero() {
			st.FetchedAt = b.fetched.Format(time.RFC3339)
		}
		out = append(out, st)

		labName := b.spec.Lab
		if labName == "" {
			labName = s.unmanagedLab()
		}
		lab := labOf(labName)
		host := b.spec.URL
		if u, err := url.Parse(b.spec.URL); err == nil && u.Host != "" {
			host = u.Host
		}
		if b.err != "" {
			msg := "clusterbook not readable: " + b.err
			if b.records != nil {
				msg += " — showing the last result from " + b.fetched.Format(time.RFC3339)
			}
			lab.Findings = append(lab.Findings, Finding{Severity: SevWarning, Kind: "source-unavailable", Object: host, Message: msg})
		}
		if b.records == nil {
			continue
		}

		idx := make(map[string]int, len(lab.Clusters))
		for i, c := range lab.Clusters {
			if !c.NoGitOps {
				idx[c.Name] = i
			}
		}
		matched := make(map[int]bool)
		for _, rec := range b.records {
			i, ok := idx[b.spec.Names[rec.Cluster]]
			if !ok {
				i, ok = idx[rec.Cluster]
			}
			if !ok {
				i, ok = matchDomain(rec.Domain(), idx, routesByCluster)
			}
			if !ok {
				what := strings.Join(rec.Addresses(), ", ")
				if rec.FQDN != "" {
					what += " · " + rec.FQDN
				}
				lab.Findings = append(lab.Findings, Finding{
					Severity: SevWarning, Kind: "orphan-allocation", Cluster: rec.Cluster, Object: what,
					Message: fmt.Sprintf("allocated in clusterbook %s, but no cluster of the lab has this name or routes under this domain", host),
				})
				continue
			}
			matched[i] = true
			c := &lab.Clusters[i]
			c.Addresses = append(c.Addresses, rec.Addresses()...)
			if rec.FQDN != "" && c.FQDN == "" {
				c.FQDN = rec.FQDN
			}
			if rec.Cluster != c.Name {
				c.ClusterbookName = rec.Cluster
			}
		}
		for i := range lab.Clusters {
			c := &lab.Clusters[i]
			if c.NoGitOps || matched[i] {
				continue
			}
			c.Findings++
			lab.Findings = append(lab.Findings, Finding{
				Severity: SevInfo, Kind: "no-allocation", Cluster: c.Name,
				Message: fmt.Sprintf("no IP / DNS allocation in clusterbook %s", host),
			})
		}
	}
	return out
}

// matchDomain finds the one cluster with a route hostname under domain.
// Several candidates are no match: the allocation is then ambiguous.
func matchDomain(domain string, idx map[string]int, routesByCluster map[string][]RouteRow) (int, bool) {
	if domain == "" {
		return 0, false
	}
	var hits []int
	for name, i := range idx {
		if clusterUnderDomain(routesByCluster[name], domain) {
			hits = append(hits, i)
		}
	}
	sort.Ints(hits)
	if len(hits) != 1 {
		return 0, false
	}
	return hits[0], true
}

func clusterUnderDomain(routes []RouteRow, domain string) bool {
	for _, r := range routes {
		for _, h := range r.Hostnames {
			h = strings.TrimPrefix(strings.ToLower(h), "*.")
			if h == domain || strings.HasSuffix(h, "."+domain) {
				return true
			}
		}
	}
	return false
}
