package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProbe_Status(t *testing.T) {
	tests := []struct {
		code   int
		wantOK bool
	}{
		{http.StatusOK, true},
		{http.StatusFound, true}, // redirect is an answer, not followed
		{http.StatusUnauthorized, true},
		{http.StatusNotFound, true},
		{http.StatusBadGateway, false},
		{http.StatusServiceUnavailable, false},
	}
	for _, tt := range tests {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(5 * time.Millisecond)
			if tt.code == http.StatusFound {
				w.Header().Set("Location", "/login")
			}
			w.WriteHeader(tt.code)
		}))
		r := New(2*time.Second).Probe(context.Background(), srv.URL)
		srv.Close()
		if r.OK != tt.wantOK || r.StatusCode != tt.code {
			t.Errorf("code %d: ok=%v status=%d, want ok=%v", tt.code, r.OK, r.StatusCode, tt.wantOK)
		}
		if r.LatencyMs < 5 {
			t.Errorf("code %d: latency %dms not recorded", tt.code, r.LatencyMs)
		}
		if !tt.wantOK && r.Stage != StageHTTP {
			t.Errorf("code %d: stage %q, want http", tt.code, r.Stage)
		}
	}
}

func TestProbe_TLSUntrusted(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()

	r := New(2*time.Second).Probe(context.Background(), srv.URL)
	if r.OK || r.Stage != StageTLS {
		t.Errorf("self-signed cert: ok=%v stage=%q err=%q, want tls failure", r.OK, r.Stage, r.Error)
	}
}

func TestProbe_TLSCertExpiry(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()

	p := New(2 * time.Second)
	p.client = srv.Client() // trusts the test certificate
	p.client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	r := p.Probe(context.Background(), srv.URL)
	if !r.OK {
		t.Fatalf("probe failed: %+v", r)
	}
	if r.CertExpiry == nil || !r.CertExpiry.After(time.Now()) {
		t.Errorf("cert expiry not captured: %v", r.CertExpiry)
	}
}

func TestProbe_ConnectRefused(t *testing.T) {
	r := New(2*time.Second).Probe(context.Background(), "http://127.0.0.1:1/")
	if r.OK || r.Stage != StageConnect {
		t.Errorf("closed port: ok=%v stage=%q err=%q, want connect failure", r.OK, r.Stage, r.Error)
	}
}

func TestProbe_DNS(t *testing.T) {
	r := New(2*time.Second).Probe(context.Background(), "https://does-not-exist.invalid/")
	if r.OK || r.Stage != StageDNS {
		t.Errorf(".invalid host: ok=%v stage=%q err=%q, want dns failure", r.OK, r.Stage, r.Error)
	}
}

func TestProbeAll_Dedupes(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { hits++ }))
	defer srv.Close()

	p := New(2 * time.Second)
	p.Concurrency = 1 // hits is not synchronized
	res := p.ProbeAll(context.Background(), []string{srv.URL, srv.URL, srv.URL + "/x"})
	if len(res) != 2 || hits != 2 {
		t.Errorf("results=%d hits=%d, want 2/2", len(res), hits)
	}
	for u, r := range res {
		if !r.OK || r.URL != u {
			t.Errorf("%s: %+v", u, r)
		}
	}
}
