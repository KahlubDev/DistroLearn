# 011: Decision status

State of `docs/adr/` and the architecture docs as of 2026-10-09, read ahead of Phase 3
planning. Every layer in ADR 0001 has a choice. The gaps sit underneath those choices.

## ADRs by status

| Status | ADRs |
|---|---|
| Accepted | 0001 (application stack), 0002 (MVP scope) |
| Proposed | none |
| Superseded | none |

Both are dated 2026-10-08. Neither records a supersedes relation.

## Decisions still missing

**Database.** ADR 0001 names PostgreSQL 18.6, managed, but never names the managed
provider. `001-stack.md` links the RDS release calendar, which implies AWS RDS without
saying so. Also undecided: the migration tool, the connection pooler, and how RLS reads
the current tenant. `tenancy.md` says the tenant is attached to the query and RLS
enforces it independently, without naming the mechanism. That choice decides whether a
connection left open between requests can read another tenant's rows.

**Auth provider.** Cognito with email and password is settled. Undecided: hosted UI
against the self-hosted UI SDK, and whether the API validates JWTs against the JWKS
endpoint or calls Cognito per request. Undecided: how a user holding memberships in two
institutions picks one at sign-in.

**Tenancy model.** Roles, RLS on `tenant_id`, and 404 rather than 403 for a non-member
are settled, but they live in `docs/architecture/tenancy.md`, which carries no status
and no revisit triggers. They read as decisions without an ADR. Undecided in writing:
shared schema with RLS against schema per tenant. RLS implies shared schema, so this
needs recording.

**Lab runtime.** gVisor is settled. Undecided: the image a lab runs, and whether lab
definitions live in a column on the lab record or in a custom resource.

**Region routing.** ADR 0002 settles MVP as single-region, with a `region` column that
routes nothing. Phase 9 of the plan asks for region-aware storage and routing, which
ADR 0002 defers to post-MVP. Also undecided: the launch region among eu-central-1,
eu-west-1, and eu-north-1, and whether lab artifacts and logs must sit in-region
(roadmap E9 defers this).

**Observability.** No logging, metrics, or tracing stack is chosen. `system.md` names
this as a security gap, and `roadmap.md` lists it as the one blocked decision.

## Open risks

1. gVisor overhead is unbenchmarked against the committed p50 under 10s boot target.
2. Lab compatibility with gVisor. A lab running a real service can fail to start, which
   invalidates the lab design rather than slowing it.
3. The API is a single interactive bottleneck. Every terminal byte and proxied service
   call crosses it and the connection ceiling is unmeasured.
4. Image pulls happen at the node, outside lab NetworkPolicy, so anything that influences
   a pull is an egress path policy cannot see.
5. Labs as a compute or attack platform. Egress limits do not stop resource use, and
   `tenancy.md` calls per-tenant quotas a product decision nobody has made.
6. Tenancy leaks through shared infrastructure: node pools, image registries, log
   pipelines. The observability gap sits on this one.
7. Kata node virtualization is unverified against our provider. Non-blocking under
   gVisor, blocking for any promise of VM-grade isolation.
8. Cognito constrains cloud choice and where EU regions are available.

## Questions for a human

1. Managed Postgres: RDS or something else. ADR 0001 says managed only.
2. How does RLS learn the current tenant: `SET LOCAL` per transaction, or one role per
   tenant?
3. Phase 9 and ADR 0002 disagree on region routing. Which wins?