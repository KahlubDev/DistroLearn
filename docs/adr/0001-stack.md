# ADR 0001: Application stack

- Status: accepted
- Date: 2026-10-08
- Related: `0002-mvp-scope.md`
- Source: `docs/research/001-stack.md`, `002-lab-isolation.md`

## Context

Ephemeral per-user sandbox labs on Kubernetes, targeting 50k MAU and 5k concurrent labs
at peak, multi-tenant by institution. Deployment constraint: one cluster via one Helm
chart, preferring managed services, avoiding lock-in where it is cheap.

## Decision

| Layer | Choice |
|---|---|
| Frontend | Next.js 16.4.0 with React 19.3.0, terminal via xterm.js |
| Backend | Go 1.27.1 |
| API | REST with OpenAPI 3.2.1; WebSocket for terminal I/O only |
| Database | PostgreSQL 18.6, managed, row-level security per tenant |
| Queue and events | NATS JetStream 2.15.0, self-hosted in the cluster |
| Auth | AWS Cognito, managed, email and password |
| IaC and CI | Terraform 1.16.5, one Helm chart with Helm 4.3.0, Argo CD 3.5.4, GitHub Actions |
| Sandbox runtime | gVisor (`runtimeClassName: gVisor`) |
| CNI | Cilium 1.20.2, enforcing NetworkPolicy |
| Egress | Default-deny per lab namespace, DNS and mirrors allowed, public internet via an operated proxy |
| Tenant separation | Namespace per lab for network, RLS for data |
| Terminal access | Single-use WebSocket ticket, ~30s TTL, relayed through the API |

Repo layout: pnpm workspaces for web, one Go module for API, workers, and the
verification runner. Cilium and the lab node pool are cluster-level, configured by
Terraform rather than by the chart. Full layout in `docs/architecture/system.md`.

Lab pods run non-root with no service account token under PSS `restricted`, one
namespace per lab on a dedicated node pool, destroyed on a hard TTL with reaping on two
independent paths.

## Rationale

Go holds thousands of long-lived connections while waiting on pod APIs, WebSockets, and
NATS rather than on CPU. Node 24.21.0 LTS suits hundreds of concurrent labs, not
thousands.

Next.js puts SSR concept pages and the lab runner in one deployable. A separate Vite SPA
means two artifacts in one chart and gives up SSR.

The terminal is the only duplex stream, so REST plus OpenAPI covers the rest. gRPC
stays internal; at the browser edge it forces grpc-web plus a proxy hop and breaks
`curl`.

PostgreSQL fits a relational domain with per-tenant joins, and RLS backstops application
checks. Managed, since failover and point-in-time recovery are not product features.

NATS JetStream covers durable work queues and fan-out in one system. Kafka suits a large
event log with many consumer groups, which 5k peak labs are not.

Cognito covers MVP auth. The backend verifies standard OIDC and keeps business records
in PostgreSQL, so no identifier lives in Cognito and the provider is replaceable without
a data migration. SSO and SCIM are out of MVP, which is what makes a managed provider
preferable to Keycloak.

Argo CD reconciles from Git so the running state is auditable. Helm 4 keeps
`apiVersion: v2` support but has breaking CLI changes, so pin it.

gVisor matches the threat present: a curious learner on a fragile system.
As a runtime class, specific lab types can move to Kata later without changing the
control plane contract. Kata needs hardware virtualization on every lab node,
unverified against our provider, with boot cost tight against p50 under 10s.

Cilium over Calico because NetworkPolicy is inert without an enforcing CNI, and Cilium
adds the Layer 7 and FQDN egress control labs need.

A relayed ticket rather than a streamed Kubernetes exec, because streaming puts an
API-server credential in the browser tab and puts the API server on the interactive
data path.

## Consequences

- The API is a single interactive bottleneck: every terminal byte and proxied service
  call passes through it.
- gVisor overhead is unbenchmarked for our pod sizes, against a committed p50 boot
  target under 10s.
- Labs running real services may not be gVisor-compatible, which invalidates a lab
  design rather than slowing it.
- Adopting AWS through Cognito constrains cloud choice and where EU regions are
  available.
- One cluster cannot hold two regions. ADR 0002 settles MVP as single-region with a
  `region` field on tenants.

## Change log

**2026-10-09: queue and events moved from managed to self-hosted.** The decision table
above now reads NATS JetStream 2.15.0, self-hosted in the cluster. `docs/research/001-stack.md`
preferred a managed offering, and that preference stands. It could not be met: AWS has no
first-party NATS service, and the managed option, Synadia Cloud, listed ap-east-2,
aws-euwest-1, aws-useast-2, and aws-uswest-2 on 2026-10-09, with no eu-central-1.

The launch region is eu-central-1 (ADR 0004), so a managed bus would have put bus events in
eu-west-1 while tenant data stayed in Frankfurt. The bus carries tenant identifiers, which
makes that a residency question. Self-hosting in the Frankfurt cluster keeps everything in
one region and accepts the cost below.

Revisit when Synadia Cloud lists eu-central-1, or any other managed NATS offering does.
We own JetStream upgrades, its storage, and its failover from then on, which is the cost
being traded away. Backup and restore for stream data is ours too, and Synadia's managed
tier does not cover it either, so self-hosting does not lose backups specifically.

## Alternatives considered

| Layer | Rejected | Reason |
|---|---|---|
| Frontend | Vite + React SPA | Loses SSR; two artifacts in one chart |
| Backend | Node.js 24.21.0 LTS | Wrong shape for thousands of held-open connections |
| API | gRPC at the browser edge | Needs grpc-web and a proxy hop; breaks `curl` |
| Database | DynamoDB | No joins or referential integrity across tenants and progress |
| Queue | Managed Kafka | Cost and ops floor above our needs; SQS has no native fan-out |
| Auth | Self-hosted Keycloak 26.8.0 | Owns upgrades and uptime for MVP-scope auth only |
| IaC | CI-driven `helm upgrade --install` | No drift detection, no record of what is running |
| Runtime | Kata Containers 4.2.0 | Node virtualization unverified; boot cost |
| Runtime | Plain runc containers | Kernel shared with host |
| CNI | Calico 3.33.0 | L3/L4 only, no FQDN egress control |
| Terminal | Streamed Kubernetes exec to browser | API-server credential in the tab |
| Monorepo tooling | Turborepo | Wraps `next build` and adds a dependency for no cache win at this size |

## Revisit triggers

Revisit when a lab type needs to run hostile code (Kata), SSO or SCIM enters
scope (Cognito against Keycloak), the second EU region is commissioned, or the
boot-time target proves unreachable under gVisor. For the queue, revisit when a managed
NATS offering lists eu-central-1 (see the 2026-10-09 change log).

## Sources

Versions verified 2026-10-07 against official release pages, with per-item links in
`docs/research/001-stack.md` and `002-lab-isolation.md`. Behavioral claims cite the
Kubernetes NetworkPolicy and Pod Security Standards documentation, the gVisor security
model and production guide, and the Kata architecture README.
