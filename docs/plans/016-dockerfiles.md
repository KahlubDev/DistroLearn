# 016: Dockerfiles for web, api, workers

Three images from one repository, one per deployable.

- Branch: `feature/016-dockerfiles`
- ADR dependency: none. Proceeds now.

## Read first

`apps/web/package.json`, `apps/web/next.config.ts`, `services/go.mod`,
`services/api/cmd/api/main.go`, `services/workers/cmd/workers/main.go`, `Makefile`,
`.gitignore`.

## Change

`services/Dockerfile` (new), `apps/web/Dockerfile` (new), `.dockerignore` (new).

**Go, one Dockerfile, two targets.** `api` and `workers` are `cmd` packages in one module,
so they share a build stage and differ only in the `go build` path. A builder stage on
`golang:1.27.1` runs `CGO_ENABLED=0 go build -trimpath`, and the runtime stage copies one
binary onto `gcr.io/distroless/static`. Distroless has no shell and no package manager, so
nothing to harden at the container level. Build with `-ldflags "-s -w -X main.version=..."`
so the running binary reports which commit it is.

**Web, standalone.** Next.js 16 needs a `node:24.21.0` runtime and `.next/standalone`
output. Set `output: "standalone"` in `next.config.ts`, since without it there is no
`.next/standalone` directory to copy. Dependencies install in a `deps` stage using
`--frozen-lockfile` so the image cannot drift from `pnpm-lock.yaml`. Copy `.next/static`
and `public/` alongside the standalone output, which Next.js does not fold in itself. Run
as the `node` user already in the base image.

The web image is a placeholder for CI at this stage. Do not add a healthcheck to it; the
API and workers own probes for now.

`.dockerignore` covers `node_modules`, `.next`, `.git`, and `services/bin`, so build
context does not carry a local tree into the daemon.

## Tests to add

`docker/build_test.sh`, run by hand rather than from `make check`, since Docker is not
available in every environment and this ticket should not gate a local run:

- All three images build from a clean tree.
- `docker run --rm <api-image> /api` exits 0 and logs the startup line.
- `docker run --rm <workers-image> /workers` exits 0 and logs the startup line.
- `docker run --rm --entrypoint id <each-image>` reports a non-root user: uid 65532 for
  the Go images, the `node` user for web.
- Web image serves the page over HTTP 200 with a body containing `DistroLearn`.
- Image sizes are recorded in the PR description, so the next ticket has a baseline.

## Acceptance criteria

- `api` and `workers` build from one Dockerfile and produce two images.
- All images build from a clean checkout with no local `node_modules` or `.next`.
- Go images contain one static binary on distroless, run as non-root, and hold no shell
  or package manager.
- Web image runs as a non-root user and serves the page.
- No image copies `.git` or a host `node_modules`.
- Pinned base images, no `latest` tag.
- `make check` still passes. Adding Dockerfiles must not disturb the Go or web builds.