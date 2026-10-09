# ADR 0004: Launch region

- Status: proposed
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

## Gap: no managed NATS in Frankfurt

ADR 0001 names NATS JetStream and `001-stack.md` says to prefer a managed offering.
There is no first-party AWS NATS service. The managed option is Synadia Cloud, whose
published region list on 2026-10-09 was ap-east-2, aws-euwest-1, aws-useast-2, and
aws-uswest-2. **eu-central-1 is not on it.**

So the managed bus and the launch region cannot be the same place today. Three ways out,
none of which this ADR decides:

- Run NATS JetStream ourselves in the cluster, in Frankfurt. Costs us the managed
  upgrade path and the backups that come with it.
- Use the managed bus in eu-west-1 and accept bus events in Ireland while tenant data
  stays in Frankfurt. The bus carries tenant identifiers, so this needs a residency
  answer before launch.
- Switch the launch region to eu-west-1, which matches the managed bus.

Unresolved. This ADR stays proposed until it is settled.

## Consequences

- `region` on every tenant defaults to `eu-central-1` and the compose and Terraform work
  can assume it.
- The bus region is the one open item in the residency story. It gates E9 and any claim
  about where learner data lives.

## Revisit triggers

Revisit when the bus hosting decision lands, when a second EU region is commissioned,
when Synadia Cloud adds a Frankfurt region, or when an institution names a region its
data processing agreement requires.