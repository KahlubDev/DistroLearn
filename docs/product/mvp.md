# DistroLearn MVP

Scope of the first shippable version. Target scale: 50k MAU, 5k concurrent labs,
multi-tenant by institution, EU data residency as a tenant option.

## In MVP (5 features)

### 1. Hands-on sandbox labs

Ephemeral, isolated lab environments (single-node and small multi-node topologies).
Boot to a usable shell in seconds, offer a terminal plus service endpoints, and tear
down automatically on exit or timeout. This is the core of the product; everything
else exists to get a learner into a lab and prove they did something.

### 2. Concept pages paired with labs

Each topic has a written concept page (targeted, not textbook-length) and a linked
lab. Reading and doing are the same unit of work.

### 3. Automated work verification

The lab checks the learner's actual system state, not a multiple-choice answer.
Examples: `kill -9` a node and assert the service stayed available; scale a
partition and assert quorum held; check a replication factor is actually 3.
Passing a check marks the objective complete.

### 4. Progress tracking

Per-learner record of started labs, verified objectives, and completion. Learners see
their own progress; instructors see cohort progress.

### 5. Multi-tenant institutions with EU data residency option

Institution as tenant with isolated data. Admin, instructor, and learner roles.
Tenant-selectable EU storage region for learner records and progress.

## NOT in MVP

Explicitly out of scope. Listed so nobody adds it as a "small extra".

- **Payments and billing.** No invoicing, no self-serve checkout, no per-seat pricing.
  Sales and contracts handle it.
- **Certification or credentialing.** No certificates, no exam engine, no accreditation.
- **Discussion forums and chat.** No per-lab comments, threads, or messaging.
- **Live or instructor-led sessions.** No synchronous classrooms, screen sharing, or
  scheduled cohorts.
- **Custom labs authored by instructors.** No lab builder or custom exercise authoring.
  Lab content ships with the product.
- **Native mobile apps.** Responsive web only.
- **Offline mode.** Learning requires a live sandbox.
- **Grading curve, attendance, or LMS features.** No weighted assignments, no proctoring.
- **SSO and SCIM.** Email plus password and a tenant admin-managed roster are enough
  for MVP. SAML/OIDC and directory sync are post-MVP.
- **Self-hosted or on-prem deployment.** SaaS only, including for institutions.
- **More than one non-EU region.** EU is the first non-US region; no additional
  geographic options in MVP.
- **Cohort analytics beyond completion.** No time-on-task leaderboards, drop-off
  funnels, or retention dashboards.

## Open questions

These need an answer before implementation planning. They are product decisions,
not for an implementer to settle.

- Sandbox isolation model and how far it is shared between concurrent labs.
- EU residency scope: learner records only, or lab artifacts and sandbox logs too.
- Whether 5k concurrent labs is sized on worst-case peak or sustained average.
