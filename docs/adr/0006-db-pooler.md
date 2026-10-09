# ADR 0006: Connection pooler

- Status: accepted
- Date: 2026-10-09
- Related: `0003-tenancy-model.md`, `0005-db-migrations.md`, `docs/research/011-status.md`

## Context

`011-status.md` settled the RLS mechanism: `SET LOCAL app.tenant_id` inside every
transaction, one application role. That decision constrains the pooler. ADR 0001 sizes
the API for thousands of held-open connections, so the pooler is not optional at scale.

## Decision

**Run PgBouncer in transaction mode, self-hosted, in the cluster.**

Transaction mode assigns a server connection for the length of a transaction and returns
it afterwards. That matches the lifetime of `SET LOCAL`, which resets at commit. The
tenant value cannot survive into the next request on a different transaction, because the
previous one already ended.

Every request wraps its queries in one transaction, which ADR 0003 requires anyway. The
transaction boundary is the same boundary the pooler keys on.

`server_reset_query` is not the safety net. It is skipped by default in transaction mode,
so it does not run when a server connection returns to the pool. Tenant scoping comes
from `SET LOCAL` being transaction-scoped, and that is the only mechanism relied on.

## Required test: two tenants across a pooled connection

This ADR is only true if `SET LOCAL` actually resets. That is a property of the pooler and
the transaction boundary together, so it gets a test rather than a comment.

**Required before this ADR can be considered implemented:** a test that inserts a row for
tenant A and one for tenant B, then through PgBouncer in transaction mode reads as tenant
A, and again reads as tenant B. It must assert tenant A sees exactly its own row and never
tenant B's, in both orders, and it must hold the pool size at or below the number of
concurrent requests so the same physical connection is reused.

Three variants, since each catches a different mistake:

- Sequential: A reads, then B reads, on the same connection. Catches a `SET` that was not
  `LOCAL`.
- Interleaved transactions: A begins and reads, B begins and reads before A commits.
  Catches isolation between concurrent transactions.
- Forced reuse: the pool is capped at one connection for the test, so B necessarily runs
  on the connection A just returned. Catches a reset that depends on something other than
  the transaction ending.

A failure here means tenant data is readable across tenants, so it is a release blocker
rather than a flaky test. The test belongs to the Phase 5 database ticket and runs in CI
against a real Postgres with a real PgBouncer, since a mock pooler proves nothing about
reset behavior.

## Consequences

- Session mode is wrong here. It pins a server connection for the whole client session,
  which defeats the pooling the API needs on the interactive path.
- Statement pooling is wrong too. It forbids multi-statement transactions, and every
  tenant-scoped request needs one.
- Prepared statements need `max_prepared_statements`, which recent PgBouncer supports.
  Without it, prepared-statement use forces a fallback. Worth confirming during the
  Phase 5 database ticket before relying on it.
- We own the PgBouncer deployment: upgrades, backups are not needed, but failover and
  connection-limit tuning are ours.

## Alternatives considered

| Option | Rejected | Reason |
|---|---|---|
| RDS Proxy | Managed, but adds a hop on the interactive path that ADR 0001 already calls the first capacity limit. Also ties the pooler to RDS and cannot be reproduced in local compose |
| Session mode | Holds a server connection for the client session, which is the connection cost we are trying to avoid |
| Statement mode | Forbids the multi-statement transaction every tenant-scoped request needs |
| PgCat | Promising, newer, and a smaller operational record than PgBouncer |
| No pooler | One database connection per API connection. Fine at scaffold scale, wrong at the 5k peak in ADR 0002 |

## Revisit triggers

Revisit when RDS Proxy's latency proves irrelevant next to the API, when PgBouncer cannot
meet the measured connection count, when managed Postgres arrives with its own
pooling that resolves the tenancy question differently, or when the two-tenant
pooled-connection test fails under a configuration change.