# MVP

First shippable version. Decisions live in `docs/adr/`.

## In MVP

### 1. Sandbox labs

Ephemeral isolated environments, single-node and small multi-node. Boot to a usable
shell in seconds, terminal plus service endpoints, teardown on exit or timeout.

### 2. Concept pages paired with labs

Each topic has a written page and a linked lab. Reading and doing are one unit of work.

### 3. Automated work verification

Checks assert on real system state: `kill -9` a node and assert availability held, scale
a partition and assert quorum held, confirm a replication factor is 3. Checks run from a
runner pod outside the learner's lab (ADR 0002).

### 4. Progress tracking

Started labs, verified objectives, completion. Learners see their own; instructors see
cohort.

### 5. Multi-tenant institutions with a region field

Institution as tenant with isolated data. Admin, instructor, learner roles. Every tenant
carries a `region` field, set to the launch region for all MVP tenants (ADR 0002).

## NOT in MVP

- **Payments.** Sales and contracts handle it.
- **Certification.** No certificates, exam engine, or accreditation.
- **Forums and chat.**
- **Live sessions.** No synchronous classrooms or screen sharing.
- **Instructor-authored labs.** No lab builder. Content ships with the product.
- **Native mobile apps.** Responsive web only.
- **Offline mode.** Labs need a live sandbox.
- **Grading, attendance, LMS features.**
- **SSO and SCIM.** Email, password, admin-managed roster.
- **Self-hosting.** SaaS only.
- **A second region.** MVP is single-region. The `region` field makes the second EU
  cluster a data change, not a migration (ADR 0002).
- **Cohort analytics beyond completion.** No leaderboards or funnels.

## Resolved

Isolation: gVisor, one namespace per lab, default-deny egress
(`docs/research/002-lab-isolation.md`). Concurrency: 5k peak. Residency: single-region,
second cluster post-MVP. Verification: separate runner pod. All in ADR 0002.
