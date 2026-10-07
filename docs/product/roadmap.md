# Roadmap

Ordered epics for the MVP described in `docs/product/mvp.md`. Each epic has one goal.
No dates: sizing depends on whether 5k concurrent labs is peak or sustained average,
which is still open (see C2 in `docs/architecture/system.md`).

Order is driven by dependency, not by perceived value. The lab runtime comes before
the learning surface because every MVP feature needs somewhere to run.

## Epics

**E1. Cluster and delivery foundation**
Stand up the cluster, node pools, Cilium, gVisor runtime class, Terraform, and the one
Helm chart with a working deploy path.

**E2. Lab runtime**
Provision a per-namespace gVisor sandbox to a usable shell in seconds, with hard TTL,
reaping, and teardown, so learners can be in a working environment.

**E3. Identity and tenancy**
Cognito sign-in, institutions, the three roles, and PostgreSQL RLS, so a learner
belongs to an institution before any lab exists.

**E4. Tenant-safe terminal**
Single-use WebSocket ticket relayed through the API, so a learner gets a terminal with
no cluster credential in the browser.

**E5. First lab end to end**
One distributed-systems topic shipped as a concept page plus a working lab plus one
automated verification check, proving the product loop before it is built at scale.

**E6. Lab scale and lifecycle hardening**
5k concurrent labs, per-tenant concurrency caps, cost alerting, and the gVisor boot-time
benchmark that validates the vision target.

**E7. Verification engine**
Generalize checks to assert on real system state across lab topologies, so completion
means demonstrated work rather than a submitted answer.

**E8. Progress tracking**
Per-learner records of started labs and verified objectives, visible to the learner and
to instructors at cohort level.

**E9. EU data residency**
Tenant-selectable EU region for tenant data, including the residency scope decision for
lab artifacts and sandbox logs.

**E10. Security and tenancy audit**
Independent review of auth, tenancy, and lab isolation before real student data enters
the system.

## Dependencies worth noting

- E5 depends on E2, E3, and E4. It is the first point at which the product can be
  judged, and it is deliberately early rather than last.
- E6 depends on E2, and its sizing depends on the peak-versus-sustained decision.
- E9 depends on the residency scope decision, which changes E2's storage design if lab
  artifacts and logs must be in-region.
- E10 is placed last but must run before launch. If its findings require rework, E6 and
  E9 are where the rework lands.

## Not in the MVP

The exclusions in `docs/product/mvp.md` are binding. No epic above delivers billing,
certification, forums, live sessions, instructor-authored labs, native apps, offline
mode, grading, SSO/SCIM, self-hosting, or extra regions. A pull request that adds one
of these is out of scope regardless of how small it looks.

## Blocked on decisions

Three open items gate parts of this roadmap, and none is an engineering task:

1. Peak versus sustained for 5k concurrent labs. Gates E6 sizing.
2. Residency scope for lab artifacts and sandbox logs. Gates E9, and may force E2
   changes.
3. Where verification runs and with what privileges. Gates E7, and shapes the lab
   network policy.
