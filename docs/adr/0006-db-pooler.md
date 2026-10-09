# ADR 0006: Connection pooler

- Status: proposed
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
meet the measured connection count, or when managed Postgres arrives with its own
pooling that resolves the tenancy question differently.