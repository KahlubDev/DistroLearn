package db_test

import (
	"context"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KahlubDev/DistroLearn/services/internal/db"
)

// The ADR 0006 required test. Three variants, because a single one does not catch every way
// tenant state can survive a pooled connection.
//
// This runs against a real PgBouncer in transaction mode in front of a real Postgres. A mock
// pooler proves nothing about reset behaviour. Point TEST_APP_DSN and TEST_ADMIN_DSN at the
// compose stack, or the test skips.
//
// What every variant asserts:
//  1. reading as tenant A returns exactly A's row,
//  2. reading as tenant B returns exactly B's row,
//  3. reading with no tenant set returns nothing.
//
// The third assertion is what bites. With set_config(..., is_local => true) the GUC is gone
// after commit, so an unset transaction reads nothing. With a plain session-level SET the
// previous tenant's value survives on the pooled server connection, because PgBouncer skips
// server_reset_query in transaction mode, and the unset read returns the previous tenant's
// rows instead of nothing.

const (
	tenantA = "11111111-1111-1111-1111-111111111111"
	tenantB = "22222222-2222-2222-2222-222222222222"
	labA    = "aaaaaaaa-1111-1111-1111-111111111111"
	labB    = "bbbbbbbb-2222-2222-2222-222222222222"
)

func appDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_APP_DSN")
	if dsn == "" {
		t.Skip("TEST_APP_DSN not set; run docker/compose_test.sh or bring up compose")
	}
	return dsn
}

// adminPool is the owner connection used to seed rows. It bypasses RLS, which is how a row
// for another tenant can be written at all.
func adminPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_ADMIN_DSN")
	if dsn == "" {
		t.Skip("TEST_ADMIN_DSN not set; run docker/compose_test.sh or bring up compose")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open admin pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func appPool(t *testing.T, maxConns int32) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg, err := pgxpool.ParseConfig(appDSN(t))
	if err != nil {
		t.Fatalf("parse app dsn: %v", err)
	}
	cfg.MaxConns = maxConns
	cfg.MaxConnLifetime = time.Hour

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("open app pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping app pool through pgbouncer: %v", err)
	}
	return pool
}

// seed writes one tenant and one lab per tenant using the owner role, which is not subject
// to RLS.
func seed(t *testing.T, admin *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	_, err := admin.Exec(ctx, `
		INSERT INTO tenants (id, name) VALUES
			($1, 'Institution A'),
			($2, 'Institution B')
		ON CONFLICT (id) DO NOTHING`, tenantA, tenantB)
	if err != nil {
		t.Fatalf("seed tenants: %v", err)
	}
	_, err = admin.Exec(ctx, `
		INSERT INTO labs (id, tenant_id, name) VALUES
			($1, $3, 'lab-for-a'),
			($2, $4, 'lab-for-b')
		ON CONFLICT (id) DO NOTHING`, labA, labB, tenantA, tenantB)
	if err != nil {
		t.Fatalf("seed labs: %v", err)
	}
}

// readLabNames returns the lab names visible in the current transaction.
func readLabNames(t *testing.T, tx pgx.Tx) []string {
	t.Helper()
	rows, err := tx.Query(context.Background(), "SELECT name FROM labs ORDER BY name")
	if err != nil {
		t.Fatalf("query labs: %v", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan lab name: %v", err)
		}
		names = append(names, n)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
	sort.Strings(names)
	return names
}

func assertVisible(t *testing.T, got []string, want ...string) {
	t.Helper()
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("saw %v, want %v (a tenant read another tenant's row, or read nothing)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("saw %v, want %v", got, want)
		}
	}
}

// txWithTenant runs fn in a transaction with the tenant set transaction-locally.
func txWithTenant(t *testing.T, pool *pgxpool.Pool, tenantID string, fn func(pgx.Tx)) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if err := db.SetTenant(ctx, tx, tenantID); err != nil {
		t.Fatalf("set tenant: %v", err)
	}
	fn(tx)

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

// txNoTenant runs fn in a transaction that never sets the tenant, which is what a handler
// that forgot to scope its query would do.
func txNoTenant(t *testing.T, pool *pgxpool.Pool, fn func(pgx.Tx)) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	fn(tx)

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

// Variant 1, sequential. A reads, then B reads, on one acquired connection, then an
// unscoped read. Catches a SET that was not LOCAL, because the unscoped read inherits the
// value A left behind.
func TestTenantIsolation_SequentialSameConnection(t *testing.T) {
	admin := adminPool(t)
	pool := appPool(t, 4)
	seed(t, admin)

	ctx := context.Background()
	// One acquired connection, so both reads run on the same physical client session and
	// PgBouncer is free to hand them the same server connection.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer conn.Release()

	txWithTenantConn(t, conn, tenantA, func(tx pgx.Tx) {
		assertVisible(t, readLabNames(t, tx), "lab-for-a")
	})
	txWithTenantConn(t, conn, tenantB, func(tx pgx.Tx) {
		assertVisible(t, readLabNames(t, tx), "lab-for-b")
	})
	// The unscoped read is the assertion that bites.
	txNoTenantConn(t, conn, func(tx pgx.Tx) {
		assertVisible(t, readLabNames(t, tx))
	})
}

// Variant 2, interleaved. A begins and reads, B begins and reads before A commits, so both
// transactions are open at once. Caches the previous tenant's value on a shared connection.
func TestTenantIsolation_InterleavedTransactions(t *testing.T) {
	admin := adminPool(t)
	pool := appPool(t, 4)
	seed(t, admin)

	ctx := context.Background()

	txA, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin A: %v", err)
	}
	defer func() { _ = txA.Rollback(context.Background()) }()
	if err := db.SetTenant(ctx, txA, tenantA); err != nil {
		t.Fatalf("set tenant A: %v", err)
	}

	txB, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin B: %v", err)
	}
	defer func() { _ = txB.Rollback(context.Background()) }()
	if err := db.SetTenant(ctx, txB, tenantB); err != nil {
		t.Fatalf("set tenant B: %v", err)
	}

	// A still holds its transaction open, so a session-level SET on B's connection cannot
	// be the reason A still reads correctly.
	assertVisible(t, readLabNames(t, txA), "lab-for-a")
	assertVisible(t, readLabNames(t, txB), "lab-for-b")

	if err := txA.Commit(ctx); err != nil {
		t.Fatalf("commit A: %v", err)
	}
	if err := txB.Commit(ctx); err != nil {
		t.Fatalf("commit B: %v", err)
	}

	// After both commits, an unscoped read must see nothing.
	txNoTenant(t, pool, func(tx pgx.Tx) {
		assertVisible(t, readLabNames(t, tx))
	})
}

// Variant 3, forced reuse. The pool is capped at one server connection, so B necessarily runs
// on the connection A just returned. Catches any reset that depends on something other than
// the transaction ending.
func TestTenantIsolation_PoolCappedAtOneConnection(t *testing.T) {
	admin := adminPool(t)
	pool := appPool(t, 1)
	seed(t, admin)

	txWithTenant(t, pool, tenantA, func(tx pgx.Tx) {
		assertVisible(t, readLabNames(t, tx), "lab-for-a")
	})
	txWithTenant(t, pool, tenantB, func(tx pgx.Tx) {
		assertVisible(t, readLabNames(t, tx), "lab-for-b")
	})
	txNoTenant(t, pool, func(tx pgx.Tx) {
		assertVisible(t, readLabNames(t, tx))
	})
}

func txWithTenantConn(t *testing.T, conn *pgxpool.Conn, tenantID string, fn func(pgx.Tx)) {
	t.Helper()
	ctx := context.Background()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := db.SetTenant(ctx, tx, tenantID); err != nil {
		t.Fatalf("set tenant: %v", err)
	}
	fn(tx)
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func txNoTenantConn(t *testing.T, conn *pgxpool.Conn, fn func(pgx.Tx)) {
	t.Helper()
	ctx := context.Background()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	fn(tx)
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
}
