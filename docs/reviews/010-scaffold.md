# 010: Scaffold review

QA review of PR #1 (`feature/010-scaffold`) against the Definition of Done in
`AGENTS.md`. Nothing in PR #1 was modified.

Cloned fresh from GitHub, checked out `feature/010-scaffold` at `0dade00`, installed
with `--frozen-lockfile`. Host had Node 24.20.0, so 24.21.0 was installed via nvm to
match `.nvmrc`. pnpm 12.10.1 came from corepack. Go 1.27.1 was present.

## Definition of Done

| Criterion | Result | Evidence |
|---|---|---|
| Code compiles (`make build`) | pass | `next build` compiled, 2 static routes; `go build ./...` clean |
| Tests pass (`make test`) | pass | vitest 1/1, `go test ./...` ok for 2 packages |
| New tests exist for the change | pass | `apps/web/src/__tests__/product-name.test.ts`, `services/api/cmd/api/main_test.go`, `services/workers/cmd/workers/main_test.go` |
| Lint and typecheck pass (`make lint`) | **fail** | See finding below. Passes on a clean tree, accepts unformatted Go |
| No unrelated changes in the diff | pass | 32 files, all skeleton |
| Docs updated where behaviour changed | pass | `AGENTS.md` gained Ticket workflow and Definition of Done |

`make check` exits 0 on a clean checkout, so the suite is green as merged. The lint
criterion fails on a defect the suite does not catch.

## Finding 1: `make lint` reports unformatted Go but passes

`Makefile:6` runs `cd services && gofmt -l . && go vet ./...`. `gofmt -l` prints
filenames and exits 0 whether or not anything is unformatted, so `&&` passes straight
through to `go vet`.

Reproduced by dropping a deliberately misformatted Go file into
`services/api/cmd/api/` on the review clone:

```
api/cmd/api/gofmt_probe.go
make lint    exit 0
make check   exit 0
```

The filename is printed to the console and ignored. A misformatted Go file reaches main
without failing a check. CI added in Phase 3 would inherit this, since it runs
`make check`.

Scope is Go only. `pnpm lint` and `tsc --noEmit` exit non-zero on a real problem, so
the web side is unaffected.

Fix is one line, `gofmt -l . | tee /dev/stderr | (! read)` or `test -z "$(gofmt -l .)"`.
Out of scope here since QA does not fix. Phase 4 or the Phase 3 CI plan should take it
as a ticket before CI lands.

## Pinned versions

Every pin in the PR table matches what installed from the lockfile. Checked by reading
each installed `package.json` against the declared range.

| Package | Pinned | Installed | Result |
|---|---|---|---|
| Node | 24.21.0 | v24.21.0 | match |
| pnpm | 12.10.1 | 12.10.1 | match |
| next | 16.4.0 | 16.4.0 | match |
| react / react-dom | 19.3.0 | 19.3.0 | match |
| typescript | 6.0.3 | 6.0.3 | match |
| eslint | 9.39.5 | 9.39.5 | match |
| vitest | 5.0.3 | 5.0.3 | match |
| Go | 1.27.1 | go1.27.1 | match |
| eslint-config-next | 16.4.0 | 16.4.0 | match |

Also installed and matching: `@types/node` 26.6.4, `@types/react` 19.3.0,
`@types/react-dom` 19.3.0, `@vitejs/plugin-react` 6.1.2, `jsdom` 30.1.2.

`pnpm peers check` reported no peer dependency issues. `git status` was empty after the
full run, so the build leaves no drift in the tree.

## The two deliberate downgrades

Both are real and correctly explained. Each cause was confirmed from the installed
package metadata rather than taken on trust.

**TypeScript 6.0.3, not 7.0.2.** `eslint-config-next` 16.4.0 depends on
`typescript-eslint` `^8.56.0`, which resolved to 8.71.1. Its peer range is
`>=4.8.4 <6.1.0`, and `@typescript-eslint/parser` 8.71.1 declares the same. 7.0.2 is
outside it, so 6.0.3 is the newest stable the toolchain accepts.

Revisit when `typescript-eslint` 9 ships with a peer range covering TypeScript 7, or
when `eslint-config-next` moves off `typescript-eslint` 8. Until one of those happens,
any TypeScript upgrade fails lint before it reaches the compiler.

**ESLint 9.39.5, not 10.12.0.** Confirmed from `eslint-config-next` 16.4.0
dependencies, which pull in three plugins that cap at ESLint 9:

| Plugin | Resolved | Peer range on eslint |
|---|---|---|
| `eslint-plugin-import` | 2.32.0 | `^2 \|\| ... \|\| ^9` |
| `eslint-plugin-jsx-a11y` | 6.10.2 | `^3 \|\| ... \|\| ^9` |
| `eslint-plugin-react` | 7.37.5 | `^3 \|\| ... \|\| ^9.7` |

Note that `typescript-eslint` 8.71.1 itself allows `^10.0.0` on ESLint. The blocker is
the three plugins only, all of them transitive from `eslint-config-next`, none a direct
dependency of this repo. That means nothing in `apps/web/package.json` has to change to
unblock ESLint 10, only the Next.js config.

Revisit when `eslint-config-next` 16.5.0 or later updates those plugin peer ranges.
Re-check before the first production build, since these are point-in-time pins.

## Other checks

- `gofmt -l .` empty, `go vet ./...` clean on the unmodified tree.
- `pnpm install --frozen-lockfile` passed, 482 lockfile entries.
- `next.config.ts` sets `agentRules: false`. Confirmed no stray `AGENTS.md` is generated
  in `apps/web` after a build, so the working tree stays clean.
- One Go module at `services/`, two `cmd` packages, `go.mod` declares `go 1.27.1`.
- No health endpoints, Dockerfiles, Helm, CI, database, or auth, matching the ticket
  scope.

## Verdict

Five of six Definition of Done criteria pass. One fails: `make lint` accepts
unformatted Go.

Merging is a human decision and the recommendation is to fix finding 1 first, since
Phase 3 CI inherits it. The fix is small and the scaffold itself is sound.