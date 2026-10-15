package clusterbook

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/clusters":
			_, _ = w.Write([]byte(`[{"cluster":"platform","ip_count":1},{"cluster":"edge-play","ip_count":1}]`))
		case "/api/v1/clusters/platform":
			_, _ = w.Write([]byte(`{"cluster":"platform","fqdn":"*.platform.example.com","ips":[{"network":"10.0.2","ip":"10.0.2.6","status":"ASSIGNED:DNS"}],"zone":"example.com"}`))
		case "/api/v1/clusters/edge-play":
			_, _ = w.Write([]byte(`{"cluster":"edge-play","ips":[{"network":"10.0.2","ip":"10.0.2.8","status":"ASSIGNED"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	recs, err := New(srv.URL+"/", time.Second).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 || recs[0].Cluster != "edge-play" || recs[1].Cluster != "platform" {
		t.Fatalf("records = %+v", recs)
	}
	if d := recs[1].Domain(); d != "platform.example.com" {
		t.Errorf("Domain() = %q", d)
	}
	if d := recs[0].Domain(); d != "" {
		t.Errorf("Domain() without fqdn = %q", d)
	}
	if a := recs[1].Addresses(); len(a) != 1 || a[0] != "10.0.2.6" {
		t.Errorf("Addresses() = %v", a)
	}
}

func TestListError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/clusters" {
			_, _ = w.Write([]byte(`[{"cluster":"gone"}]`))
			return
		}
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	if _, err := New(srv.URL, time.Second).List(context.Background()); err == nil {
		t.Fatal("expected error for failing detail request")
	}
}
