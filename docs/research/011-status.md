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

**Database. Answered.** RDS for PostgreSQL, keeping it on AWS beside Cognito. The
instance class and failover mode are still open and need an ADR. `001-stack.md` already
links the RDS release calendar, so this names the service the ADR left implicit.

The RLS mechanism is answered too: `SET LOCAL app.tenant_id` inside every transaction,
one app role, never one role per tenant. The setting resets at commit, so a connection
returned to the pool carries no tenant. Every request wraps its queries in a
transaction.

Still open: the migration tool and the connection pooler.

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

**Region routing. Answered.** ADR 0002 stands. MVP stays single-region and the `region`
column routes nothing. Phase 9 narrows to region-aware tagging and the residency scope
for lab artifacts and logs inside the one region. Routing waits for a post-MVP ADR.

Still open: the launch region among eu-central-1, eu-west-1, and eu-north-1, and
whether lab artifacts and logs must sit in-region (roadmap E9 defers this).

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

All three are answered (database, RLS mechanism, Phase 9 scope) and recorded above.
Nothing else in this file blocks Phase 3 planning.

Two items need an ADR rather than a plan: the tenancy design in
`docs/architecture/tenancy.md`, which is written as settled but carries no status line,
and the observability gap named in `system.md` and `roadmap.md`.