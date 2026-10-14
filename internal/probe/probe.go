// Package probe performs one-shot reachability checks of HTTP(S) endpoints:
// DNS resolution, TCP connect, TLS handshake (with certificate expiry) and the
// HTTP status. It is meant for the dashboard snapshot, not for alerting —
// continuous probing belongs in blackbox-exporter.
package probe

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Stages at which a probe can fail.
const (
	StageDNS     = "dns"
	StageConnect = "connect"
	StageTLS     = "tls"
	StageHTTP    = "http"
)

// Result is the outcome of probing one URL.
type Result struct {
	URL string `json:"url"`
	// OK means the endpoint answered with a status below 500 — routing,
	// certificate and backend work. 401/403/404 count as OK: the app answered.
	OK         bool      `json:"ok"`
	Stage      string    `json:"stage,omitempty"` // failing stage when !OK
	Error      string    `json:"error,omitempty"`
	StatusCode int       `json:"statusCode,omitempty"`
	LatencyMs  int64     `json:"latencyMs"`
	CertExpiry *time.Time `json:"certExpiry,omitempty"` // leaf certificate NotAfter (HTTPS only)
	CheckedAt  time.Time `json:"checkedAt"`
}

// Prober probes URLs with a shared HTTP client.
type Prober struct {
	Timeout     time.Duration
	Concurrency int
	resolver    *net.Resolver
	client      *http.Client
}

// New returns a Prober with the given per-URL timeout.
func New(timeout time.Duration) *Prober {
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		TLSHandshakeTimeout: timeout,
		DisableKeepAlives:   true,
	}
	return &Prober{
		Timeout:     timeout,
		Concurrency: 8,
		resolver:    net.DefaultResolver,
		client: &http.Client{
			Transport: transport,
			Timeout:   timeout,
			// A redirect is an answer: report it instead of following it.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

// Probe checks a single URL.
func (p *Prober) Probe(ctx context.Context, rawURL string) (res Result) {
	res = Result{URL: rawURL, CheckedAt: time.Now()}
	start := time.Now()
	defer func() { res.LatencyMs = time.Since(start).Milliseconds() }()

	ctx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()

	u, err := url.Parse(rawURL)
	if err != nil {
		res.Stage, res.Error = StageHTTP, err.Error()
		return res
	}
	if net.ParseIP(u.Hostname()) == nil {
		if _, err := p.resolver.LookupHost(ctx, u.Hostname()); err != nil {
			res.Stage, res.Error = StageDNS, err.Error()
			return res
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		res.Stage, res.Error = StageHTTP, err.Error()
		return res
	}
	resp, err := p.client.Do(req)
	if err != nil {
		res.Stage, res.Error = classify(err), err.Error()
		return res
	}
	_ = resp.Body.Close()

	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		notAfter := resp.TLS.PeerCertificates[0].NotAfter
		res.CertExpiry = &notAfter
	}
	res.StatusCode = resp.StatusCode
	if resp.StatusCode >= 500 {
		res.Stage, res.Error = StageHTTP, resp.Status
		return res
	}
	res.OK = true
	return res
}

// ProbeAll checks all URLs concurrently and returns results keyed by URL.
func (p *Prober) ProbeAll(ctx context.Context, urls []string) map[string]Result {
	unique := make(map[string]struct{}, len(urls))
	for _, u := range urls {
		unique[u] = struct{}{}
	}

	out := make(map[string]Result, len(unique))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, max(p.Concurrency, 1))
	for u := range unique {
		wg.Add(1)
		sem <- struct{}{}
		go func(u string) {
			defer wg.Done()
			defer func() { <-sem }()
			r := p.Probe(ctx, u)
			mu.Lock()
			out[u] = r
			mu.Unlock()
		}(u)
	}
	wg.Wait()
	return out
}

func classify(err error) string {
	var certErr *tls.CertificateVerificationError
	var recordErr tls.RecordHeaderError
	var opErr *net.OpError
	switch {
	case errors.As(err, &certErr), errors.As(err, &recordErr),
		strings.Contains(err.Error(), "tls:"), strings.Contains(err.Error(), "x509:"):
		return StageTLS
	case errors.As(err, &opErr) && opErr.Op == "dial":
		return StageConnect
	default:
		return StageHTTP
	}
}
