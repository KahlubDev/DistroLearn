# Tenant Isolation

How a learner at Institution A cannot observe or affect Institution B. Follows
`docs/research/001-stack.md` and `002-lab-isolation.md`; see `system.md` for
components.

## Four layers

Each is chosen so a defect in one is contained by another.

1. **Identity.** Cognito knows who a person is, not which institution they belong to.
2. **Membership.** Repo tables map user to institution with a role, checked by the API
   on every request.
3. **Data.** PostgreSQL row-level security on `tenant_id`. A forgotten tenant filter
   returns nothing.
4. **Network.** One namespace per lab, default-deny both directions.

Layers 3 and 4 do not depend on layer 2 being right. A handler that skips a tenant
check still hits RLS and returns no rows.

## Request resolution

1. Verify the Cognito token. Reject before any data access.
2. Load the caller's memberships from PostgreSQL.
3. Apply the role check for the operation.
4. Attach the tenant to the query. RLS enforces it independently.

A caller with no membership in the requested tenant gets 404, not 403. A 403 confirms
the resource exists.

## PostgreSQL

The API connects as a role subject to RLS, never one that bypasses it, and no code path
disables policies. RLS backstops a missed `WHERE` clause rather than acting as primary
access control: as a primary mechanism, a policy bug would be a total isolation failure
instead of a degraded one.

| Role | Can do |
|---|---|
| Admin | Manage their institution's roster and settings |
| Instructor | See cohort progress for their institution |
| Learner | Start labs, work in them, see their progress |

A user may hold memberships in several institutions, each with its own role.
Authorization uses the membership for the institution owning the resource.

Tenants carry a `region` field, set to the launch region for every MVP tenant. It routes
nothing yet (ADR 0002). RLS keys on `tenant_id`, not `region`.

## Lab isolation

Per lab: own namespace, dedicated node pool, hard TTL, nothing shared. Per pod:

- `automountServiceAccountToken: false`, so the lab cannot read the Kubernetes API.
- PSS `restricted` at admission: `runAsNonRoot: true`,
  `allowPrivilegeEscalation: false`, no privileged containers,
  `seccompProfile: RuntimeDefault`. Same profile disallows `hostNetwork`, `hostPID`,
  `hostIPC`.
- Read-only root filesystem where the lab permits it, `emptyDir` for scratch.
- `runtimeClassName: gVisor`, intercepting syscalls before they reach the host kernel.
- Resource requests, limits, per-node concurrency cap.

Egress is default-deny with DNS explicitly re-allowed, because a bare default-deny also
blocks DNS and every lab would fail silently. Mirrors are allow-listed, public internet
goes through an operated proxy, hostname destinations use Cilium L7/FQDN policy.

## Terminal and service access

The browser gets a single-use WebSocket ticket scoped to one lab, ~30s TTL. The API
spends it and opens the PTY with its own ServiceAccount, after checking the lab record
belongs to the caller's tenant and live session.

Lab service endpoints are proxied through the API under the same check, so a learner
cannot reach one by guessing a Service name or pod IP.

The verification runner is the one component permitted to probe lab state from outside.
It runs in its own namespace, holds no credentials, and returns pass or fail only
(ADR 0002). A compromised lab can reach it, bounding the blast radius to one check.

## Not covered

- **Shared infrastructure is the residual risk.** Node pools, image registries, and log
  pipelines are shared across tenants, unsolved by namespaces or RLS (`002` risk 7).
- **Image pulls happen at the node**, outside lab NetworkPolicy, so a lab able to
  influence what gets pulled is an egress path policy cannot see (`002` risk 5).
- **Observability is unspecified.** See `system.md`.
- **Abuse within a tenant is out of scope.** Concurrency caps and cost alerting are
  required (`002` risk 6); per-tenant quotas are a product decision.
