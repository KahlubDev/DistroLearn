# ADR 0005: Database migrations

- Status: accepted
- Date: 2026-10-09
- Related: `0003-tenancy-model.md`, `docs/research/011-status.md`

## Context

`011-status.md` left the migration tool open. Migrations run from the same Go module as
the API and workers, so they ship with the code rather than as a separate deployment.
That shapes the choice more than any feature comparison.

## Decision

**Use goose (pressly/goose) for SQL migrations.**

Migrations live in `services/migrations/` as numbered `.sql` files with
`-- +goose Up` and `-- +goose Down` sections, embedded into the binaries with
`//go:embed`. The API and workers binaries each carry the migrations and can run them at
startup.

The recommendation rests on three things.

**Embedded files travel with the binary.** One artifact per service, which matches the
single-module layout in `system.md` and means no migration step in the deploy pipeline
that can drift from the code.

**Plain SQL, with the RLS policy in the same file.** Creating a policy, enabling RLS, and
writing the grant are ordinary DDL, so the migration reads in the order it applies. Tools
that want the schema described in Go structs or HCL need a translation step between the
migration and what PostgreSQL enforces, and that step is where tenant-scoping bugs hide.

**Go migrations are available when needed later**, without a second tool. RLS backfill
migrations may need to read rows in batches, which SQL alone handles awkwardly. That is a
reason to keep goose, not a reason to adopt it today.

## Consequences

- Migrations run at service startup. Two replicas starting together needs an advisory
  lock, so the migration runner takes one before applying. That belongs in the Phase 5
  database ticket, not here.
- The `goose` binary is a dev and CI tool. Nothing in production depends on it, since
  migrations are embedded.
- Migration files are append-only once merged. A change to shipped schema is a new
  numbered file.

## Alternatives considered

| Option | Rejected | Reason |
|---|---|---|
| golang-migrate | Equally sound, but no Go-function migrations for a batched backfill later |
| Atlas | Schema described in HCL, so a second representation sits between the migration and the database. Also a separate binary and version to pin |
| dbmate | SQL-first and Go-native, but no embed support, so migrations need shipping as loose files |
| Hand-rolled runner | Small enough to write, large enough to get wrong under concurrent startup |
| Flyway or Liquibase | JVM in a Go deploy, plus their own config format |

## Revisit triggers

Revisit when a schema needs a declarative diff rather than versioned files, when Atlas
enters the picture for another reason, or when a migration needs Go logic and goose's
support proves too thin.