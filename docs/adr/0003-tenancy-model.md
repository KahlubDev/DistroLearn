# ADR 0003: Tenancy model

- Status: accepted
- Date: 2026-10-09
- Source: `docs/architecture/tenancy.md`, answers in `docs/research/011-status.md`
- Related: `0001-stack.md`, `0002-mvp-scope.md`

## Context

`tenancy.md` describes tenant isolation in enough detail to build against, but carries no
status line and no revisit triggers, so a reader cannot tell whether it was decided or
drafted. It also omits the mechanism by which a row-level security policy learns which
tenant is active, which is the detail that decides whether a pooled connection can leak
one tenant's rows into another's request.

## Decision

Tenant isolation runs on four layers, each chosen so a defect in one is contained by
another.

1. **Identity.** Cognito knows who a person is, not which institution they belong to.
2. **Membership.** Repo tables map a user to an institution with a role, checked on every
   request.
3. **Data.** PostgreSQL row-level security on `tenant_id`.
4. **Network.** One namespace per lab, default-deny both directions.

Layers 3 and 4 do not depend on layer 2 being correct. A handler that skips a tenant check
still hits RLS and returns no rows.

**Shared schema, one application role.** Every tenant lives in the same tables, keyed by
`tenant_id`. RLS keys on `tenant_id`, never on `region`.

**The policy learns the tenant from `SET LOCAL app.tenant_id`, set inside every
transaction.** One application role, never one role per tenant. The setting is
transaction-scoped, so it resets at commit and a connection returned to the pool carries
no tenant. This matches the pooler mode in ADR 0005.

RLS is a backstop, not the primary access control. As a primary mechanism, a policy bug
would be a total isolation failure rather than a degraded one.

**Request resolution** in order: verify the Cognito token and reject before any data
access, load the caller's memberships, apply the role check for the operation, attach the
tenant to the query. A caller with no membership in the requested tenant gets 404, not
403, because a 403 confirms the resource exists.

**Roles.** Admin manages their institution's roster and settings. Instructor sees cohort
progress for their institution. Learner starts labs and sees their own progress. A user
may hold memberships in several institutions, each with its own role, and authorization
uses the membership for the institution owning the resource.

**Tenants carry a `region` field** set to the launch region for every MVP tenant
(ADR 0002). It routes nothing.

## Consequences

- Every request wraps its queries in a transaction. A query issued outside one has no
  tenant set, and RLS returns nothing rather than everything.
- RLS policies and the `SET LOCAL` call must stay in step. A query path that skips the
  transaction is a silent isolation failure, not a loud one, so tests cover it directly.
- Shared schema means one noisy tenant competes for the same indexes and IO. Per-tenant
  isolation of resources stays a product decision (`tenancy.md`, "Not covered").
- 404 rather than 403 needs a test per resource family, since it is a per-handler
  decision.

## Alternatives considered

| Option | Rejected | Reason |
|---|---|---|
| One database role per tenant | Thousands of tenants, thousands of roles, slow to provision |
| Schema per tenant | Migration fan-out grows with tenant count; RLS was chosen to avoid this |
| `SET` without `LOCAL` | Value survives on a pooled connection and leaks into the next request |
| RLS as primary access control | A policy bug becomes a total isolation failure, not a degraded one |
| 403 for a non-member | Confirms the resource exists to a caller with no access |

## Revisit triggers

Revisit when SSO or SCIM enters scope, when a tenant needs its own database or its own
resource isolation, when measured load makes shared-schema contention visible, or when
the second EU region lands and `region` starts routing.