# 017 QA: local compose stack

PR #11, `feature/017-local-compose` at `4be334e`. Reviewed 2026-10-10 against
`docs/plans/017-local-compose.md`, the pooled-connection requirements in
`docs/plans/018-database.md`, ADR 0003, ADR 0005, ADR 0006, and the Definition of Done.

Environment: fresh `git clone --branch feature/017-local-compose` into a clean directory,
Docker Server 29.8.1, Compose v5.5.1, Go 1.27.1. `make check` passes on the branch.

Verdict: the ADR 0006 work is correct and the three-variant test is a real test. Nine
defects, three of them configuration that `.env.example` advertises as configurable but
the stack does not honour. Finding 1 blocks the plan's first acceptance criterion.

## First run needs two `up` commands

`docker compose up -d --wait` on an empty volume fails:

```
api-1     | ERROR api setup failed error="migrate: db: ping for migrations: failed to connect to
          | `user=distrolearn database=distrolearn`: 172.18.0.2:5432 (postgres): dial error:
          | dial tcp 172.18.0.2:5432: connect: connection refused"
container distrolearn-api-1 exited (1)
```

`compose.yaml:38` probes `pg_isready -U ... -d ...` with no `-h`, so it connects to the
unix socket. On first init the Postgres entrypoint runs a temporary server bound to the
socket only, to execute `docker-entrypoint-initdb.d`. The healthcheck passes against that
server while TCP is still closed. `depends_on: condition: service_healthy` then releases
the api and the workers into a refused connection.

Measured on a fresh volume:

```
t=1  socket: no response                        tcp: no response
t=3  socket: accepting connections              tcp: no response
t=4  socket: rejecting connections              tcp: accepting connections
```

Adding `-h 127.0.0.1` to the healthcheck closes the window. This is the state a new
developer hits, so it is the first thing the ticket is judged on. Plan 017 line 51 asks for
`docker compose up -d --wait` to reach a healthy state with no manual sleep.

## Tenant isolation

Three variants pass against the running stack:

```
--- PASS: TestTenantIsolation_SequentialSameConnection (0.18s)
--- PASS: TestTenantIsolation_InterleavedTransactions (0.18s)
--- PASS: TestTenantIsolation_PoolCappedAtOneConnection (0.19s)
```

Swapping `set_config($1, $2, true)` for `false` in `db.SetTenant`, the plain `SET` swap,
fails all three. Every variant leaks tenant B to a transaction that set no tenant:

```
--- FAIL: TestTenantIsolation_SequentialSameConnection
    saw [lab-for-b], want [] (a tenant read another tenant's row, or read nothing)
--- FAIL: TestTenantIsolation_InterleavedTransactions
    saw [lab-for-b], want []
--- FAIL: TestTenantIsolation_PoolCappedAtOneConnection
    saw [lab-for-b], want []
```

The builder's note about needing a PgBouncer restart reproduces. With the correct code
restored and PgBouncer left running, all three still fail, now leaking `lab-for-a`, the
value the plain-`SET` run left behind. After `docker compose restart pgbouncer` all three
pass again. A session-level GUC outlives the code change that set it, because
`server_reset_query_always` is 0 and transaction mode never resets the server connection.
Worth carrying into a deployment runbook, not into ADR 0006.

The `app` role is `rolsuper = f`, `rolbypassrls = f`, so the policies apply to it.

## SHOW CONFIG

```
 admin_users                 | pgbouncer_admin                    |                     | yes
 auth_type                   | scram-sha-256                      | md5                 | yes
 default_pool_size           | 20                                 | 20                  | yes
 max_client_conn             | 200                                | 100                 | yes
 max_prepared_statements     | 200                                | 200                 | yes
 pool_mode                   | transaction                        | session             | yes
 server_reset_query          | DISCARD ALL                        | DISCARD ALL         | yes
 server_reset_query_always   | 0                                  | 0                   | yes
```

`server_reset_query_always = 0` is the ADR 0006 position: tenant scoping does not depend on
the reset query. `server_version` is `18.6 (Debian 18.6-1.pgdg13+2)`.

## Images and ports

All five external images carry a digest and each digest resolves against its registry
(`docker buildx imagetools inspect`):

| Image | Digest |
|---|---|
| `postgres` | `sha256:74935e72…cfc4336` |
| `edoburu/pgbouncer` | `sha256:7d7a27d9…9b2340` |
| `golang` (build) | `sha256:162be529…d3a6d2` |
| `gcr.io/distroless/static-debian12` | `sha256:afa5c872…57f7ab` |
| `node` (web build) | `sha256:3d27e5c1…57add0` |

`api`, `workers`, and `web` are built from this repository, so they have no digest, which is
correct. No `:latest` anywhere.

Only `postgres` and `pgbouncer` publish ports, both ephemeral and both bound to loopback.
Confirmed from the host:

```
LISTEN 0 4096 127.0.0.1:14065 0.0.0.0:*
LISTEN 0 4096 127.0.0.1:14066 0.0.0.0:*
```

`api` and `workers` publish nothing and are unreachable from the host. Two stacks ran side
by side on ports 5857/5858 and 5859/5862.

## Credentials

`git rev-list --all --objects` finds no blob named `.env` on any ref. The passwords used for
this run appear in zero commits under `git log --all -p -S`. A sweep of the full patch
history for `AKIA`, `gh[pousr]_`, `github_pat_`, `xox[baprs]-`, `sk_live_`, `AIza`, JWT
shapes, and private key headers returns nothing. `change-me-owner` and `change-me-app` appear
only in `.env.example`. The long opaque strings in the history are `integrity:` hashes from
`pnpm-lock.yaml`. `.env` is ignored, confirmed by `git check-ignore -v .env`.

## Findings

### 1. First `up --wait` on an empty volume fails (blocker)

Described above. `compose.yaml:38` needs `-h 127.0.0.1`.

### 2. `POSTGRES_APP_USER` is hardcoded in the migration

`.env.example` documents it and `compose.yaml` threads `${POSTGRES_APP_USER:-app}` through
four places, but `services/migrations/00001_tenancy.sql:40-42` grants to a literal `app`:

```
ERROR api setup failed error="migrate: db: migrate: ERROR 00001_tenancy.sql: ... \"GRANT
USAGE ON SCHEMA public TO app;\": ERROR: role \"app\" does not exist (SQLSTATE 42704)"
```

ADR 0005 makes migration files append-only, so this needs a new numbered file rather than
an edit to `00001`. Alternatively `.env.example` should stop offering the knob.

### 3. `POSTGRES_DB` is hardcoded in the PgBouncer config

`docker/pgbouncer/pgbouncer.ini:14` fixes both the alias and the target:

```
distrolearn = host=postgres port=5432 dbname=distrolearn
```

With `POSTGRES_DB=distrolearn_b`, both services fail:

```
workers-1 | ERROR workers setup failed error="db: ping: failed to connect to `user=app
          | database=distrolearn_b`: ... FATAL: no such database: distrolearn_b (SQLSTATE 08P01)"
```

`POSTGRES_USER` does override cleanly. Findings 2 and 3 are the same shape: the compose file
treats four variables as configurable and three files treat them as constants.

### 4. The ADR 0006 test does not run in CI

Plan 017 line 70 says the test "belongs here and in CI, not only in a unit test", and
`018-database.md` line 27 says it "runs there and in CI from then on". `.github/workflows/`
contains only `ci.yml`, which runs `make check`. No workflow brings up compose or sets
`TEST_APP_DSN`. Under `make test` all three variants skip:

```
--- SKIP: TestTenantIsolation_SequentialSameConnection
    TEST_ADMIN_DSN not set; run docker/compose_test.sh or bring up compose
...
ok  	github.com/KahlubDev/DistroLearn/services/internal/db	0.020s
```

A green build carries no information about tenant isolation. The skip is the right default
for a laptop, so the gap is a missing CI job, not a bad skip.

### 5. `web` publishes no host port

`compose.yaml:120-129` gives `web` no `ports`, so `docker compose --profile web up` starts a
Next.js container with no reachable address. `README.md:62-67` tells the developer to run
it. Loopback binding with an ephemeral host port would match the other services.

### 6. `tenants` has no RLS and the app role can read it

`relrowsecurity` is `f` on `tenants`, and the migration grants `SELECT` on it to `app`.
A transaction scoped to tenant A reads every tenant row:

```
BEGIN; SELECT set_config('app.tenant_id','11111111-…-111111111111',true);
Institution A|eu-central-1
Institution B|eu-central-1
COMMIT
```

For a two-table scaffold this is a scope note rather than a leak of tenant-owned data, but
`tenants` is the table ADR 0003's repo and membership work will populate. Turning RLS on now
is cheap; doing it once membership tables exist is not.

### 7. Stale comment in `pgbouncer.ini`

Lines 41-42 say the userlist is "Written at startup by /entrypoint.sh". The entrypoint was
replaced by `render-userlist.sh`, which is mounted at line 55 and named at line 59.

### 8. The PgBouncer admin user shares the app password

`render-userlist.sh:25` writes `pgbouncer_admin` with `${PGBOUNCER_APP_PASSWORD}`. The
separation is by name only, so anything holding the app credential can reach `SHOW CONFIG`
and `SHUTDOWN`. Local compose only, and the comment at line 22 of the ini claims a
separation that does not hold.

### 9. Changing a password with an existing volume fails with no hint

Editing `POSTGRES_APP_PASSWORD` in `.env` and running `up` against a populated volume:

```
ERROR workers setup failed error="db: ping: failed to connect to `user=app
database=distrolearn`: ... FATAL: password authentication failed for user \"app\" (SQLSTATE 28P01)"
```

Standard Postgres behaviour, since `docker-entrypoint-initdb.d` runs once. The README does
not say so, and the fix (`docker compose down -v`) discards lab state. Worth one line in the
README.

## Plan 017 contradicts itself about PgBouncer

`docs/plans/017-local-compose.md` says both of the following.

Lines 37-40:

> Migrations, per ADR 0005: goose runs at API startup, with an advisory lock so two replicas
> do not race. **PgBouncer is not in compose.** ADR 0006 covers production pooling, and a local
> pooler in front of one database process hides the connection bugs worth catching on a
> laptop.

Lines 42-45, twenty lines later, in the same section:

> Compose is where the ADR 0006 two-tenant pooled-connection test first runs. That test needs
> a real PgBouncer in transaction mode in front of a real Postgres, which compose can supply
> and a unit test cannot. **Add a `pgbouncer` service to this file**, and a test that writes
> one row for each of two tenants and asserts neither can read the other's.

And the closing acceptance criterion at line 69 requires it again.

The two statements cannot both hold. The builder resolved it in favour of including
PgBouncer and said so in the PR body. That is the right call: ADR 0006 is accepted,
`018-database.md` line 27 names ticket 017 as the place the pooler lands, and the
justification in lines 37-40 is weak, since the whole point of a local pooler is to surface
the bugs it allegedly hides. But the plan shipped the contradiction into the document, so the
next reader has to resolve it again.

### Proposed fix

Replace lines 37-45 of `docs/plans/017-local-compose.md` with:

```
Migrations, per ADR 0005: goose runs at API startup, with an advisory lock so two replicas
do not race. Migrations take a direct connection to Postgres rather than the pooled path,
since an advisory lock is session-scoped and transaction pooling cannot carry it.

Compose also carries PgBouncer in transaction mode. It is here for one reason: ADR 0006's
two-tenant pooled-connection test needs a real pooler in front of a real Postgres, and a
unit test cannot supply either. Local pooling does not hide connection bugs here, it is the
subject under test. The services themselves point at PgBouncer, so the path under test is
the path that runs.
```

That keeps the ADR 0005 and ADR 0006 references, drops the claim that the two sections
disagree, and records why the migration path bypasses the pooler.

## Definition of Done

| Item | Result |
|---|---|
| `make build` | Passes |
| `make test` | Passes, but the three ADR 0006 variants skip (finding 4) |
| New tests for the change | Present and effective. The plain-`SET` swap fails all three |
| `make lint` | Passes, `gofmt` clean, `go vet` clean |
| No unrelated changes | The diff adds `services/internal/healthcheck` and makes readiness ping Postgres, both outside ticket 017 and both disclosed in the PR body. Acceptable |
| Docs updated | `018-database.md` updated. `017-local-compose.md` still carries the contradiction above |

## Not checked

Region routing, the NATS dependency in ADR 0001, and anything about production deployment.
`max_prepared_statements` is set to 200, which satisfies the ADR 0006 open question about
prepared statements, but nothing here proves pgx uses them.