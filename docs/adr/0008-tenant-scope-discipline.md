# ADR 0008: Tenant scope discipline in application code

- Status: accepted
- Date: 2026-10-14, accepted 2026-10-14
- Amends: `0003-tenancy-model.md`, `0006-db-pooler.md`
- Related: `docs/reviews/017-compose.md`, `docs/plans/022-database.md`

## Context

ADR 0003 decides that the tenant comes from `SET LOCAL app.tenant_id` inside every
transaction, and ADR 0006 decides that transaction mode in PgBouncer is safe because the
transaction boundary is the boundary the tenant value dies on. Neither ADR says how the
application code is prevented from setting that value at session scope, and nothing
currently stops it.

`docs/reviews/017-compose.md` measures what happens when it does. Changing
`set_config($1, $2, true)` to `false` in `db.SetTenant` makes all three ADR 0006 variants
leak one tenant's rows to a transaction that set no tenant. Restoring the correct code and
leaving PgBouncer running does not recover: all three still fail, now leaking the value the
earlier run left behind, because a session-level GUC outlives the code that set it and only
`docker compose restart pgbouncer` clears it. PgBouncer skips `server_reset_query` in
transaction mode and `server_reset_query_always` is 0, so nothing resets a server
connection on its way back to the pool.

`services/internal/db` exposes both `InTenant`, which opens a transaction, sets the tenant,
runs a callback and commits, and `SetTenant`, which sets the tenant on a caller-supplied
transaction. The second is the shape a careless handler reaches for.

## Decision

**Keep transaction mode and `server_reset_query_always = 0`, and enforce the transaction
scope in application code with two controls.**

**One entry point.** `InTenant` becomes the only exported way to set tenant scope.
`SetTenant` is unexported. A request path that needs several statements in one transaction
uses `InTenant` with a callback rather than setting the tenant itself.

`InTenant` is the entry point for request paths, not a universal replacement for
`SetTenant`. It opens and commits the transaction itself, so it cannot express two
transactions held open at once, which is what the interleaved variant does. That is the one
case that reaches past it, and the one reason
`services/internal/db/tenant_isolation_test.go` moves from `package db_test` to `package db`
to reach the unexported helper. The three variants are unchanged. If a future request path
needs concurrent transactions under one tenant, it needs a decision rather than a
reach-around, because that is the shape where the reset guarantee has to be re-argued.

**One source-scanning test.** A test under `services/internal/db` reads the Go sources in
`services/` and fails if the tenant GUC is set anywhere except the helper, and if
`set_config` appears anywhere with a third argument other than `true`. This is the control
that would have failed the measured regression, since that regression is a one-word change
to a third argument.

**`server_reset_query_always` stays 0.** Measured on the compose stack on 2026-10-14:

| `set_config` third argument | `server_reset_query_always` | three ADR 0006 variants |
|---|---|---|
| `true`, correct | 0 | pass |
| `false`, broken | 0 | fail, all three |
| `true`, correct | 1 | pass |
| `false`, broken | 1 | **pass** |

The last row is the reason. With `server_reset_query_always = 1`, PgBouncer issues
`DISCARD ALL` when a server connection returns to the pool, which clears the poisoned GUC,
so all three variants pass against code that leaks tenants. ADR 0006 makes that test
mandatory and calls a failure a release blocker. Enabling C would keep the test green while
the defect it exists to catch was shipping, which is worse than not having the test.

## Consequences

- The scan test has to know the helper's location. A rename breaks it, on purpose.
- `tenant_isolation_test.go` becomes an internal test. It loses the black-box property of
  importing `db` as a consumer would, and gains the ability to assert on the unexported
  helper. The three variants assert against Postgres through PgBouncer either way, so the
  behaviour under test does not change.
- `SetTenant` being unexported does not stop a caller writing
  `tx.Exec(ctx, "SET app.tenant_id = $1", id)`. Nothing at the application layer stops that.
  The scan test is what covers it, which is why both controls are in the decision rather
  than either alone.
- `server_reset_query_always` stays 0, so a developer who runs the broken code locally
  carries a poisoned value until they restart PgBouncer. The README already tells
  developers to run `docker compose up --build` after a migration edit; a line on
  `docker compose restart pgbouncer` after a tenant-scope edit belongs with it.
- Option A alone is a soft control, and option B alone leaves the misleading exported
  helper in the API. Together they are the smallest pair that catches the measured
  regression and removes the shape that invites it.

## Alternatives considered

| Option | Rejected | Reason |
|---|---|---|
| `server_reset_query_always = 1` | Measured to mask all three ADR 0006 variants when the code is broken | Keeps a mandatory test green against leaking code. Also one `DISCARD ALL` round trip to Postgres per transaction on the hot path, which was not measured here |
| Keep `SetTenant` exported | It is the shape a handler reaches for when it wants to scope its own transaction | Two entry points, one of which is the one the finding came from |
| A Go lint rule rejecting `set_config` | Needs a new linter and configuration in a repo that runs `go vet` and `gofmt` | Same detection as a source-scanning test, more moving parts |
| Advisory lock per transaction | Serialises tenant-scoped work | Does not survive a crash mid-transaction |
| Reset the GUC in `InTenant`'s cleanup | Duplicates what `SET LOCAL` already does, and misses any path that does not use `InTenant` | The defect is a missing reset only because the reset is transaction-scoped |

## Revisit triggers

Revisit when the scan test produces a false positive that blocks real work, when
`memberships` lands and a second GUC needs the same discipline, or when a PgBouncer version
makes `DISCARD ALL` free enough on the transaction path to reconsider the table above
against a measured latency budget.