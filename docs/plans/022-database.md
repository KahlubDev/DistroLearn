# 022: Database, membership, and tenant scope

The Phase 5 database ticket. It adds the membership table ADR 0003 layer 2 names, closes the
two findings the 017 review left open, and pins both decisions as tests.

- Branch: `feature/022-db-plan` for this document. The build branch follows the accepted
  amendments.
- ADR dependency: **none accepted.** Two amendments are proposed and both this plan and its
  builder are blocked on them, named per step below.
  - `docs/adr/0008-tenant-scope-discipline.md`, status proposed, amends ADR 0003 and
    ADR 0006.
  - `docs/adr/0009-tenant-directory-visibility.md`, status proposed, amends ADR 0003.
- ADR dependency, already accepted: ADR 0003, ADR 0005, ADR 0006.

## Read first

`docs/adr/0008-tenant-scope-discipline.md`, `docs/adr/0009-tenant-directory-visibility.md`,
`docs/adr/0003-tenancy-model.md`, `docs/adr/0005-db-migrations.md`,
`docs/adr/0006-db-pooler.md`, `docs/plans/018-database.md`,
`docs/reviews/017-compose.md` findings 4 and 6, `services/internal/db/db.go`,
`services/internal/db/tenant_isolation_test.go`, `services/migrations/00001_tenancy.sql`,
`compose.yaml`.

## Blocked on

Nothing starts until both amendments are accepted. The steps below say which is blocking.

| Step | Blocked on |
|---|---|
| Unexport `SetTenant`, make `InTenant` the only exported entry, move `tenant_isolation_test.go` to `package db` | 0008 |
| Source-scanning test rejecting a bare tenant `SET` | 0008 |
| README line on `docker compose restart pgbouncer` after a tenant-scope edit | 0008 |
| `00002_memberships.sql` and the `memberships` row security | 0009 |
| `TestTenantDirectory_ReadableUnscoped` | 0009 |
| Postgres healthcheck window | none, can start immediately |

## Change

### Migrations

New work starts at `00002`. ADR 0005 makes migration files append-only once merged, and
`00001_tenancy.sql` merged with PR #11, so `00001` is now frozen. Any change to it needs a
new numbered file.

`00002_memberships.sql`:

| Object | Definition |
|---|---|
| `memberships.user_id` | `uuid`, not null. The Cognito subject. No FK: identity lives outside this database |
| `memberships.tenant_id` | `uuid`, not null, references `tenants (id)` |
| `memberships.role` | `text`, not null, check in `('admin', 'instructor', 'learner')`. The three roles ADR 0003 names |
| `memberships.created_at` | `timestamptz`, not null, default `now()` |
| Primary key | `(user_id, tenant_id)`. One role per user per institution, so a user holds several memberships in distinct rows |
| Index | `memberships_tenant_id_idx` on `(tenant_id)`, for the per-tenant roster read |
| Row security | enabled, policy `memberships_tenant_isolation` on `tenant_id = NULLIF(current_setting('app.tenant_id', true), '')::uuid`, both `USING` and `WITH CHECK`. Same shape as `labs_tenant_isolation` in `00001` |
| Grants | `SELECT, INSERT` to the application role, granted through the `distrolearn.app_role` setting that `00001` already uses, not through a literal |

`tenants` and `labs` are unchanged. No column is added to either.

### The tenant model

Two settings, and this is the part that needs a decision beyond the two amendments.

`app.tenant_id` is unchanged. `set_config(..., is_local => true)` inside a transaction, set
by `InTenant`, read by the policies on `labs` and `memberships`. ADR 0008 covers how it is
set and by whom.

`app.user_id` is new and is **not** covered by either proposed amendment. Membership
resolution needs to answer "which tenants does this user belong to" before any tenant is
known, so it needs the user set at a point where `app.tenant_id` cannot be. That is a
second GUC with its own scope rule, its own reset guarantee, and its own interaction with
`server_reset_query_always`. It gets its own amendment before the builder writes it. Until
that amendment is accepted, `memberships` can be created and populated, but no request path
reads it.

`tenants` stays readable unscoped. ADR 0009 carries the reason: a policy keyed on
`app.tenant_id` filters every row out of the unscoped lookup that resolves the tenant.

### The three pooled-connection variants

They exist and they are not rebuilt. `TestTenantIsolation_SequentialSameConnection`,
`TestTenantIsolation_InterleavedTransactions` and `TestTenantIsolation_PoolCappedAtOneConnection`
run in CI through the `tenant isolation (compose)` check that PR #16 added, which also
fails the run when a variant skips. Ticket 021 closed finding 4.

Phase 5 changes their package, from `db_test` to `db`, so they can reach the unexported
tenant helper ADR 0008 requires. The assertions are identical. Running them against a real
PgBouncer and a real Postgres is what makes them worth anything, and that is already wired.

### Postgres healthcheck window

Not in either amendment, and worth doing in this ticket because it was hit three times while
writing it. `compose.yaml` gives postgres `start_period: 5s` and `retries: 30` at
`interval: 2s`, so the container is marked unhealthy after about 65 seconds. On a cold
volume, `initdb` with data checksums took longer than that on the machine this was written
on, and `docker compose up --wait` failed with `dependency failed to start: container
distrolearn-postgres-1 is unhealthy` while the entrypoint was still at `performing
post-bootstrap initialization`. Raising `start_period` to 60s and `retries` to 60 gives the
container about three minutes before it is declared unhealthy, which covers a slow cold
init without waiting on the `up --wait` timeout.

This is the one step in the plan that can start before either amendment is accepted.

## Tests

`services/internal/db/membership_test.go`, new, against the compose stack:

- `TestMembership_VisibleOnlyWithinTenant`. Seeds a membership for tenant A and one for
  tenant B through the owner role. Reads as A, asserts exactly A's row, then reads as B and
  asserts exactly B's, then reads unscoped and asserts nothing. The unscoped assertion is
  the one that bites, and it is the same shape as the third assertion in every ADR 0006
  variant.
- `TestMembership_PrimaryKeyRejectsSecondRole`. Inserts `(user_id, tenant_id)` twice and
  asserts the second is a unique violation. A user with two roles in one institution is a
  data error, not a feature.
- `TestTenantDirectory_ReadableUnscoped`. Reads `tenants` in a transaction that sets no
  tenant and asserts both seeded rows come back. This pins ADR 0009. If someone enables row
  security on `tenants` later, this fails and the failure names the ADR that has to change.

`services/internal/db/tenant_scope_source_test.go`, new, no database needed:

- `TestTenantScopeSetOnlyInHelper`. Reads every `.go` file under `services/` and fails if
  `set_config` appears with a third argument other than `true`, or if the tenant GUC name
  appears outside `services/internal/db/db.go`. The measured regression is exactly a change
  of that third argument from `true` to `false`, so this test fails on it and passes without
  it.

The three ADR 0006 variants stay as they are, in their new package.

## Acceptance criteria

- `00002_memberships.sql` exists, starts at `00002`, and grants through
  `distrolearn.app_role` rather than a literal role name.
- `memberships` has row security enabled, with a policy on `tenant_id` in the same shape as
  `labs_tenant_isolation`.
- `tenants` has no row security, and `TestTenantDirectory_ReadableUnscoped` says so
  executably.
- `SetTenant` is unexported, `InTenant` is the only exported way to set tenant scope, and
  `tenant_isolation_test.go` is `package db`.
- `TestTenantScopeSetOnlyInHelper` fails when `set_config($1, $2, true)` becomes
  `set_config($1, $2, false)` in `db.SetTenant`, and passes when it is correct. Paste both
  runs.
- `server_reset_query_always` is 0 in `docker/pgbouncer/pgbouncer.ini.template`.
- Postgres survives a cold volume with `docker compose up --wait` and no manual retry.
- All three ADR 0006 variants pass against the stack, and the `tenant isolation (compose)`
  check is green.
- No request path reads `memberships` until the `app.user_id` amendment is accepted.
- `make check` passes, with the database tests skipping under it because no DSN is set, the
  same as today.

## Not in this ticket

Reading a membership row into a request. That needs the `app.user_id` amendment and a
handler, and neither exists. Lab lifecycle, NATS, region routing, and anything about
production deployment.