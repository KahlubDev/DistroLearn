# 017: Local compose with Postgres

One command brings up Postgres, the API, and the workers against it.

- Branch: `feature/017-local-compose`
- ADR dependency: **none.** ADR 0005 (migrations, goose) and ADR 0006 (pooler) are
  accepted, so this ticket is unblocked.
- Also depends on 014 through 016 for CI and the Dockerfiles.

## Read first

`docs/adr/0005-db-migrations.md`, `docs/adr/0006-db-pooler.md`,
`docs/adr/0003-tenancy-model.md`, `services/Dockerfile`, `apps/web/Dockerfile`,
`docs/plans/015-health-endpoints.md`.

## Change

`compose.yaml` (new), `.env.example` (new), `README.md` (add a local section).

Four services:

- `postgres`. `postgres:18.6`, matching ADR 0001. Volume-backed, so lab state survives a
  restart. Healthcheck on `pg_isready`, because the API and workers must not start
  against a database still booting.
- `api`. Built from `services/Dockerfile`, target `api`. `depends_on: postgres` with
  `condition: service_healthy`.
- `workers`. Same image, target `workers`.
- `web`. Built from `apps/web/Dockerfile`. Optional, behind a compose profile, so a
  backend-only run does not pay for a Next.js build.

Compose's Postgres version is a developer convenience and should match the pinned
18.6, which is why the tag is exact rather than `18` or `latest`.

`.env.example` holds `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, and the
`DATABASE_URL` the services read. `.env` is already in `.gitignore`.

Migrations, per ADR 0005: goose runs at API startup, with an advisory lock so two replicas
do not race. PgBouncer is not in compose. ADR 0006 covers production pooling, and a local
pooler in front of one database process hides the connection bugs worth catching on a
laptop.

Compose is where the ADR 0006 two-tenant pooled-connection test first runs. That test
needs a real PgBouncer in transaction mode in front of a real Postgres, which compose can
supply and a unit test cannot. Add a `pgbouncer` service to this file, and a test that
writes one row for each of two tenants and asserts neither can read the other's.

## Tests to add

`docker/compose_test.sh`, run by hand:

- `docker compose up -d --wait` reaches a healthy state with no manual sleep.
- `api` and `workers` both report `/healthz` 200.
- Data written through the API survives `docker compose restart`.
- `docker compose down -v` then `up` yields an empty database, so the volume is doing the
  work rather than an anonymous container layer.
- No service reaches the internet at runtime. Every image is built locally or pulled
  once at build time, so a run needs no outbound call.
- The Postgres version reported by `SELECT version()` is 18.6.

## Acceptance criteria

- `docker compose up --wait` brings up Postgres, the API, and the workers healthy.
- Postgres is pinned to 18.6 and is volume-backed.
- The API and workers do not start until Postgres passes its healthcheck.
- No credentials are committed; `.env.example` carries placeholders only.
- No service depends on a hardcoded host port, so a developer can run two stacks.
- Web is behind a profile and does not start by default.
- README documents the one command to start and stop the stack.
- A `pgbouncer` service in transaction mode runs in front of Postgres, and the ADR 0006
  two-tenant pooled-connection test passes against it. That test fails if `SET LOCAL`
  ever stops resetting, so it belongs here and in CI, not only in a unit test.