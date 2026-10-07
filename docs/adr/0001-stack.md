# ADR 0001: Application stack

- Status: proposed
- Date: 2026-10-08
- Deciders: unassigned
- Source: `docs/research/001-stack.md`, `docs/research/002-lab-isolation.md`

## Context

DistroLearn runs ephemeral, per-user sandbox labs on Kubernetes, targeting 50k MAU and
5k concurrent labs, multi-tenant by institution, with EU data residency as an option.
The deployment constraint is one Kubernetes cluster via one Helm chart, preferring
managed services and avoiding lock-in where it is cheap.

Two research docs examined the stack layers and, separately, lab isolation. This ADR
records the combined decision.

## Decision

**Application services**

| Layer | Choice |
|---|---|
| Frontend | Next.js 16.4.0 with React 19.3.0, terminal via xterm.js |
| Backend | Go 1.27.1 |
| API | REST with OpenAPI 3.2.1; WebSocket for terminal I/O only |
| Database | PostgreSQL 18.6, managed, row-level security per tenant |
| Queue and events | NATS JetStream 2.15.0 |
| Auth | AWS Cognito, managed, email and password |
| IaC and CI | Terraform 1.16.5, one Helm chart with Helm 4.3.0, Argo CD 3.5.4, GitHub Actions |

**Lab isolation**

| Concern | Choice |
|---|---|
| Sandbox runtime | gVisor (`runtimeClassName: gVisor`) |
| CNI | Cilium 1.20.2, enforcing NetworkPolicy |
| Egress | Default-deny per lab namespace, DNS and package mirrors explicitly allowed, public internet via an operated proxy |
| Tenant separation | Namespace per lab for network isolation, RLS for data isolation |
| Terminal access | Single-use WebSocket ticket, ~30s TTL, relayed through the API |

Lab pods are namespaced per lab on a dedicated node pool, run non-root with no service
account token under Pod Security Standards `restricted`, and are destroyed on a hard
TTL with reaping on two independent paths.

## Rationale

Go for the backend because the control plane holds thousands of long-lived connections
and waits on pod APIs, WebSocket relays, and NATS rather than on CPU. Static binaries
help the boot-time budget. Node 24.21.0 LTS was the serious alternative and would suit
a control plane with hundreds of concurrent labs rather than thousands.

Next.js because one deployable covers SSR for concept pages, which are the top of the
funnel, and the heavily client-side lab runner. A separate Vite SPA would mean two
artifacts in one chart and would give up SSR.

REST plus OpenAPI because the terminal is the only genuinely duplex, long-lived stream.
gRPC stays internal to service-to-service traffic; as the browser-facing edge it would
force grpc-web plus a proxy hop and break plain `curl` debugging.

PostgreSQL because the domain is relational and full of per-tenant joins, and RLS gives
database-enforced tenant isolation behind application checks. Managed, because failover
and point-in-time recovery are not product features.

NATS JetStream because it covers both durable work queues and fan-out in one small
system. Managed Kafka is the right answer for a large event log with many consumer
groups and is not what 5k concurrent labs need.

Cognito because MVP needs email, password, and an admin-managed roster. Per-001 the
backend verifies standard OIDC and stores all business records in PostgreSQL, so no
identifier lives in Cognito and the provider is replaceable without a data migration.
SSO and SCIM are explicitly out of MVP, which is what makes a managed provider the
right call rather than Keycloak.

Terraform outside the cluster, one Helm chart inside it, Argo CD reconciling from Git so
the running state is auditable rather than whatever the last CI run left behind.

gVisor over Kata because it matches the threat actually present, a curious learner
working on a deliberately fragile system rather than someone holding a VM escape
exploit. It is also a runtime class, so specific lab types can move to Kata later
without changing the control plane contract. Kata was not rejected on merit; it needs
hardware virtualization on every lab node, which is unverified against our provider,
and its boot cost has little slack against the p50 under 10s target.

Cilium over Calico because NetworkPolicy is inert without an enforcing CNI, and Cilium
adds the Layer 7 and FQDN egress control that lets labs reach specific services and
nothing else.

A relayed WebSocket ticket rather than a streamed Kubernetes exec, because the second
puts an API-server credential in the browser tab and makes the API server part of the
interactive data path.

## Consequences

Accepted costs:

- Every terminal byte and proxied service call passes through the API, making it a
  single interactive bottleneck at the hot path for 5k concurrent labs.
- gVisor carries performance overhead that is unbenchmarked for our pod sizes, against
  a committed p50 boot target of under 10 seconds.
- Some labs running real services may not be gVisor-compatible, which can invalidate a
  lab design rather than merely slow it.
- One managed PostgreSQL plus one cluster cannot satisfy tenant-selectable EU residency.
  See "Known conflicts".
- Adopting AWS via Cognito constrains cloud choice, including where EU regions are
  available.
- Helm 4 keeps `apiVersion: v2` chart support but has breaking CLI changes; pin it.

## Known conflicts

Flagged, not resolved. Each needs a decision from the product or architecture owner.

1. **Single cluster and single chart versus Cilium, the lab node pool, and EU
   residency.** The constraint from 001 cannot cover cluster-level CNI and node pool
   configuration, and one cluster cannot be in two regions. The plausible shape is one
   chart deployed to two clusters with a resolver routing learners to their tenant's
   region, but that is a product and cost decision.
2. **Vision records "5k peak, sustained" as a measure while mvp.md lists whether 5k is
   peak or sustained average as an open question.** One document is stale.
3. **Verification has no agreed home.** It must not run learner-controlled code in the
   control plane, but whether checks run as an in-namespace sidecar or a separate
   sandbox changes worker permissions, network policy, and a compromised lab's reach.
4. **gVisor overhead is unmeasured against the committed boot-time target.**
5. **No observability stack is chosen.** Log pipelines are named as a likely tenancy
   leak path, so this is a security gap, not just an observability gap.
6. **Lab artifacts and sandbox logs residency scope is undecided**, and it determines
   whether EU residency is a database concern or a storage-wide one.
7. **Node virtualization support for Kata is unverified.** Non-blocking under gVisor,
   but it must be checked before any VM-grade isolation claim is made to a customer.

## Alternatives considered

| Layer | Rejected | Reason |
|---|---|---|
| Frontend | Vite + React SPA | Loses SSR for concept pages; two artifacts in one chart |
| Backend | Node.js 24.21.0 LTS | Wrong shape for thousands of held-open connections; right answer at hundreds |
| API | gRPC at the browser edge | Needs grpc-web and a proxy hop, breaks `curl` |
| Database | DynamoDB | No joins or referential integrity across tenants and progress |
| Queue | Managed Kafka | Operational and cost floor above our needs; no native pub/sub fan-out for SQS |
| Auth | Self-hosted Keycloak 26.8.0 | Owns upgrades, backups, and availability for MVP-scope auth only |
| IaC | CI-driven `helm upgrade --install` | No drift detection, no record of what is running |
| Runtime | Kata Containers 4.2.0 | Node virtualization unverified; boot cost against a tight target |
| Runtime | Plain runc containers | Kernel shared with host; a lab teaching exploitation is exactly when this fails |
| CNI | Calico 3.33.0 | L3/L4 only; no FQDN egress control |
| Terminal | Streamed Kubernetes exec to browser | Places an API-server credential in the tab |

## Revisit triggers

Revisit this ADR when any of the following occurs:

- A lab type needs to run genuinely hostile code (revisit Kata).
- Verification architecture is decided (revises the lab network policy and worker
  permissions).
- EU residency scope includes lab artifacts or logs (revises storage and the
  cluster count).
- The boot-time target proves unreachable under gVisor (revisits the runtime choice).
- SSO or SCIM enters scope (revisit Cognito against Keycloak).

## Sources

Versions verified 2026-10-07 against official release pages, with links in
`docs/research/001-stack.md` and `docs/research/002-lab-isolation.md`. Component
releases: Next.js
[v16.4.0](https://github.com/vercel/next.js/releases/tag/v16.4.0), React
[v19.3.0](https://github.com/facebook/react/releases/tag/v19.3.0), Go
[1.27.1](https://go.dev/dl/?mode=json), PostgreSQL
[versions.json](https://www.postgresql.org/versions.json), NATS server
[v2.15.0](https://github.com/nats-io/nats-server/releases/tag/v2.15.0), gVisor
[release-20260928.0](https://github.com/google/gvisor/releases/tag/release-20260928.0),
Kata [4.2.0](https://github.com/kata-containers/kata-containers/releases/tag/4.2.0),
Cilium [v1.20.2](https://github.com/cilium/cilium/releases/tag/v1.20.2), Terraform
[v1.16.5](https://github.com/hashicorp/terraform/releases/tag/v1.16.5), Helm
[v4.3.0](https://github.com/helm/helm/releases/tag/v4.3.0), Argo CD
[v3.5.4](https://github.com/argoproj/argo-cd/releases/tag/v3.5.4). Behavioral claims
sourced to the Kubernetes NetworkPolicy and Pod Security Standards documentation, the
gVisor security model and production guide, and the Kata architecture README.
