# ADR 0004: Launch region

- Status: accepted
- Date: 2026-10-09
- Related: `0001-stack.md`, `0002-mvp-scope.md`, `docs/research/011-status.md`

## Context

`011-status.md` left the launch region open among eu-central-1 (Frankfurt), eu-west-1
(Ireland), and eu-north-1 (Stockholm). The region sets where tenant data lives, so it is
a residency decision before it is an infrastructure one. Frankfurt was selected and
approved.

Every AWS service named in ADR 0001 was checked against the AWS regional endpoints and
service quotas pages on 2026-10-09.

## Decision

**The launch region is eu-central-1 (Frankfurt).** ADR 0002 is unchanged: MVP is
single-region, the `region` field on tenants routes nothing, and the second EU cluster is
post-MVP work.

### Availability check

| Service | eu-central-1 | Note |
|---|---|---|
| Amazon Cognito user pools | available | `cognito-idp.eu-central-1.amazonaws.com` |
| Amazon RDS for PostgreSQL | available | 18.6 released 2026-08-25, standard support to September 2027 |
| Amazon EKS | available | including dual-stack and EKS Auth endpoints |
| Amazon ECR | available | |
| Amazon S3 | available | |

Frankfurt is the region EU institutions most often name in data processing agreements.
It carries the widest set of the services ADR 0001 already commits to. ADR 0001's
consequence about Cognito constraining cloud choice and EU region availability is
accepted as the cost of that.

## Gap, resolved 2026-10-09: managed NATS was not available in Frankfurt

ADR 0001 names NATS JetStream and `001-stack.md` says to prefer a managed offering.
There is no first-party AWS NATS service. The managed option is Synadia Cloud, whose
published region list on 2026-10-09 was ap-east-2, aws-euwest-1, aws-useast-2, and
aws-uswest-2. **eu-central-1 was not on it.**

**Decision: self-host NATS JetStream in the eu-central-1 cluster.** ADR 0001 carries a
dated change note recording the move from managed to self-hosted.

The bus carries tenant identifiers, so running it outside the launch region needed a
residency answer before launch rather than after. Self-hosting keeps every byte in
Frankfurt and accepts ownership of JetStream upgrades, storage, and failover. Backup and
restore for stream data was never covered by the managed tier, so nothing was given up
there.

This closes the gap.

## Consequences

- `region` on every tenant defaults to `eu-central-1` and the compose and Terraform work
  can assume it.
- We run the bus. JetStream upgrades, storage sizing, and failover are ours, and they sit
  on the same Terraform-plus-one-chart footing as the rest of the cluster.
- The bus being in-region means learner data stays in Frankfurt end to end for MVP, which
  is the answer `vision.md` promises institutions.

## Revisit triggers

Revisit when a managed NATS offering lists eu-central-1 (ADR 0001, 2026-10-09 change log),
when the second EU region is commissioned, when measured NATS operation becomes a
sustained burden, or when an institution names a region its data processing agreement
requires.