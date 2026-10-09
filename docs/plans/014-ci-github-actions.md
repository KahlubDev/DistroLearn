# 014: CI with GitHub Actions

GitHub Actions runs `make check` on every push and pull request.

- Branch: `feature/014-ci-github-actions`
- ADR dependency: none. Proceeds now.

## Read first

`Makefile`, `package.json`, `apps/web/package.json`, `services/go.mod`, `.nvmrc`,
`pnpm-workspace.yaml`.

## Change

`.github/workflows/ci.yml` (new).

One job on `ubuntu-24.04` (pinned 2026-10-09), triggered on `push` to `main` and on
`pull_request`.

- Node 24.21.0 from `.nvmrc`, pnpm 12.10.1 via `corepack enable` so the version comes
  from `packageManager` rather than a second hardcoded number.
- Go 1.27.1, matching `services/go.mod`.
- `pnpm install --frozen-lockfile`.
- `make check`, the single entry point. No step reimplements a target, so CI and local
  runs cannot drift.

Do not add a separate `gofmt` or `go vet` step. `make lint` covers both, and the
`test -z "$(gofmt -l .)"` fix from `docs/reviews/010-scaffold.md` already makes an
unformatted file fail the build.

`workflow_dispatch` stays because the red-run probe depended on it: verifying the gofmt
failure needed a run against a branch with no pull request, which no other trigger allows.

## Tests to add

None. A workflow is its own test. Its correctness shows up as a red build.

Manual proof before opening the PR: push the branch and confirm the run goes green, then
add a deliberately misformatted Go file on a scratch branch and confirm the same workflow
goes red. That second run is the regression test for the finding in `010-scaffold.md`.

## Acceptance criteria

- `make check` runs on every push to `main` and every pull request.
- Node, pnpm, and Go versions are read from `.nvmrc`, `packageManager`, and `go.mod`.
- `pnpm install --frozen-lockfile` succeeds with the committed lockfile, unmodified.
- A misformatted Go file fails the run.
- A failing `pnpm lint`, `tsc --noEmit`, `vitest`, or `go test` fails the run.
- No duplicate lint or test logic outside `Makefile`.
- README gains a CI status badge.