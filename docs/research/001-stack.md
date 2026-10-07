# 001: Stack Recommendation

One option per layer. Versions verified against official release pages on 2026-10-07.
Decisions are recorded in `docs/adr/0001-stack.md`.

| Layer | Chosen | Version | Source |
|---|---|---|---|
| Frontend | Next.js App Router, React | 16.4.0 / 19.3.0 | [next.js v16.4.0](https://github.com/vercel/next.js/releases/tag/v16.4.0), [react v19.3.0](https://github.com/facebook/react/releases/tag/v19.3.0) |
| Backend | Go | 1.27.1 | [go.dev/dl](https://go.dev/dl/?mode=json) |
| API | REST + OpenAPI, WebSocket for terminal I/O | 3.2.1 | [OAS 3.2.1](https://github.com/OAI/OpenAPI-Specification/releases/tag/3.2.1) |
| Database | PostgreSQL, managed, RLS per tenant | 18.6 | [versions.json](https://www.postgresql.org/versions.json), [RDS release calendar](https://docs.aws.amazon.com/AmazonRDS/latest/PostgreSQLReleaseNotes/postgresql-release-calendar.html) |
| Queue / events | NATS JetStream | server 2.15.0, client 1.54.0 | [nats-server v2.15.0](https://github.com/nats-io/nats-server/releases/tag/v2.15.0), [nats.go v1.54.0](https://github.com/nats-io/nats.go/releases/tag/v1.54.0) |
| Auth | AWS Cognito, email + password | current service | [AWS endpoints](https://docs.aws.amazon.com/general/latest/gr/cognito.html) (eu-central-1, eu-west-1, eu-north-1) |
| IaC / CI | Terraform, one Helm chart, Argo CD, GitHub Actions | 1.16.5 / 4.3.0 / 3.5.4 | [terraform v1.16.5](https://github.com/hashicorp/terraform/releases/tag/v1.16.5), [helm v4.3.0](https://github.com/helm/helm/releases/tag/v4.3.0), [argo-cd v3.5.4](https://github.com/argoproj/argo-cd/releases/tag/v3.5.4) |

## Rationale and rejections

**Next.js 16.4.0.** One deployable covers SSR for concept pages, which are the top of
the funnel, plus the client-heavy lab runner. Rejected Vite + React SPA: loses SSR and
puts two artifacts in one chart.

**Go 1.27.1.** The control plane holds thousands of long-lived connections and waits on
pod APIs, WebSockets, and NATS rather than on CPU. Static binaries help the boot budget.
Rejected Node 24.21.0 LTS ([releases](https://nodejs.org/en/about/previous-releases);
26.11.0 is Current, not LTS): event-loop latency and memory under thousands of held-open
connections. That answer works at hundreds of concurrent labs.

**REST + OpenAPI 3.2.1.** The terminal is the only duplex stream. Rejected
gRPC at the browser edge (grpc-go 1.84.0, protobuf 36.2): needs grpc-web plus a proxy
hop, breaks `curl`. Fine later for service-to-service.

**PostgreSQL 18.6, managed.** The domain is relational and full of per-tenant joins;
RLS gives database-enforced isolation as a backstop. Major 18 is current with EOL
2030-11-14. Managed, since failover and point-in-time recovery are not product
features. Rejected DynamoDB: no joins or referential integrity across tenants, rosters,
and progress.

**NATS JetStream 2.15.0.** Durable work queues and fan-out in one small system,
at-least-once, per-consumer replay. Prefer a managed NATS offering; the protocol is
portable, so lock-in stays cheap to avoid. Rejected managed Kafka: correct for a large
event log with many consumer groups, above our needs at 5k peak. Also rejected SQS
alone: a fine work queue with no native fan-out, so SNS gets bolted on and two systems
replace one.

**AWS Cognito.** MVP needs email, password, and an admin-managed roster. The backend
verifies standard OIDC and keeps user and tenant records in PostgreSQL, so no business
identifier lives in Cognito and the provider is replaceable without a data migration.
Rejected self-hosted Keycloak 26.8.0 ([release](https://github.com/keycloak/keycloak/releases/tag/26.8.0)):
owning upgrades, backups, and availability for auth features MVP does not sell. Revisit
when SSO and SCIM land.

**Terraform, one Helm chart, Argo CD.** Terraform owns the cluster, managed Postgres,
Cognito, and DNS. Argo CD reconciles from Git so the running state is auditable. Helm 4
keeps `apiVersion: v2` support ([4.0.0 notes](https://github.com/helm/helm/releases/tag/v4.0.0)),
but the CLI has breaking changes, so pin it. Rejected CI-driven `helm upgrade --install`:
no drift detection, no record of what is running.

## Limits

- Cluster version is unpinned. Kubernetes 1.37.1 is current upstream
  ([release](https://github.com/kubernetes/kubernetes/releases/tag/v1.37.1)), but which
  version a managed cluster offers is a deployment-time fact. Confirm before pinning.
- Do not use Node 26.11.0 in production images; it is Current, not LTS.
- These are point-in-time checks from 2026-10-07. Re-verify before the first production
  build.
