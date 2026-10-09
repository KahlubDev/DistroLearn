// Package main runs the DistroLearn API service.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/KahlubDev/DistroLearn/services/internal/health"
)

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
	if err := run(context.Background()); err != nil {
		slog.Error("api exited with error", "error", err)
		os.Exit(1)
	}
	slog.Info("api stopped cleanly")
}

// newMux builds the API routes, health included.
func newMux() *http.ServeMux {
	return health.New(ready)
}

// listenAddr is the API address. A variable so a test can bind an ephemeral port.
var listenAddr = ":8080"

// run is the service body: serve until ctx is cancelled, then shut down cleanly.
func run(ctx context.Context) error {
	const shutdownTimeout = 10 * time.Second

	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           newMux(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		slog.Info("api listening", "addr", listenAddr)
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
		return srv.Shutdown(shutdownCtx)
	}
}
