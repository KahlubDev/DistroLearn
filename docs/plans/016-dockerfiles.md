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

## Image size baseline

Measured 2026-10-09 with Docker 29.8.1 on linux/amd64, from `docker/build_test.sh`.
Sizes are uncompressed, from `docker image inspect .Size`.

| Image | Bytes | Approx |
|---|---:|---|
| `distrolearn-api` | 15,764,841 | 15 MB |
| `distrolearn-workers` | 15,764,835 | 15 MB |
| `distrolearn-web` | 1,694,995,729 | 1616 MB |

The two Go images are near identical, as expected from the same distroless base and the
same size of main package. The web image is 1565 MB of `node:24.21.0` base plus 41 MB of
application: `/app` itself measures 40 MB. The size is inherited from the full node image,
which carries a compiler and headers, not from anything this Dockerfile copies.

If the web image size matters, the fix is a runtime base without the build toolchain
(`node:24.21.0-slim` or an `alpine` variant) rather than changing what is copied. Left
alone here: no sizing target has been set, and slim alpine changes the libc the Next.js
native modules expect.

## Acceptance criteria

- `api` and `workers` build from one Dockerfile and produce two images.
- All images build from a clean checkout with no local `node_modules` or `.next`.
- Go images contain one static binary on distroless, run as non-root, and hold no shell
  or package manager.
- Web image runs as a non-root user and serves the page.
- No image copies `.git` or a host `node_modules`.
- Pinned base images, no `latest` tag.
- `make check` still passes. Adding Dockerfiles must not disturb the Go or web builds.

## Downstream requirements

**`terminationGracePeriodSeconds` must stay above the 10 second `shutdownTimeout` in both
services.** Both the API and the workers bound their graceful drain with a package
constant `shutdownTimeout = 10 * time.Second`. Kubernetes sends SIGTERM, waits for
`terminationGracePeriodSeconds`, then sends SIGKILL. If that grace period is 10 or less,
SIGKILL lands before the drain finishes and in-flight terminal and lab teardown traffic is
cut mid-stream, which is the failure the drain was added to prevent.

Set it to 30 or more in the Helm chart. Anything at or below 10 silently disables the
graceful shutdown that `services/api/cmd/api/main.go` and
`services/workers/cmd/workers/main.go` implement.

## Built differently from the plan, 2026-10-09

**Base images are pinned by digest, not version tag.** The plan says pin exactly. A
version tag is a mutable pointer, so `golang:1.27.1` can be repushed under the same name.
Digests are the only exact pin, and they are recorded with the registry that served them.

**The web image is built from the repository root, not `apps/web`.** This is a pnpm
workspace, so `pnpm install --frozen-lockfile` needs the root `pnpm-lock.yaml` and the
sibling package manifests. An `apps/web`-only context cannot satisfy it. `.dockerignore`
exists at the root, which is the context actually used, and also at `apps/web` for anyone
who builds that way.

**`apps/web/public/` was created.** Next.js does not create it, and the image copies it,
so the build would fail without it. Holds a `.gitkeep`.

**A `version` variable was added to both `main.go` files.** The plan asks for
`-X main.version=...`, which is silently ignored by the linker when no such variable
exists. It now exists, defaults to `dev`, and is logged at startup, so a running container
reports which commit it is.

## Defects found by running the build, 2026-10-09

Four, all in ticket 016 files, all found by `docker/build_test.sh` rather than by reading.
The first two would have shipped an image that did not work.

1. **Wrong build path, both Go images failed to build.** `COPY services/ ./` puts the
   module contents at the workdir root, so the binary path is `./api/cmd/api`, not
   `./cmd/api`. Every Go build failed with `stat /src/cmd/api: directory not found`.

2. **The workers image contained the api binary.** One shared compile stage had
   `ARG TARGET=api`, and `docker build --target workers` selects a later stage without
   setting an argument in an earlier one. Both images logged `api starting` and bound
   8080, so the workers image was useless and nothing caught it, since a healthy check on
   the wrong port still looked healthy. Split into `build-api` and `build-workers`, and
   the script now asserts each image logs its own service name.

3. **The non-root check used `id`, which distroless does not have.** The check reported
   failure for the correct images. Now reads `Config.User` from the image config for the
   Go images, and still runs `id` inside the web image where a shell exists.

4. **The health probes were unreliable.** Fixed sleeps raced a cold start, and the probe
   client was the 1.6 GB web image, which took about 12s per attempt and exhausted the
   retry budget. Now retries with backoff from a small curl container, and reports the
   status and body it actually saw. A `set -e` interaction was also fixed: a failed
   command substitution aborted the script instead of retrying.