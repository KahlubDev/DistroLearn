# Database plan requirement

The Phase 5 database ticket must satisfy ADR 0006's required test. Recorded here on
2026-10-09 so the requirement is not lost when that plan gets written.

## Required: two tenants across a pooled connection

A test that inserts a row for tenant A and one for tenant B, then through PgBouncer in
transaction mode reads as tenant A and again reads as tenant B, asserting neither sees the
other's row. Both orders, with the pool size capped at or below the number of concurrent
requests so a physical connection is genuinely reused.

Variants, since each catches a different mistake:

- Sequential: A reads, then B reads, on the same connection. Catches a `SET` that was not
  `LOCAL`.
- Interleaved: A begins and reads, B begins and reads before A commits.
- Forced reuse: pool capped at one connection, so B runs on the connection A just
  returned.

A failure means tenant data is readable across tenants, so this blocks the release rather
than being retried.

## Where it runs

Against a real Postgres and a real PgBouncer, never a mock. A fake pooler proves nothing
about reset behavior. Ticket 017 (`docs/plans/017-local-compose.md`) supplies both in
compose, so this test runs there and in CI from then on.

## Related

- ADR 0003: `SET LOCAL app.tenant_id` inside every transaction, one role.
- ADR 0006: PgBouncer in transaction mode; `server_reset_query` is not the safety net.

## Follow-up: Go cache path in ci.yml

Recorded 2026-10-09 from the ticket 014 QA review, and now actionable: ticket 017 added
`github.com/jackc/pgx/v5` and `github.com/pressly/goose/v3`, so `services/go.sum` exists.

**Open a follow-up on branch `fix/020-go-cache` after 017 merges.** Add

```
cache-dependency-path: services/go.sum
```

to the `actions/setup-go` step in `.github/workflows/ci.yml`. Until then every CI run
carries this warning:

```
Restore cache failed: Dependencies file is not found. Supported file pattern: go.mod
```

It is non-fatal, since `go.mod` alone is enough to key the cache. Deliberately left out of
ticket 017, which must not touch `ci.yml`.