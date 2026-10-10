// Package main runs the DistroLearn workers service.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KahlubDev/DistroLearn/services/internal/db"
	"github.com/KahlubDev/DistroLearn/services/internal/health"
	"github.com/KahlubDev/DistroLearn/services/internal/healthcheck"
)

// shutdownTimeout bounds the graceful shutdown. After it, in-flight work is cut off, so
// a stuck consumer cannot hold the process open past a pod's termination grace period.
const shutdownTimeout = 10 * time.Second

// stopSignals are the signals that start a graceful shutdown. SIGTERM is what Kubernetes
// sends on pod deletion, SIGINT what a terminal sends on Ctrl-C. A second signal is left
// to the runtime, so an impatient operator can still force the process down.
var stopSignals = []os.Signal{syscall.SIGTERM, syscall.SIGINT}

// pool is the workers' Postgres pool, opened through PgBouncer.
var pool *pgxpool.Pool

// natsConn reports whether the worker can do useful work: a usable NATS connection and a
// reachable database.
//
// The NATS connection itself arrives with the JetStream client (ADR 0001 settled
// self-hosting it in the Frankfurt cluster). Until then that half reports ready, because
// there is no connection to be missing. The database half is live now, from ticket 017.
func natsConn(ctx context.Context) error {
	if pool == nil {
		return errors.New("database pool not initialised")
	}
	return pool.Ping(ctx)
}

// setup opens the pool. Only the API runs migrations, so the workers waits for the schema
// rather than racing it.
func setup(ctx context.Context) error {
	p, err := db.Open(ctx, db.Config{AppDSN: os.Getenv("DATABASE_URL")})
	if err != nil {
		return err
	}
	pool = p
	return nil
}

// version is stamped at build time with
// -ldflags "-X main.version=$(git rev-parse --short HEAD)".
var version = "dev"

func main() {
	// "healthcheck" makes the binary its own probe. The runtime image is distroless and has
	// no shell, so a CMD-SHELL healthcheck cannot run, and compose --wait needs the
	// service to report its own health.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck.Run("http://127.0.0.1" + listenAddr + "/healthz"))
	}

	slog.Info("workers starting", "version", version)

	// NotifyContext cancels ctx on the first stop signal and restores the default
	// behaviour afterwards. Without it SIGTERM kills the process outright and any
	// in-flight lab teardown message is lost.
	ctx, stop := signal.NotifyContext(context.Background(), stopSignals...)
	defer stop()

	// The pool opens before the listener, so the first readiness probe that answers is one
	// the worker can keep. No migrations here: the API owns those.
	setupCtx, cancelSetup := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelSetup()
	if err := setup(setupCtx); err != nil {
		slog.Error("workers setup failed", "error", err)
		os.Exit(1)
	}

	if err := run(ctx); err != nil {
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
		slog.Info("workers listening", "addr", addr)
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
		if err := srv.Shutdown(shutdownCtx); err != nil {
			// A deadline here means a handler overran the grace period. Report it rather
			// than exiting 0, since work was cut off.
			return err
		}
		return nil
	}
}
