# DistroLearn

[![CI](https://github.com/KahlubDev/DistroLearn/actions/workflows/ci.yml/badge.svg)](https://github.com/KahlubDev/DistroLearn/actions/workflows/ci.yml)

Browser-based hands-on distributed-systems learning. Multi-tenant by institution,
EU data residency option, ephemeral sandbox labs.

Stack: Next.js 16.4.0 (React 19.3.0) for web, Go 1.27.1 for API and workers,
PostgreSQL 18.6, NATS JetStream. Decisions are in `docs/adr/`.

## Layout

```
apps/web          Next.js App Router
services/api      Go API service
services/workers  Go worker service
charts            Helm chart (empty)
infra             Terraform (empty)
docs              product, research, architecture, adr, tasks, reviews
```

One Go module covers `services/`. pnpm workspaces cover `apps/`.

## Requirements

Node 24.21.0 (see `.nvmrc`) and Go 1.27.1.

```
nvm use
corepack enable
pnpm install --frozen-lockfile
```

## Commands

```
pnpm dev                # web at http://localhost:3000
cd services && go run ./api/cmd/api
cd services && go run ./workers/cmd/workers
```

## Checks

```
make check              # lint, typecheck, test, build for web and Go
make lint
make test
make build
```

## Local stack

Postgres, PgBouncer in transaction mode, the API, and the workers. Copy the example env
first; the real file is gitignored.

```
cp .env.example .env
docker compose up --wait    # backend
docker compose down
```

Add the web app with `--profile web`. Ports are assigned by Docker rather than fixed, so a
second stack runs alongside the first:

```
docker compose --profile web up --wait
```

The API applies migrations on startup under an advisory lock, so two replicas starting
together do not race. Readiness pings Postgres through PgBouncer; liveness does not, so a
database blip removes the replica from rotation instead of restarting it.

### Tenant isolation test

The ADR 0006 test needs a real PgBouncer in front of a real Postgres, so it runs here
rather than under `make check`:

```
docker/compose_test.sh
```

It runs all three variants and fails if any tenant can read another tenant's row.
