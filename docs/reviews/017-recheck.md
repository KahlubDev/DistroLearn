# 017 QA recheck: local compose stack

PR #11 at `373e1ab`, the fix commit for `docs/reviews/017-compose.md`. Reviewed 2026-10-11
from a fresh `git clone --branch feature/017-local-compose`, Docker Server 29.8.2, Compose
v5.5.1, Go 1.27.1.

Verdict: all seven findings assigned to this commit are fixed and each fix was reproduced
from scratch. Three new defects, none of them in the fixes themselves. Findings 4 and 6
from the first review are still open by design.

## Finding 1, first `up --wait` on an empty volume: fixed

`docker compose down -v`, then one `docker compose up --wait` on an empty volume with the
default `.env`:

```
NAME                      IMAGE                                     COMMAND                SERVICE    STATUS                        PORTS
distrolearn-api-1         distrolearn-api:dev                       "/service"             api        Up 16 seconds (healthy)       8080/tcp
distrolearn-pgbouncer-1   edoburu/pgbouncer@sha256:7d7a27d9…9b2340   "/bin/sh /render-use…" pgbouncer  Up 22 seconds (healthy)       127.0.0.1:1536->6432/tcp
distrolearn-postgres-1    postgres@sha256:74935e72…cfc4336          "docker-entrypoint.s…" postgres   Up About a minute (healthy)   127.0.0.1:1532->5432/tcp
distrolearn-workers-1     distrolearn-workers:dev                   "/service"             workers    Up 19 seconds (healthy)       8081/tcp
```

No second `up`, no manual sleep. `compose.yaml:39` probes with `-h 127.0.0.1`.

## Finding 2, `POSTGRES_APP_USER` honoured: fixed

`POSTGRES_APP_USER=labapp` and `POSTGRES_DB=distrolearn_recheck` together, empty volume:

```
api         Up 7 seconds (healthy)
pgbouncer   Up 12 seconds (healthy)   127.0.0.1:1542->6432/tcp
postgres    Up 22 seconds (healthy)   127.0.0.1:1541->5432/tcp
web         Up About a minute         127.0.0.1:1537->3000/tcp
workers     Up 9 seconds (healthy)
```

Every service healthy. The role followed the variable and the grants followed the role:

```
 rolname | rolsuper | rolbypassrls
 labapp  | f        | f

 labs    | {distrolearn=arwdDxtm/distrolearn,labapp=ar/distrolearn}   | distrolearn
 tenants | {distrolearn=arwdDxtm/distrolearn,labapp=ar/distrolearn}   | distrolearn

 datname           | setconfig
 distrolearn_recheck | {distrolearn.app_role=labapp}
```

`labapp=ar` is INSERT and SELECT, the two grants the migration intends, and nothing else.
The `DO $grants$` block with `format %I` is a sound way to do this. No role named `app`
exists in this stack.

## Finding 3, `POSTGRES_DB` honoured: fixed

The template renders and PgBouncer loads it:

```
[databases]
distrolearn_recheck = host=postgres port=5432 dbname=distrolearn_recheck
```

Live `SHOW DATABASES` from the running pooler agrees:

```
distrolearn_recheck|postgres|5432|distrolearn_recheck||20|5|5|3600|||0|5|0|2|0|0
pgbouncer||6432|pgbouncer|pgbouncer|2|0|0|3600|statement|||0|0|0|1|0|0
```

## Finding 5, web reachable: fixed

```
distrolearn-web-1  distrolearn-web:dev  Up 2 seconds  127.0.0.1:1537->3000/tcp
```

Loopback and ephemeral, matching the other two services.

## Finding 7, stale comment: fixed

`pgbouncer.ini.template:46` now credits `/render-userlist.sh`. No `entrypoint.sh` reference
remains anywhere in the tree.

## Finding 8, PgBouncer admin has its own password: fixed

```
app               password_length=13
pgbouncer_admin   password_length=25
```

The app credential is refused by the pooler console:

```
FATAL:  not allowed
```

The admin credential is accepted and reports `pool_mode=transaction`. Both files are mode
`0600` inside container-local `/tmp`, neither of which is a bind mount.

## Finding 9, README documents the volume behaviour: fixed

A `### Changing a password` section names all three variables, gives the failure text, and
gives the `down -v` recovery.

## App role privileges

`rolsuper`, `rolcreatedb`, `rolcreaterole`, `rolreplication`, `rolbypassrls` are all false.
`rolinherit` is true, which is the default and grants nothing on its own. No memberships in
`pg_auth_members`. `has_schema_privilege('app','public','CREATE')` and
`has_database_privilege('app',current_database(),'CREATE')` are both false. Per table in its
own database:

```
labs            sel=t ins=t upd=f del=f
tenants         sel=t ins=t upd=f del=f
goose_db_version sel=f ins=f upd=f del=f
```

That is exactly the migration's `GRANT SELECT, INSERT`, no more. The roughly 190 `pg_*`
catalog relations that read as accessible come from the default PUBLIC grants on catalogs,
not from anything this stack granted.

Through PgBouncer the app role reaches one database and is refused the rest:

```
db=postgres     -> FATAL:  no such database: postgres
db=template1    -> FATAL:  no such database: template1
db=distrolearn  -> 1
```

One caveat. Connecting straight to Postgres, bypassing the pooler, the app role does reach
the `postgres` and `template1` databases, because `datacl` is empty on both and the
default gives PUBLIC CONNECT. It can do nothing once there: `CREATE TABLE` returns
`permission denied for schema public`, and `public` carries only
`pg_database_owner=UC/pg_database_owner,=U/pg_database_owner`, so USAGE to PUBLIC and
nothing else. Local compose only, and it is what any non-superuser role gets by default in
Postgres. Noted so nobody reads the CONNECT grant as a decision this stack made.

## Tenant isolation

Three variants pass against the overridden stack:

```
--- PASS: TestTenantIsolation_SequentialSameConnection (0.23s)
--- PASS: TestTenantIsolation_InterleavedTransactions (0.09s)
--- PASS: TestTenantIsolation_PoolCappedAtOneConnection (0.06s)
```

Plain `SET` swap, `set_config($1, $2, false)`, fails all three:

```
--- FAIL: TestTenantIsolation_SequentialSameConnection
    saw [lab-for-b], want [] (a tenant read another tenant's row, or read nothing)
--- FAIL: TestTenantIsolation_InterleavedTransactions
    saw [lab-for-b], want []
--- FAIL: TestTenantIsolation_PoolCappedAtOneConnection
    saw [lab-for-b], want []
```

Restoring `true` and re-running without touching PgBouncer still fails, leaking `lab-for-a`
this time. After `docker compose restart pgbouncer` all three pass. Same behaviour as the
first review, and it is worth a line in the runbook rather than a code change.

## Rendered config and secrets in git

`git rev-list --all --objects` finds one blob named `docker/pgbouncer/pgbouncer.ini`, from
`4be334e`, the hand-written file that `373e1ab` renamed to `.template`. It holds no
credential, only `auth_file = /tmp/userlist.txt`. No rendered ini is tracked and none exists
in the working tree.

The three passwords used for this run appear in zero commits under `git log --all -p -S`.
`change-me-owner`, `change-me-app`, and `change-me-pgbouncer-admin` appear only in
`.env.example`. `git show 373e1ab` adds no literal credential: every password reference is
either `${VAR}`, `change-me-pgbouncer-admin`, or prose. Token-pattern sweeps over the single
commit and over the full 13278-line history for AWS, GitHub, Slack, Stripe, Google, JWT, and
private key shapes return nothing.

## `make check`

Passes. `gofmt` clean, `go vet` clean, web lint, typecheck, vitest, and Next build all pass.

`docker/compose_test.sh` on a clean tree reports `passed: 5, failed: 0`, including the new
admin-credential check.

## New findings

### A. `docker compose up` does not rebuild after a migration changes

`services/migrations/embed.go` embeds the SQL with `//go:embed *.sql`, per ADR 0005, so the
migrations are inside the binary. Adding `00002_probe.sql` and running
`docker compose up -d --wait`:

```
max goose version_id: 1     (expect 2)
```

Every service reported healthy. With `up --build`:

```
max goose version_id: 2
```

Compose reuses the existing `distrolearn-api` image, the old binary runs, and goose never
sees the new file. Nothing warns. This is the gotcha the ticket already plans to document in
the README under 021, and it deserves the space, because the failure is a healthy-looking
stack on an out-of-date schema rather than an error.

### B. `docker compose down` leaves `web` running

After `docker compose --profile web up`, a plain `docker compose down` produces no output
for `web` and the container stays up on its host port. Compose only stops profile services
when the profile is active, which is documented compose behaviour, but `README.md:59`
presents `docker compose down` as the way to stop the stack. `docker compose --profile web
down` is what actually stops it.

### C. `compose_test.sh` reports a healthy service as unhealthy

Reproduced from a clean tree: `--profile web up`, then plain `down` (leaving `web` running
per finding B), then `docker/compose_test.sh`:

```
FAIL  not healthy: web
...
passed: 4, failed: 1
```

`compose_test.sh:33` reads `docker compose ps --format '{{.Service}} {{.Health}}'` and fails
anything whose `Health` is not the literal string `healthy`. `web` declares no healthcheck,
so its field is empty, and empty is counted as unhealthy. This got more likely now that
finding 5 makes `--profile web` worth running.

## Still open

**Finding 4.** The three ADR 0006 variants skip under `make test`:

```
--- SKIP: TestTenantIsolation_SequentialSameConnection
--- SKIP: TestTenantIsolation_InterleavedTransactions
--- SKIP: TestTenantIsolation_PoolCappedAtOneConnection
ok  	github.com/KahlubDev/DistroLearn/services/internal/db	0.013s
```

`ci.yml` is still the only workflow and still never brings up compose. A green build says
nothing about tenant isolation.

**Finding 6.** `relrowsecurity` is `f` on `tenants` and `t` on `labs`. Unchanged.

## Definition of Done

| Item | Result |
|---|---|
| `make build` | Passes |
| `make test` | Passes, ADR 0006 variants skip (finding 4) |
| New tests for the change | `compose_test.sh` gains the admin-password check, which fails if the two passwords match |
| `make lint` | Passes |
| No unrelated changes | The diff is confined to the seven findings plus the plan-text fix |
| Docs updated | README gains the password section, plan 017 loses the PgBouncer contradiction |

## Not checked

Production deployment, region routing, and the NATS dependency in ADR 0001.
`max_prepared_statements` is still set to 200, which satisfies the ADR 0006 open question
without proving pgx uses them.