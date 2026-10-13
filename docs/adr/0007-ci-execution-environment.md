# ADR 0007: CI execution environment

- Status: proposed
- Date: 2026-10-12
- Related: `0001-stack.md`, `0006-db-pooler.md`, `docs/plans/021-ci-integration.md`

## Context

ADR 0001 puts CI on GitHub Actions. ADR 0006 puts the two-tenant pooled-connection test
in CI against a real PgBouncer and a real Postgres, which is
`.github/workflows/compose.yml`.

Those two decide what CI does. Four things the compose job depends on have no decision on
record. Tickets 014 and 019 each settled one locally and neither wrote it down, so a reader
of `ci.yml` cannot tell whether the choices are rules or accidents.

- Where the job runs. The label is pinned in two workflow files that will drift apart if
  nobody notices.
- Where the database credentials come from. The job needs a Postgres owner password, an
  application password, and a PgBouncer console password. Any answer here is a rule about
  what CI is allowed to hold.
- How third-party actions are referred to.
- Whether an image built by one run is available to the next.

## Decision

**Run on GitHub-hosted `ubuntu-24.04`. No self-hosted runner.**

`ubuntu-24.04` ships Docker Engine and the Compose v2 plugin, so no Docker setup action
runs and none needs pinning. A self-hosted runner would be the only way to reach a
privileged or nested-Docker configuration, and nothing here needs one. Pinning the label
rather than `ubuntu-latest` keeps an OS release change from arriving mid-week, which is
what the runner notice recorded in `docs/reviews/014-ci.md` observation 2 predicts for
`ubuntu-latest` on 2026-10-19.

**Credentials come from `.env.example` placeholders, never from repository secrets.**

`cp .env.example .env` is a step in the job. The three placeholders are already distinct,
which is what `docs/reviews/017-recheck.md` finding 8 needs, and the stack is destroyed at
the end of the job. The database in CI is a database this workflow created and deleted.
Storing credentials for it in repository settings would give a secret to read where there
is nothing secret to read, and would put a `secrets.*` reference in a workflow that
currently has none. If a future job needs a credential that is not disposable, that job
needs its own decision.

**Every `uses:` line is a 40 character commit SHA, with the version tag in a trailing
comment.**

A tag is a mutable pointer, so pinning one leaves the action replaceable. A SHA is not.
The comment is what makes the pin reviewable.

A SHA has to be derived from the tag's commit, not from the tag object. GitHub's refs API
returns the tag object SHA for an annotated tag, and an action pinned to a tag object does
not run. `pnpm/action-setup` v6.1.0 is that case: `git/ref/tags/v6.1.0` returns tag object
`d9184bf1...`, and the commit behind it is `ea17c68d...`, which is what `ci.yml` pins
(`docs/reviews/014-ci.md` lines 44-49). Each SHA is resolved against the refs API when the
pin is written, and the resolution is recorded in the pull request body. Reading a SHA out
of another workflow does not verify it.

**No cross-run image cache.**

No `actions/cache`, no registry push, nothing that carries a Docker layer from one run to
the next. Two reasons. A cached layer is a layer whose provenance the job cannot state, so
a green run stops naming the tree it tested. And the hazard this job exists to catch is a
stale image: `docker compose up` reuses whatever image is present, so a cache that
survives a run is the same failure mode as the one the build step prevents
(`docs/reviews/017-recheck.md` finding A). The runner image already caches the Go module
and build cache through `actions/setup-go`, which is keyed on `services/go.sum` and is
verified content rather than an opaque layer.

## Consequences

- A job added later inherits all four rules without restating them. A job that needs a
  privileged runner, a real secret, or a prebuilt image has to say why it is exempt.
- The compose job is slower than it needs to be. Every run rebuilds both Go images and
  re-pulls Postgres and PgBouncer. That is the price of the provenance rule.
- Reviewers read a SHA in review. A `uses:` line that names a branch, a tag, or `@main` is
  a finding, and the tag comment says which release to check the SHA against.
- `pull_request` from a fork runs with no write token and no secrets, and this job needs
  neither. It cannot write to the repository even if a step tried to.

## Alternatives considered

| Option | Rejected | Reason |
|---|---|---|
| `ubuntu-latest` | What `ci.yml` used before the pin | A label that moves on a schedule. The migration to Ubuntu 26 was announced for 2026-10-19 |
| Self-hosted runner | Privileged Docker and a warm cache | Needs runner lifecycle, patching, and an on-call rotation for one job that does not need any of it |
| Repository secrets for the database passwords | Conventional for CI | The database is created and destroyed by the run. A secret grants access to nothing outside the job, and it puts a `secrets.*` reference in a workflow with none |
| Pinning action tags instead of SHAs | Readable, and what the tag comment is for | A tag can be moved to a new commit after review, so the pin is not a pin |
| Caching built images between runs | Saves several minutes | Carries a layer of unstated provenance into a job whose whole purpose is testing a stated tree |
| `bash docker/compose_test.sh` as the job | One script for both places | The script pins the Postgres patch version, which turns CI red when the image moves to 18.7, and its web health check and admin credential checks are stack facts rather than ADR 0006 facts |

## Revisit triggers

Revisit when a job needs Docker-in-Docker or a privileged runner, when a workflow needs a
credential that outlives the run, when a build takes long enough that the layer cache
earns its own decision with provenance attached, or when GitHub-hosted `ubuntu-24.04`
reaches end of support.
