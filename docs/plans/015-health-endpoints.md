# 015: Health endpoints

`/healthz` and `/readyz` on the API and workers, so a Kubernetes probe and the operator
can tell a starting service from a stuck one.

- Branch: `feature/015-health-endpoints`
- ADR dependency: none. Proceeds now.

## Read first

`services/api/cmd/api/main.go`, `services/api/cmd/api/main_test.go`, the matching
`main.go` and `main_test.go` under `services/workers/`, `services/go.mod`.

## Change

`services/internal/health/health.go` (new), plus both `main.go` files.

One package shared by the two services, since they answer the same question.

- `GET /healthz`. Liveness. 200 while the process is up. No dependencies, no I/O. If it
  touches the database it stops meaning "not dead" and starts meaning "database is
  reachable", and a database blip restarts every replica.
- `GET /readyz`. Readiness. 200 when the service can do useful work, 503 otherwise.
  Workers check the NATS connection, since an unconnected consumer accepts nothing. The
  API's readiness starts as process-and-config only; it gains a database check in Phase 5
  once migrations and the pooler exist, and pretending otherwise would be a probe that
  passes before the service can serve.

Both return JSON: `{"status":"ok"}`. Keep it one field. Consumers are probes.

The handlers take a `ready func(context.Context) error`, so each service supplies its own
without the package importing anything service-specific.

## Built differently from the plan, 2026-10-09

Two decisions taken while building, recorded here so the plan matches the code.

**A 503 returns `{"status":"unavailable"}`, not `{"status":"ok"}`.** The shape stays one
field. The text differs because a 503 whose body says ok is something an operator trusts
and is wrong about, and probe bodies end up pasted into tickets.

**`run()` blocks until its context is cancelled.** The skeleton returned nil straight
away, so `main()` logged "stopped cleanly" and exited. A service that answers probes has
to stay up. `run(ctx)` serves until cancelled, then shuts down within a 10s grace period,
and a shutdown that overruns is reported rather than swallowed.

## Signals

`main()` wraps its context in `signal.NotifyContext` for SIGTERM and SIGINT. SIGTERM is
what Kubernetes sends on pod deletion; without handling it, in-flight terminal and lab
teardown traffic is cut mid-stream rather than drained. `srv.Shutdown` bounds the drain by
`shutdownTimeout` (10s), below a typical pod termination grace period, so a stuck handler
cannot hold the process open.

## Tests to add

`services/internal/health/health_test.go`:

- `/healthz` returns 200 with no dependency configured.
- `/readyz` returns 200 when the check returns nil.
- `/readyz` returns 503 when the check returns an error.
- `/readyz` returns 503 when the context is cancelled, so a hung dependency cannot hang
  the probe past the probe timeout.

In both services' `main_test.go`:

- Each service mounts its routes and answers `/healthz` with 200. This is what catches a
  service that builds but never registers the handler.
- `stopSignals` contains SIGTERM and SIGINT. Cancelling the context stops `serve()` and
  returns nil within the shutdown timeout.
- The server answers a real request while running, so the shutdown test is not passing
  against a server that never bound.

## Acceptance criteria

- Both services serve `/healthz` and `/readyz` on the same address as the rest of the
  service.
- `/healthz` performs no dependency I/O and returns 200 in a unit test with no
  dependencies running.
- `/readyz` returns 503 when a dependency check fails or the context is cancelled.
- Handlers accept any method; the router answers 404 for unknown paths.
- Tests fail if either `main.go` stops mounting the routes.
- Both services shut down within a bounded timeout on SIGTERM or SIGINT.
- `make check` passes.