package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"syscall"
	"testing"
	"time"
)

// TestMuxServesHealthEndpoints fails if main stops mounting the health routes, which is
// the case where the binary builds and serves nothing.
func TestMuxServesHealthEndpoints(t *testing.T) {
	t.Parallel()

	mux := newMux()

	// Liveness does not touch the database, so it answers 200 with no pool configured.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("GET /healthz = %d, want %d", rec.Code, http.StatusOK)
	}

	// Readiness pings Postgres from ticket 017 on, so with no pool it must be 503 rather
	// than claiming the replica can take traffic.
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("GET /readyz without a pool = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/unknown", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /unknown = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// TestReadyFailsWithoutPool covers the pre-setup case. Readiness pings Postgres from ticket
// 017 on, so an API that has not opened its pool must report not ready rather than claiming
// it can serve.
func TestReadyFailsWithoutPool(t *testing.T) {
	t.Parallel()

	old := pool
	pool = nil
	t.Cleanup(func() { pool = old })

	if err := ready(context.Background()); err == nil {
		t.Error("ready() = nil with no pool, want an error so the replica stays out of rotation")
	}
}

// TestStopSignalsCoverSigtermAndSigint pins the signals that start a graceful shutdown.
// SIGTERM is what a Kubernetes pod deletion sends, SIGINT is Ctrl-C. Losing either means
// the process is killed with in-flight requests cut off.
func TestStopSignalsCoverSigtermAndSigint(t *testing.T) {
	t.Parallel()

	want := map[os.Signal]bool{syscall.SIGTERM: false, syscall.SIGINT: false}
	for _, sig := range stopSignals {
		if _, ok := want[sig]; ok {
			want[sig] = true
		}
	}
	for sig, found := range want {
		if !found {
			t.Errorf("stopSignals is missing %v, so the process would not shut down gracefully on it", sig)
		}
	}
}

// TestServeStopsOnCancelledContext covers the shutdown path a stop signal triggers:
// cancelling the context is what signal.NotifyContext does on SIGTERM or SIGINT.
func TestServeStopsOnCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	errc := make(chan error, 1)
	go func() { errc <- serve(ctx, "127.0.0.1:0", http.NewServeMux()) }()

	// Let the listener come up before cancelling, so the shutdown path runs rather than
	// a cancellation that lands before ListenAndServe starts.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("serve() = %v, want nil", err)
		}
	case <-time.After(shutdownTimeout + 5*time.Second):
		t.Fatal("serve() did not return after the context was cancelled")
	}
}

// TestServeAnswersRequestsBeforeShutdown confirms the server is actually serving during
// its life, so the shutdown test above is not passing against a server that never bound.
func TestServeAnswersRequestsBeforeShutdown(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	// Closed so serve() can bind it. The port is free again for the length of the test.
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errc := make(chan error, 1)
	go func() { errc <- serve(ctx, addr, mux) }()

	url := "http://" + addr + "/ping"
	var lastErr error
	for range 50 {
		resp, err := http.Get(url) //nolint:noctx // short local probe in a test
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET /ping = %d, want %d", resp.StatusCode, http.StatusOK)
			}
			cancel()
			if err := <-errc; err != nil {
				t.Fatalf("serve() = %v, want nil", err)
			}
			return
		}
		lastErr = err
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("server never answered /ping: %v", lastErr)
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
	case <-time.After(shutdownTimeout + 5*time.Second):
		t.Fatal("run() did not return after the context was cancelled")
	}
}
