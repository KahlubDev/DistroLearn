# 0002: MVP scope decisions

- Status: accepted
- Date: 2026-10-08
- Supersedes: nothing
- Related: `0001-stack.md`, `docs/architecture/system.md`

## Context

Three questions were open across `docs/product/mvp.md` and
`docs/research/002-lab-isolation.md`: whether 5k concurrent labs is a peak or a
sustained figure, how far EU residency reaches in MVP, and where verification checks
run. They blocked roadmap sizing (E6), the EU residency epic (E9), and the
verification engine (E7).

## Decision

**5k concurrent labs is a peak figure.** Capacity is sized for 5k in E6, not
continuously. A sustained figure would mean buying capacity for the whole day, which
the product does not need at launch.

**MVP ships single-region with a `region` field on tenants from day one.** The field
records which region a tenant belongs to and defaults to the launch region. No
routing, no second cluster, no cross-region failover in MVP. A second EU cluster is
post-MVP work, and adding the field now means the migration is a data change rather
than a schema change.

**Verification runs in a runner pod outside the learner's lab.** The runner is a
separate pod in the verification namespace. It never executes learner-controlled code
inside the control plane, and it never runs inside the learner's lab namespace. The
runner reaches lab state over an explicit, narrow network path.

## Consequences

- The vision measure becomes "5k peak" rather than "5k peak, sustained". Sizing moves
  into E6 where it belongs.
- The single-cluster, single-chart constraint from ADR 0001 holds for MVP. What
  remains open is the second cluster itself, which is post-MVP.
- The runner pod needs egress into lab namespaces to assert on real system state.
  That is the one permitted exception to default-deny, and it needs its own
  NetworkPolicy plus a read-only Kubernetes role scoped to lab namespaces.
- A compromised lab can reach the runner. The runner holds no credentials of its own
  and returns pass or fail only, so the reachable blast radius is one check.
- Writing `region` on tenants before it routes anything costs one nullable column. The
  alternative, retrofitting it when the second cluster lands, means backfilling every
  tenant row and auditing any query that forgot to filter on it.

## Alternatives considered

| Question | Rejected | Reason |
|---|---|---|
| 5k as peak or sustained | Size for sustained 5k | Buys capacity for the whole day at launch |
| Residency | Two clusters at launch | Doubles fixed cost before the second region has tenants |
| Residency | Decide the column later | Backfill plus an audit of every query that omitted the filter |
| Verification | Sidecar inside the lab namespace | A compromised lab reaches its own verifier |
| Verification | Runner inside the control plane | Executes learner-controlled code in the trusted path |

## Revisit triggers

Revisit when a second region is commissioned (post-MVP), when the runner needs write
access to lab state rather than read-only probes, or when measured concurrency exceeds
5k peak.
