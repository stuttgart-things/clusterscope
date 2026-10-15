// Package clusterbook reads the IP / DNS allocations per cluster from a
// clusterbook REST API (stuttgart-things/clusterbook), as a cluster source for
// the lab view (#60).
package clusterbook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// IP is one allocated address.
type IP struct {
	Network string `json:"network"`
	IP      string `json:"ip"`
	Status  string `json:"status"` // e.g. ASSIGNED, ASSIGNED:DNS
}

// Record is the allocation of one cluster name.
type Record struct {
	Cluster string `json:"cluster"`
	FQDN    string `json:"fqdn,omitempty"` // e.g. *.infra.sthings-vsphere.labul.sva.de
	Zone    string `json:"zone,omitempty"`
	IPs     []IP   `json:"ips"`
}

// Domain is the FQDN without its wildcard label: the domain the cluster's
// routes live under ("" without DNS).
func (r Record) Domain() string {
	return strings.TrimPrefix(strings.TrimSuffix(r.FQDN, "."), "*.")
}

// Addresses lists the allocated IPs.
func (r Record) Addresses() []string {
	out := make([]string, 0, len(r.IPs))
	for _, ip := range r.IPs {
		out = append(out, ip.IP)
	}
	return out
}

// Client talks to one clusterbook.
type Client struct {
	base string
	http *http.Client
}

// New returns a client for the clusterbook at baseURL.
func New(baseURL string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Client{base: strings.TrimSuffix(baseURL, "/"), http: &http.Client{Timeout: timeout}}
}

// List returns every cluster with its allocation, sorted by name. The list
// endpoint only carries names and counts, so the details are fetched per
// cluster (in parallel).
func (c *Client) List(ctx context.Context) ([]Record, error) {
	var names []struct {
		Cluster string `json:"cluster"`
	}
	if err := c.get(ctx, "/api/v1/clusters", &names); err != nil {
		return nil, err
	}

	out := make([]Record, len(names))
	errs := make([]error, len(names))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i, n := range names {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			var r Record
			if err := c.get(ctx, "/api/v1/clusters/"+url.PathEscape(name), &r); err != nil {
				errs[i] = err
				return
			}
			if r.Cluster == "" {
				r.Cluster = name
			}
			out[i] = r
		}(i, n.Cluster)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Cluster < out[j].Cluster })
	return out, nil
}

func (c *Client) get(ctx context.Context, path string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", path, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("GET %s: %w", path, err)
	}
	return nil
}
