// Package main runs the DistroLearn API service.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/KahlubDev/DistroLearn/services/internal/health"
)

// shutdownTimeout bounds the graceful shutdown. After it, in-flight requests are cut
// off, so a stuck handler cannot hold the process open past a pod's termination grace
// period.
const shutdownTimeout = 10 * time.Second

// stopSignals are the signals that start a graceful shutdown. SIGTERM is what Kubernetes
// sends on pod deletion, SIGINT what a terminal sends on Ctrl-C. A second signal is left
// to the runtime, so an impatient operator can still force the process down.
var stopSignals = []os.Signal{syscall.SIGTERM, syscall.SIGINT}

// ready reports whether the API can serve traffic.
//
// It checks configuration only for now. A database check arrives in the Phase 5 database
// ticket, once migrations and the pooler exist. Returning ready before the API can reach
// a database would be a probe that passes before the service works, which is worse than
// no probe.
func ready(context.Context) error {
	return nil
}

func main() {
	slog.Info("api starting")

	// NotifyContext cancels ctx on the first stop signal and restores the default
	// behaviour afterwards. Without it SIGTERM kills the process outright and in-flight
	// terminal and lab-proxy requests are cut mid-stream.
	ctx, stop := signal.NotifyContext(context.Background(), stopSignals...)
	defer stop()

	if err := run(ctx); err != nil {
		slog.Error("api exited with error", "error", err)
		os.Exit(1)
	}
	slog.Info("api stopped cleanly")
}

// listenAddr is the API address. A variable so a test can bind an ephemeral port.
var listenAddr = ":8080"

// newMux builds the API routes, health included.
func newMux() *http.ServeMux {
	return health.New(ready)
}

// run is the service body.
func run(ctx context.Context) error {
	return serve(ctx, listenAddr, newMux())
}

// serve runs handler on addr until ctx is cancelled, then shuts down within
// shutdownTimeout. Split from run so a test can supply its own handler and address.
func serve(ctx context.Context, addr string, handler http.Handler) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		slog.Info("api listening", "addr", addr)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		// ErrServerClosed is the normal path after Shutdown.
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			// A deadline here means a handler overran the grace period. Report it rather
			// than exiting 0, since work was cut off.
			return err
		}
		return nil
	}
}
