# 001: Stack Recommendation

One option per layer for the MVP. All versions verified against official release
pages on 2026-10-07.

| Layer | Chosen | Version verified | Source |
|---|---|---|---|
| Frontend | Next.js (App Router) on React | 16.4.0 / React 19.3.0 | [next.js v16.4.0](https://github.com/vercel/next.js/releases/tag/v16.4.0), [react v19.3.0](https://github.com/facebook/react/releases/tag/v19.3.0) |
| Backend language | Go | 1.27.1 | [go.dev/dl](https://go.dev/dl/?mode=json) (`go1.27.1`, stable) |
| API style | REST + OpenAPI, WebSocket for terminal I/O | OpenAPI 3.2.1 | [OAS 3.2.1](https://github.com/OAI/OpenAPI-Specification/releases/tag/3.2.1) |
| Database | PostgreSQL on a managed service (RDS/Aurora or Cloud SQL), RLS per tenant | 18.6 (major 18, current) | [postgresql.org/versions.json](https://www.postgresql.org/versions.json), [RDS release calendar](https://docs.aws.amazon.com/AmazonRDS/latest/PostgreSQLReleaseNotes/postgresql-release-calendar.html) |
| Queue / events | NATS JetStream | server 2.15.0, Go client 1.54.0 | [nats-server v2.15.0](https://github.com/nats-io/nats-server/releases/tag/v2.15.0), [nats.go v1.54.0](https://github.com/nats-io/nats.go/releases/tag/v1.54.0) |
| Auth | AWS Cognito, managed, email + password | current service | [AWS General Reference, Amazon Cognito endpoints](https://docs.aws.amazon.com/general/latest/gr/cognito.html) (eu-central-1, eu-west-1, eu-north-1) |
| IaC / CI | Terraform + one Helm chart + Argo CD, GitHub Actions pipelines | Terraform 1.16.5, Helm 4.3.0, Argo CD 3.5.4 | [terraform v1.16.5](https://github.com/hashicorp/terraform/releases/tag/v1.16.5), [helm v4.3.0](https://github.com/helm/helm/releases/tag/v4.3.0), [argo-cd v3.5.4](https://github.com/argoproj/argo-cd/releases/tag/v3.5.4) |

## Why each, and what was rejected

### Frontend: Next.js 16.4.0
One deployable covers server-rendered concept pages (SEO matters for a learning
product; concept pages are the top-of-funnel) and the client-heavy lab runner with
its live terminal. Static export is not enough for the lab side, and a separate SPA
would mean two deployables and two pipelines in the one Helm chart.

Rejected: Vite + React SPA (18.x era). Faster dev loop and simpler mental model, but
we lose SSR for concept pages and end up shipping and operating two artifacts.

### Backend language: Go 1.27.1
The lab control plane holds many thousands of long-lived connections and spends most
of its time waiting on pod APIs, WebSocket relays, and NATS. Go gives small static
binaries (fast pod start, which the p50 < 10s lab boot budget cares about),
predictable memory under thousands of concurrent connections, and trivial
cross-compilation for arm64 EU nodes.

Rejected: TypeScript on Node 24.21.0 LTS ([Node.js releases](https://nodejs.org/en/about/previous-releases);
v24.21.0 is Latest LTS, v26.11.0 is Current and not yet LTS). One language across
the stack and a better hiring pool are real advantages, but event-loop latency under
thousands of held-open lab connections, and the memory profile of the connection
tier, are the wrong shape for this workload. Node 24.21.0 LTS would be the right call
for a control plane with hundreds of concurrent labs.

### API style: REST + OpenAPI 3.2.1
Resource-oriented CRUD and query endpoints over HTTP/JSON, with OpenAPI 3.2.1 as the
generated contract driving both the typed Go client and the frontend. The terminal
channel is a WebSocket, which is the one genuinely duplex, long-lived stream in the
product; everything else stays inspectable with curl and loggable as JSON.

Rejected: gRPC as the public edge (grpc-go 1.84.0, protobuf 36.2). Excellent for
internal service-to-service traffic and worth adopting later between services. As
the browser-facing boundary it forces grpc-web plus a proxy hop, breaks plain
`curl` debugging, and puts a codegen toolchain in front of every front-end change.

### Database: PostgreSQL 18.6, managed
Learners, institutions, roles, rosters, labs, and progress are relational and full of
joins and per-tenant filtering. PostgreSQL's row-level security gives tenant
isolation a database-enforced backstop behind application checks, which matters for
the multi-tenant requirement and is cheap. Major 18 is current with end of life
2030-11-14, so the version runway is long. Managed service over self-managed, since
failover, backups, and point-in-time recovery are not product features.

Rejected: DynamoDB. No joins or referential integrity across tenants, rosters, and
progress; every tenant-scoped read would need an index discipline that a relational
model gives us directly. Fine at 50k MAU for single-entity documents, wrong shape for
this schema.

### Queue / events: NATS JetStream 2.15.0
Two distinct jobs. Durable work queues: lab provisioning, verification runs,
teardown, timeout reaping. Fan-out: progress updates and terminal-adjacent events that
several services observe. JetStream covers both with one small system, at-least-once
delivery, and per-consumer replay. The wire protocol and clients are portable across
providers, so lock-in stays cheap to avoid: prefer a managed NATS offering, run it
in-cluster only if that is not available at needed scale.

Rejected: managed Kafka. Correct for a large durable event log with many independent
consumer groups and high retention. At 5k concurrent labs we have a handful of
consumers and short retention, and Kafka's operational and cost floor is well above
what this needs. Also rejected Amazon SQS as the sole mechanism: it is a fine work
queue but has no native pub/sub fan-out, so we would bolt SNS on top and end up with
two systems where NATS is one.

### Auth: AWS Cognito (managed)
MVP needs email plus password, an admin-managed roster, and three roles. Cognito
handles credential storage, token issuance, and brute-force protection we would
otherwise own, and it is available in EU regions, which the data-residency option
depends on. SSO and SCIM are explicitly out of MVP, so we are not buying capability
we cannot use.

Lock-in mitigation is cheap here: the backend verifies standard OIDC/JWT and stores
its own user and tenant records in PostgreSQL, so no business identifier lives in
Cognito. Swapping providers later is a configuration change plus a re-login, not a
data migration.

Rejected: self-hosted Keycloak 26.8.0 ([keycloak 26.8.0](https://github.com/keycloak/keycloak/releases/tag/26.8.0)).
Nominally the more flexible option and the one to revisit when SSO and SCIM land. For
MVP it means owning upgrades, backups, and availability for a feature set we are not
yet selling.

### IaC / CI: Terraform 1.16.5, Helm 4.3.0, Argo CD 3.5.4, GitHub Actions
Terraform owns everything outside the cluster: the Kubernetes cluster, managed
Postgres, Cognito user pools, and DNS. Everything inside the cluster is exactly one
Helm chart, satisfying the deployment constraint with a single `helm upgrade` path and
one values file per environment. Argo CD reconciles the cluster from Git so the
running state is auditable and drift is visible, rather than being whatever the last
CI run left behind.

Note for the scaffolder: Helm 4 keeps support for chart `apiVersion: v2`
([Helm 4.0.0 release notes](https://github.com/helm/helm/releases/tag/v4.0.0)), so
mainstream dependency charts install unchanged, but the CLI has breaking changes and
should be pinned.

Rejected: CI pushing straight to the cluster with `helm upgrade --install`. Fewer
moving parts, but no drift detection and no record of what is actually running, which
is uncomfortable for a multi-tenant product handling student data.

## Assumptions and limits of this research

- Cluster runtime version is deliberately not pinned here. Kubernetes 1.37.1 is
  current upstream ([kubernetes v1.37.1](https://github.com/kubernetes/kubernetes/releases/tag/v1.37.1)),
  but which version a managed cluster offers is a deployment-time fact, not a
  research finding. Confirm the provider's supported list before pinning.
- This document does not decide sandbox lab isolation, which is the open question in
  `docs/product/mvp.md` that most affects the backend and IaC layers.
- Node 26.11.0 exists and is newer than 24.21.0, but it is Current, not LTS. Do not
  use it for production images.
- Version checks are point-in-time as of 2026-10-07. Re-verify before the first
  production build; none of these are pinned to a floating tag in the recommendations
  above.
