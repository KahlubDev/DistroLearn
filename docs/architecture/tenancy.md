# Tenant Isolation

How a learner at Institution A can never observe or affect Institution B. Follows
`docs/research/001-stack.md` (PostgreSQL + RLS, Cognito) and
`docs/research/002-lab-isolation.md` (namespace per lab, default-deny egress).
Component relationships are in `docs/architecture/system.md`.

## The layers

Four independent mechanisms. None is sufficient alone; each is chosen so a defect in
one is contained by another.

1. **Identity** — Cognito knows who a person is and nothing about which institution
   they belong to.
2. **Membership and role** — our own tables map user to institution with a role
   (admin, instructor, learner), enforced by the API on every request.
3. **Data** — PostgreSQL row-level security keyed on tenant, so a forgotten tenant
   filter returns nothing instead of another institution's rows.
4. **Network** — one namespace per lab with default-deny in both directions, so two
   learners' labs cannot reach each other even given each other's pod IPs, and the lab
   pool cannot reach the product's own services.

The important property is that layers 3 and 4 do not depend on layer 2 being correct.
A bug in a handler that forgets a tenant check still hits RLS and still returns no
rows.

## Identity and membership

Cognito is deliberately not the source of truth for tenancy. Per 001, the backend
verifies a standard OIDC/JWT and keeps its own user and tenant records in PostgreSQL,
so no business identifier lives in Cognito and swapping identity providers later is a
configuration change rather than a data migration.

Every API request resolves in this order:

1. Verify the Cognito token. Reject on failure, before any data access.
2. Load the caller's institution memberships and role from PostgreSQL.
3. Apply the role check for the specific operation.
4. Attach the tenant to the query. RLS then enforces it independently.

An authenticated user with no membership in the requested tenant is a 404, not a 403.
Returning 403 confirms that the resource exists.

## Data isolation in PostgreSQL

Every table that holds tenant data carries `tenant_id`. RLS policies are set so that a
query missing its tenant filter returns the empty set rather than a broader result.

The API connects as a role that is subject to RLS. It does not connect as a role that
bypasses it, and no code path sets a session variable that turns policies off. RLS is
the backstop for a missed `WHERE` clause, not the primary access-control mechanism, and
that distinction matters: if it were the primary mechanism, a policy bug would be a
total isolation failure rather than a degraded one.

Role model, per 002 and mvp.md:

| Role | Can do |
|---|---|
| Admin | Manage their institution's roster and settings. No cross-institution access. |
| Instructor | See cohort progress for their own institution. |
| Learner | Start labs, work in them, see their own progress. |

Institution is the tenant boundary. A user may hold memberships in more than one
institution; each membership carries its own role, and authorization is evaluated
against the membership for the institution owning the resource.

## Lab isolation

Per 002, every lab is its own namespace, on a dedicated lab node pool, with a hard TTL
and nothing shared with another learner. Applied to each lab pod:

- `automountServiceAccountToken: false`, so a lab cannot read the Kubernetes API.
- Pod Security Standards `restricted` at admission: `runAsNonRoot: true`,
  `allowPrivilegeEscalation: false`, no privileged containers, `seccompProfile: RuntimeDefault`.
- No `hostNetwork`, `hostPID`, or `hostIPC`; the restricted profile disallows all three.
- Read-only root filesystem where the lab permits it, `emptyDir` for scratch.
- `runtimeClassName: gVisor`, so the lab's syscalls are intercepted rather than
  reaching the host kernel directly.
- Resource requests and limits, plus a per-node concurrency cap.

Default-deny egress, with DNS explicitly re-allowed because a bare default-deny also
blocks DNS and every lab would fail silently. Package mirrors are allow-listed.
Anything needing the public internet goes through an operated proxy, so egress stays
observable and rate-limited. Where a destination is a hostname, Cilium L7/FQDN policy is
preferred over IP blocks.

## Terminal and service access

The browser never holds a kubeconfig, a Kubernetes API token, or an SSH key. Per 002,
it receives a single-use WebSocket ticket scoped to one lab with a ~30s TTL; the API
spends that ticket and opens the PTY with its own ServiceAccount. Authorization
checks the lab record belongs to the caller's tenant and to their live session.

Lab service endpoints are proxied through the API under the same check rather than
exposed directly, so a learner cannot reach a lab service by guessing a Service name or
pod IP from inside their own namespace.

## What is not covered

Stated plainly so these are not mistaken for solved.

- **Shared infrastructure is the residual risk.** Node pools, image registries, and log
  pipelines are shared across tenants. 002 risk 7 names them as the likeliest leak path
  and they are not solved by namespaces or RLS.
- **Image pulls happen at the node**, outside lab NetworkPolicy. A lab able to influence
  what gets pulled is an egress path policy cannot see (002 risk 5).
- **Observability is unspecified.** Log pipelines are both a leak path and an audit
  requirement, and no stack has been chosen. See C5 in `system.md`.
- **EU residency scope is undecided.** mvp.md asks whether learner records only, or
  also lab artifacts and sandbox logs, must be in-region. That directly changes this
  design: if logs and lab artifacts count, they need per-region storage and the
  residency boundary moves.
- **Abuse within a tenant is out of scope here.** Concurrency caps and cost alerting
  are required (002 risk 6), but per-tenant quotas are a product decision.
