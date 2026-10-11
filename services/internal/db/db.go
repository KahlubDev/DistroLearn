// Package db opens Postgres connections and applies migrations.
//
// Tenant scoping is set with set_config(..., is_local => true), which is the parameterised
// form of SET LOCAL. SET itself does not accept a bind parameter, and ADR 0003 requires the
// transaction-scoped variant: the value resets at commit, so a connection returned to
// PgBouncer carries no tenant.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	"github.com/pressly/goose/v3"

	"github.com/KahlubDev/DistroLearn/services/migrations"
)

// tenantSetting is the GUC the RLS policy reads (ADR 0003).
const tenantSetting = "app.tenant_id"

// Config holds the two DSNs. Migrations must run on a direct connection: PgBouncer in
// transaction mode cannot carry session-level statements such as an advisory lock across
// transactions, and DDL does not belong in a transaction-pooled path.
type Config struct {
	// AppDSN reaches Postgres through PgBouncer, as the `app` role.
	AppDSN string
	// MigrateDSN reaches Postgres directly, as the owner role.
	MigrateDSN string
}

// Open returns a pool over the application DSN.
func Open(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	if cfg.AppDSN == "" {
		return nil, errors.New("db: AppDSN is empty")
	}
	pool, err := pgxpool.New(ctx, cfg.AppDSN)
	if err != nil {
		return nil, fmt.Errorf("db: open pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	return pool, nil
}

// Migrate applies embedded migrations under an advisory lock, so two replicas starting
// together do not race (ADR 0005).
func Migrate(ctx context.Context, migrateDSN string, log func(msg string, args ...any)) error {
	if migrateDSN == "" {
		return errors.New("db: MigrateDSN is empty")
	}

	// goose v3 takes a database/sql handle. pgx registers itself as driver "pgx".
	conn, err := sql.Open("pgx", migrateDSN)
	if err != nil {
		return fmt.Errorf("db: connect for migrations: %w", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := conn.PingContext(ctx); err != nil {
		return fmt.Errorf("db: ping for migrations: %w", err)
	}

	// Session-level lock on a direct connection. A fixed key, so every DistroLearn
	// deployment serialises on the same lock within its own database.
	const advisoryLockKey = 8_675_309
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", advisoryLockKey); err != nil {
		return fmt.Errorf("db: advisory lock: %w", err)
	}
	defer func() {
		// Best effort on a background context, so a hung unlock cannot block shutdown.
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(unlockCtx, "SELECT pg_advisory_unlock($1)", advisoryLockKey)
	}()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("db: goose dialect: %w", err)
	}
	if log != nil {
		goose.SetLogger(gooseLogger{log})
	}
	if err := goose.UpContext(ctx, conn, "."); err != nil {
		return fmt.Errorf("db: migrate: %w", err)
	}
	return nil
}

// InTenant runs fn inside a transaction with the tenant set locally.
//
// Every tenant-scoped read and write goes through here. The transaction is the boundary
// ADR 0003 relies on: the tenant value cannot outlive it.
func InTenant(ctx context.Context, pool *pgxpool.Pool, tenantID string, fn func(pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("db: begin: %w", err)
	}
	// Rollback after a successful commit is a no-op, so one path covers both exits.
	defer func() { _ = tx.Rollback(context.Background()) }()

	if err := SetTenant(ctx, tx, tenantID); err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetTenant sets the tenant for the current transaction only. is_local is true, which is
// SET LOCAL semantics: the value is discarded at commit.
func SetTenant(ctx context.Context, tx pgx.Tx, tenantID string) error {
	_, err := tx.Exec(ctx, "SELECT set_config($1, $2, true)", tenantSetting, tenantID)
	if err != nil {
		return fmt.Errorf("db: set tenant: %w", err)
	}
	return nil
}

// gooseLogger routes goose output through the service logger.
type gooseLogger struct{ log func(string, ...any) }

func (l gooseLogger) Fatalf(format string, v ...any) { panic(fmt.Sprintf(format, v...)) }
func (l gooseLogger) Printf(format string, v ...any) {
	l.log(fmt.Sprintf(format, v...))
}
