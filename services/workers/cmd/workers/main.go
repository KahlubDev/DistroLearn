// Package main runs the DistroLearn workers service.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/KahlubDev/DistroLearn/services/internal/health"
)

// natsConn reports whether the worker holds a usable NATS connection.
//
// An unconnected consumer accepts nothing, so a worker without one is alive but must not
// take work. The connection itself arrives with the NATS JetStream client; until then
// this reports ready, because there is no connection to be missing.
func natsConn(context.Context) error {
	return nil
}

func main() {
	slog.Info("workers starting")
	if err := run(context.Background()); err != nil {
		slog.Error("workers exited with error", "error", err)
		os.Exit(1)
	}
	slog.Info("workers stopped cleanly")
}

// listenAddr is the workers address. A variable so a test can bind an ephemeral port.
var listenAddr = ":8081"

// newMux builds the workers routes, health included.
func newMux() *http.ServeMux {
	return health.New(natsConn)
}

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
		slog.Info("workers listening", "addr", listenAddr)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
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
