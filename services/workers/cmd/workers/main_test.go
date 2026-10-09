package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestMuxServesHealthEndpoints fails if main stops mounting the health routes, which is
// the case where the binary builds and serves nothing.
func TestMuxServesHealthEndpoints(t *testing.T) {
	t.Parallel()

	mux := newMux()

	for _, path := range []string{"/healthz", "/readyz"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want %d", path, rec.Code, http.StatusOK)
		}
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/unknown", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /unknown = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestNatsConnReportsOK(t *testing.T) {
	t.Parallel()

	if err := natsConn(context.Background()); err != nil {
		t.Errorf("natsConn() = %v, want nil", err)
	}
}

// TestRunServesAndShutsDownCleanly replaces the skeleton test, which called a run() that
// returned immediately. run() now blocks serving, so it is cancelled instead.
func TestRunServesAndShutsDownCleanly(t *testing.T) {
	old := listenAddr
	listenAddr = "127.0.0.1:0"
	t.Cleanup(func() { listenAddr = old })

	ctx, cancel := context.WithCancel(context.Background())

	errc := make(chan error, 1)
	go func() { errc <- run(ctx) }()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("run() = %v, want nil", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("run() did not return after the context was cancelled")
	}
}
