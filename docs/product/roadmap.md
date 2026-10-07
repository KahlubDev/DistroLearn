# Roadmap

Ordered epics for the MVP in `docs/product/mvp.md`. No dates: sizing lands in E6
(ADR 0002).

## Epics

**E1. Cluster and delivery foundation**
Cluster, node pools, Cilium, gVisor runtime class, Terraform, and the one Helm chart.

**E2. Lab runtime**
Per-namespace gVisor sandbox to a usable shell in seconds, with hard TTL, reaping, and
teardown.

**E3. Identity and tenancy**
Cognito sign-in, institutions, three roles, PostgreSQL RLS, and the `region` field on
tenants.

**E4. Tenant-safe terminal**
Single-use WebSocket ticket relayed through the API, so no cluster credential reaches
the browser.

**E5. First lab end to end**
One topic as a concept page, a working lab, and one verification check. First point the
product can be judged.

**E6. Peak scale and lifecycle hardening**
Size for 5k concurrent labs as a peak, per-tenant caps, cost alerting, and the gVisor
boot-time benchmark against p50 under 10s.

**E7. Verification runner**
Checks assert on real system state across lab topologies, running from a runner pod
outside the learner's lab.

**E8. Progress tracking**
Per-learner records of started labs and verified objectives, visible to the learner and
to instructors at cohort level.

**E9. EU data residency**
Second EU region behind the existing `region` field. Post-MVP under ADR 0002. Residency
scope for lab artifacts and logs is decided first.

**E10. Security and tenancy audit**
Independent review of auth, tenancy, and lab isolation before real student data enters.

## Dependencies

- E5 needs E2, E3, E4.
- E6 needs E2.
- E9 changes E2's storage design if lab artifacts and logs must be in-region.
- E10 runs before launch. Findings land in E6 and E9.

## Not in the MVP

Exclusions in `docs/product/mvp.md` are binding: no billing, certification, forums,
live sessions, instructor-authored labs, native apps, offline mode, grading, SSO/SCIM,
self-hosting, or a second region.

## Blocked on decisions

One. No observability stack is chosen. It gates no epic, but log pipelines are a named
tenancy leak path, so E10 will find it.
