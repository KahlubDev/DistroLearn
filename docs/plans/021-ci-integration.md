# 021: CI job for the compose-backed tenant isolation test

One GitHub Actions job brings up the compose stack and runs the three ADR 0006 variants
against a real PgBouncer in front of a real Postgres, and goes red when any of them skips.

- Branch: `feature/021-ci-plan`
- Closes finding 4 in `docs/reviews/017-compose.md` and `docs/reviews/017-recheck.md`, which
  `docs/plans/018-database.md` line 27 already promised.
- ADR dependency: ADR 0006 (the test runs in CI), ADR 0001 (GitHub Actions).
- Proposal: `docs/adr/0007-ci-execution-environment.md`, status proposed. It records the
  runner, the pinning rule, and the credential rule, none of which an accepted ADR covers.

## Read first

`docs/adr/0006-db-pooler.md`, `docs/adr/0005-db-migrations.md`, `docs/adr/0001-stack.md`,
`docs/plans/017-local-compose.md`, `docs/plans/018-database.md`,
`docs/reviews/017-recheck.md` finding A, `compose.yaml`, `docker/compose_test.sh`,
`services/internal/db/tenant_isolation_test.go`, `.github/workflows/ci.yml`.

## Change

| File | Change |
|---|---|
| `.github/workflows/compose.yml` | New. One job, eleven steps |
| `docs/adr/0007-ci-execution-environment.md` | New, status proposed |
| `README.md` | Second badge, and the tenant isolation section says where the variants run in CI |
| `docs/plans/018-database.md` | The "runs there and in CI from then on" line links to this plan |

Nothing under `services/` changes. `tenant_isolation_test.go` keeps its skip guard, since
the skip is right for a laptop. `ci.yml` and `docker/compose_test.sh` are untouched.

### The job

`name: Compose`, job id `tenant-isolation`, job name `tenant isolation (compose)`. That name
is the string a branch protection rule selects, the way `make check` is today
(`docs/reviews/014-ci.md` lines 154-156). Triggers match `ci.yml`: `push` to `main`,
`pull_request`, and `workflow_dispatch`. The dispatch trigger exists for the red-run probe,
the same reason it is in `ci.yml` (lines 52-57 there).

`concurrency: group: compose-${{ github.ref }}`, a separate group from `ci-${{ github.ref }}`
so a new push does not cancel a compose run and a compose run does not cancel a check run.
`permissions: contents: read` at the top level, `timeout-minutes: 20`.

`runs-on: ubuntu-24.04`, the pin `ci.yml` already carries. That image ships Docker Engine
and the Compose v2 plugin, so no Docker setup action is needed and none is pinned.

| # | Step | Why it exists |
|---|---|---|
| 1 | Check out, `actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1` | Same SHA as `ci.yml`, re-verified 2026-10-12 against the refs API (lightweight tag) |
| 2 | Set up Go, `actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0`, `go-version-file: services/go.mod`, `cache-dependency-path: services/go.sum` | `go test` runs on the runner. The Go version comes from `go.mod`, the cache key from `go.sum`, so both files are the single source as in `ci.yml`. No Node or pnpm step, the job does not touch the web app |
| 3 | Report versions: `docker version`, `docker compose version`, `go version` | A runner image change shows up in the log instead of as an unexplained failure |
| 4 | `cp .env.example .env` | `compose.yaml` uses `${POSTGRES_PASSWORD:?}`, so compose refuses to render without it. The three placeholders are already distinct, which is what finding 8 requires. No repository secret is used. `.env` is gitignored, so the tree stays clean |
| 5 | `docker compose build`, with `VERSION="${GITHUB_SHA:0:8}"` exported first (the shell value wins over `VERSION=dev` in `.env`) | See "Why build before up" below. The binary logs its version at startup (`services/api/cmd/api/main.go` line 82), so a failure dump names the commit it tested |
| 6 | `docker compose up -d --wait --wait-timeout 300` | Postgres, PgBouncer, api, workers. A migration failure makes the api container exit 1, which fails this step rather than reaching the test |
| 7 | Read `pool_mode` from the running pooler with `SHOW CONFIG` over `pgbouncer_admin`, fail unless it is `transaction` | See "Why check pool mode" below |
| 8 | Run the three variants (below) | The subject under test |
| 9 | Assert no variant skipped (below) | `go test` exits 0 when a test skips, so the assertion is the job's real gate |
| 10 | `if: failure()`: `docker compose ps -a` and `docker compose logs --no-color --tail 200` | A red job with no logs cannot be triaged |
| 11 | `if: always()`: `docker compose down -v` | Cheap on an ephemeral runner, and it keeps the job usable on a dev machine |

Steps 7 and 8 each source `.env` with `set -a` and `set +a` themselves. Job level `env` is
resolved before step 4 runs, so it cannot read a file the job creates.

Step 8, port discovery and the run. The ports are ephemeral, so they are read, not assumed,
the way `docker/compose_test.sh` lines 88-92 do:

```sh
pgb_port="$(docker compose port pgbouncer 6432 | sed 's/.*://')"
pg_port="$(docker compose port postgres 5432 | sed 's/.*://')"
app_url="postgres://${POSTGRES_APP_USER:-app}:${POSTGRES_APP_PASSWORD}@127.0.0.1:${pgb_port}/${POSTGRES_DB:-distrolearn}?sslmode=disable"
admin_url="postgres://${POSTGRES_USER:-distrolearn}:${POSTGRES_PASSWORD}@127.0.0.1:${pg_port}/${POSTGRES_DB:-distrolearn}?sslmode=disable"

cd services
TEST_APP_DSN="${app_url}" TEST_ADMIN_DSN="${admin_url}" \
	go test ./internal/db/... -count=1 -run TestTenantIsolation -v \
	>"${RUNNER_TEMP}/variants.log" 2>&1
cat "${RUNNER_TEMP}/variants.log"
```

Output goes to a file and is then printed, rather than piped into `tee`. The default
`run:` shell on Linux is `bash -e` with no `pipefail`, so a `tee` pipeline would report the
exit status of `tee` and swallow a failing `go test`. No `-count=2` and no retry: ADR 0006
line 50 calls a failure here a release blocker, not a flake.

Step 9, the assertion, which is the part that makes a skip red:

```sh
log="${RUNNER_TEMP}/variants.log"
for variant in SequentialSameConnection InterleavedTransactions PoolCappedAtOneConnection; do
	if ! grep -q -- "--- PASS: TestTenantIsolation_${variant} (" "${log}"; then
		echo "::error::TestTenantIsolation_${variant} did not pass, it skipped or failed"
		exit 1
	fi
done
if grep -q -- '--- SKIP:' "${log}"; then
	echo "::error::a variant skipped, so tenant isolation was not tested"
	exit 1
fi
```

The names are the three in `services/internal/db/tenant_isolation_test.go` lines 192, 220,
266, which are also the only tests in the package. The annotation names the variant, so the
failure says which one went missing rather than only that something did.

### Why build before up

`services/migrations/embed.go` embeds the SQL with `//go:embed *.sql` (ADR 0005), and
`services/Dockerfile` line 22 copies all of `services/` into the `deps` stage, so a new
migration file changes the layer cache key and the rebuilt binary carries it.

The hazard is `docs/reviews/017-recheck.md` finding A. After adding `00002_probe.sql`,
`docker compose up -d --wait` reused the old image, goose stayed at version 1, and every
service reported healthy. Nothing warned.

On a GitHub-hosted runner the Docker daemon starts empty, so `up` builds the missing
images on its own. The explicit step is still the right shape, for the reason measured
under Tests: it names the step that failed rather than leaving `up` to report a compiler
error under a step called `Bring up the stack`.
`docker/compose_test.sh` still relies on the implicit build. That is finding A against the
script, it stays open, and `README.md` lines 78-82 already tell a developer to use
`--build`.

### Why check pool mode

ADR 0006 decides transaction mode. A session-mode pooler would still let all three variants
pass, because `SET LOCAL` resets at commit either way, so the job would go green without
testing the decision the ADR made. One `SHOW CONFIG` read closes that.

The rest of `compose_test.sh` stays local. Its Postgres patch version check turns CI red
when the image moves to 18.7, and its admin credential and web health checks are stack
facts rather than ADR 0006 facts. Running the script in CI would inherit all three, which
is why the job is not `bash docker/compose_test.sh`.

## Tests

A workflow is its own test (`docs/plans/014-ci-github-actions.md` line 36). The gate is the
skip assertion in step 9, and it gets proved the way ticket 014 proved its red run.

**Probe 1, required. A forced skip turns the job red.**

1. On `fix/021-skip-probe`, branched from the PR branch, add `t.Skip("probe")` as the first
   statement of `TestTenantIsolation_InterleavedTransactions`.
2. Push and run the compose workflow on that branch with `workflow_dispatch`.
3. Expect the job red, with the go test step green (a skip is exit 0) and the assertion step
   failing on `::error::TestTenantIsolation_InterleavedTransactions did not pass`. That shape
   is the proof. A red run that fails inside `go test` would show the job is red for an
   unrelated reason.
4. Paste the run URL into the PR body, then delete the branch locally and on origin.

**Probe 2, removed.** An earlier draft made the build step the subject of a probe, on the
claim that a compile error would surface as an `up --wait` timeout instead of compiler
output. Measured on Compose v2.38.2, that is not what happens. With no build step, `up`
builds inline and fails with the same compiler output, in 173s against a 300s wait timeout,
before it creates a container. There is no timeout to avoid, and no stale image to inherit,
since a hosted runner's daemon starts empty.

What the explicit step buys is the name of the step that went red: `Build the service
images` rather than `Bring up the stack`. That is worth having in a log and not worth a
probe, so the probe goes. The step stays for the name and so the job does not depend on
`up` building implicitly.

Finding A stays open for `docker/compose_test.sh` and for a developer laptop, where the
daemon is warm. On this machine, `docker compose up -d --wait` against an existing
`distrolearn-api:dev` image came up green at exit 0 with a deliberate syntax error in
`services/internal/db/db.go` still in the tree.

**Probe 3, optional. The variants still bite.** Swap
`set_config($1, $2, true)` for `false` in `db.SetTenant` and dispatch. The job goes red at
step 8 with all three variants failing. QA reproduced this twice
(`docs/reviews/017-compose.md` lines 53-63, `docs/reviews/017-recheck.md` lines 149-158),
so it is confirmation rather than discovery.

## Acceptance criteria

- Every push to `main` and every pull request runs a check named `tenant isolation (compose)`.
- `docker compose build` runs as its own step, before `up`, in that order. A build failure
  fails the run before any container starts.
- The stack reaches healthy with `up -d --wait --wait-timeout 300` and no manual sleep.
- The app DSN points at the published PgBouncer port and the admin DSN at the published
  Postgres port, both discovered with `docker compose port`.
- All three variants run. The run is red when any of them is skipped, proved by probe 1.
- The run is red when `pool_mode` is not `transaction`.
- No variant is retried and no failure is tolerated.
- Every `uses:` line is a 40 character commit SHA with the tag in a trailing comment.
  Re-derive each SHA against the GitHub refs API at implementation time and record the
  resolution in the PR body, rather than copying one from another workflow.
  `docs/reviews/014-ci.md` lines 44-49 says why: `pnpm/action-setup` v6.1.0 is an annotated
  tag, and pinning the tag object produces an action that does not run.
- No repository secret is referenced, no `write` permission appears, and `permissions` is
  top level `contents: read`.
- `ci.yml`, `docker/compose_test.sh`, and everything under `services/` are unchanged.
- README says where the ADR 0006 variants run.

Making `tenant isolation (compose)` a required status check is a repository setting, not a
file in this diff. Name it in the PR body so whoever merges can turn it on.

## Decisions already covered by accepted ADRs

- Running the ADR 0006 test in CI against a real Postgres and a real PgBouncer: ADR 0006
  lines 50-53. No new ADR.
- A failing variant blocks the release rather than being retried: ADR 0006 line 50 and
  `docs/plans/018-database.md` line 21. Hence no retry step.
- Migrations run at service startup from files embedded in the binary: ADR 0005. The build
  step follows from the embed, it is not a new decision.
- Compose carries PgBouncer in transaction mode and the services point through it: ADR 0006,
  with the contradiction in `docs/plans/017-local-compose.md` resolved in the 017 fix commit.

## Not covered by an accepted ADR

Runner choice, the credential rule, and the pinning rule are conventions from tickets 014
and 019 rather than decisions on record. `docs/adr/0007-ci-execution-environment.md`
proposes them: GitHub-hosted `ubuntu-24.04` with no self-hosted runner, placeholders from
`.env.example` instead of repository secrets, every action pinned to a commit SHA, and no
cross-run image cache. Its status is proposed, so a reviewer can change any of the four
without reopening this ticket.

## Out of scope

Finding 6, RLS on `tenants`, is unchanged. Region routing, NATS, and production deployment
were not looked at.