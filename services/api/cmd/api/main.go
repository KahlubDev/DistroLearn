// Package main runs the DistroLearn API service.
package main

import (
	"context"
	"errors"
	"fmt"
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

// shutdownTimeout bounds the graceful shutdown. After it, in-flight requests are cut
// off, so a stuck handler cannot hold the process open past a pod's termination grace
// period.
const shutdownTimeout = 10 * time.Second

// stopSignals are the signals that start a graceful shutdown. SIGTERM is what Kubernetes
// sends on pod deletion, SIGINT what a terminal sends on Ctrl-C. A second signal is left
// to the runtime, so an impatient operator can still force the process down.
var stopSignals = []os.Signal{syscall.SIGTERM, syscall.SIGINT}

// pool is the application's Postgres pool, opened through PgBouncer. Readiness depends on
// it, so the package holds it rather than threading it through every call site.
var pool *pgxpool.Pool

// ready reports whether the API can serve traffic.
//
// Now that the database exists (ticket 017), readiness pings it. Liveness still does not,
// since a database blip must remove the replica from rotation rather than restart it.
func ready(ctx context.Context) error {
	if pool == nil {
		return errors.New("database pool not initialised")
	}
	return pool.Ping(ctx)
}

// setup runs migrations and opens the pool. Migrations run here, at API startup, under the
// advisory lock ADR 0005 requires. Only the API migrates; the workers wait for the schema.
func setup(ctx context.Context) error {
	cfg := db.Config{
		AppDSN:     os.Getenv("DATABASE_URL"),
		MigrateDSN: os.Getenv("MIGRATIONS_DATABASE_URL"),
	}

	if cfg.MigrateDSN != "" {
		if err := db.Migrate(ctx, cfg.MigrateDSN, slog.Info); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
		slog.Info("migrations applied")
	}

	p, err := db.Open(ctx, cfg)
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
	// no shell, so a CMD-SHELL healthcheck cannot work, and compose --wait needs the
	// service to report its own health.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck.Run("http://127.0.0.1" + listenAddr + "/healthz"))
	}

	slog.Info("api starting", "version", version)

	// NotifyContext cancels ctx on the first stop signal and restores the default
	// behaviour afterwards. Without it SIGTERM kills the process outright and in-flight
	// terminal and lab-proxy requests are cut mid-stream.
	ctx, stop := signal.NotifyContext(context.Background(), stopSignals...)
	defer stop()

	// Migrations and the pool open before the listener, so the first readiness probe that
	// answers is one the service can actually keep.
	setupCtx, cancelSetup := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelSetup()
	if err := setup(setupCtx); err != nil {
		slog.Error("api setup failed", "error", err)
		os.Exit(1)
	}

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
