# 002: Lab Isolation and Terminal Access

Answers the open question in `docs/product/mvp.md` ("sandbox isolation model and
how far it is shared between concurrent labs") and the two enforcement questions it
implies. Builds on the Kubernetes cluster and single-chart constraint from
`docs/research/001-stack.md`.

Verified against official docs on 2026-10-07. Component versions cited where a
release number is meaningful: gVisor `release-20260928.0`
([release](https://github.com/google/gvisor/releases/tag/release-20260928.0)),
Kata Containers 4.2.0 ([release](https://github.com/kata-containers/kata-containers/releases/tag/4.2.0)),
Cilium 1.20.2 ([release](https://github.com/cilium/cilium/releases/tag/v1.20.2)).

## 1. Running ephemeral per-user labs

Every lab is a set of pods created by the control plane (Go service, `001`), on a
dedicated node pool, in its own namespace, with a hard TTL. Nothing is shared
between two learners. That is the honest answer to "how far is it shared": nodes are
shared, labs are not.

### gVisor  vs Kata  vs plain containers

| | Mechanism | Strength | Fit |
|---|---|---|---|
| Plain containers (runc) | cgroups, namespaces | Kernel shared with host; an escape is a kernel compromise. | Not acceptable. A lab teaching exploitation is exactly the case where someone tries it. |
| gVisor (`runsc`) | Userspace Sentry re-implements the Linux system ABI, intercepting syscalls. Sandbox can "manipulate virtualized system resources... and not underlying host system resources" ([security model](https://gvisor.dev/docs/architecture_guide/security/)). | Much smaller host kernel surface. No device emulation. Selected per pod by `runtimeClassName` ([K8s quick start](https://gvisor.dev/docs/user_guide/quick_start/kubernetes/)). | Best fit. Documented for "running untrusted code" and multi-tenant isolation, with the tradeoff stated as "some performance overhead" ([production guide](https://gvisor.dev/docs/user_guide/production/)). |
| Kata 4.2.0 | Each container in a lightweight VM using hardware virtualization as "a second layer of defence" ([architecture](https://github.com/kata-containers/kata-containers/blob/main/docs/design/architecture/README.md)). | Strongest: hardware boundary. | Needs hardware virtualization on every lab node. Provider support unverified. Higher boot cost against a p50 < 10s target with little slack. |

Recommendation: **gVisor for MVP.** It matches the threat we actually have, a curious
learner poking at a deliberately fragile system rather than someone holding a VM
escape exploit, and being a runtime class it can be enabled per pod, so we can move
specific lab types to Kata later without changing the control plane contract.

On strength: gVisor's own docs caution that virtualization hardware does not by
itself make a system "more or less secure", and that gVisor does not stop hardware
side channels ([security model FAQ](https://gvisor.dev/docs/architecture_guide/security/)).
Do not describe gVisor to customers as "a VM".

What we reject: Kata for now, on unverified node-virtualization support and boot time.
Revisit when a lab type needs to run genuinely hostile code, such as fuzzing a
kernel-adjacent service.

Per-lab hardening, applied to every lab pod regardless of runtime:
- Dedicated namespace per lab, deleted on teardown.
- A dedicated node pool with taints and tolerations so lab pods never co-locate with
  the product itself.
- `automountServiceAccountToken: false` on lab pods. No lab should be able to read
  the Kubernetes API.
- Pod Security Standards `restricted` enforced at admission for the lab namespace.
  This requires `runAsNonRoot: true`, `allowPrivilegeEscalation: false`,
  `privileged` unset, and `seccompProfile: RuntimeDefault`
  ([Pod Security Standards](https://kubernetes.io/docs/concepts/security/pod-security-standards/)).
- No `hostNetwork`, `hostPID`, or `hostIPC`; the restricted profile disallows all
  three.
- Read-only root filesystem where the lab allows it, `emptyDir` for scratch.
- Resource requests and limits per pod, plus a concurrency cap per node driven by
  the `001` sizing decision.

Lifecycle: lab pod has a TTL shorter than any learner session. A reaper in the
control plane deletes expired labs, and a second path deletes them if the control
plane itself is down. Leaked labs are the main cost and abuse risk, so this needs to
be belt-and-braces.

## 2. Default-deny egress and tenant isolation

**NetworkPolicy does not work unless the CNI supports it.** The Kubernetes docs are
explicit: "Your cluster must use a network plugin that supports NetworkPolicy
enforcement" ([Network Policies](https://kubernetes.io/docs/concepts/services-networking/network-policies/)).

Recommendation: **Cilium 1.20.2 as the CNI.** It enforces NetworkPolicy and adds
Layer 7 policy and FQDN-based egress rules, which matters because labs need to reach
specific services and nothing else ([Cilium policy docs](https://docs.cilium.io/en/stable/security/policy/)).
Calico 3.33.0 would also work for L3/L4 but does not give the FQDN egress control we
want.

Egress, layered:
- A default-deny egress policy per lab namespace. The docs warn that "a default
  deny-all egress policy also blocks DNS traffic", so DNS must be re-allowed
  explicitly to the cluster DNS service or every lab silently fails.
- An explicit allow to the lab-internal DNS and any package mirror the labs need.
- Nothing else. If a lab needs the public internet, it goes through an allow-listed
  proxy we operate, so that egress is observable and rate-limited.
- Cilium L7/FQDN rules rather than bare IP blocks wherever the destination is
  expressible as a hostname.

Tenant isolation, two independent mechanisms:
- **Network.** One namespace per lab, default-deny in both directions. Two learners'
  labs cannot reach each other even if they guess each other's pod IPs, and the
  lab pool cannot reach the product's own services.
- **Data.** PostgreSQL row-level security keyed on tenant, as chosen in `001`. A
  query that forgets its tenant filter returns nothing rather than another
  institution's data.

Treat these as defence in depth. A bug in one layer is contained by the other, which
is the standard argument for doing both rather than picking one.

## 3. Browser to terminal, no cluster credentials in the browser

The browser must never hold a kubeconfig, a bearer token for the Kubernetes API, or
an SSH key to the lab host. Any of those in a browser tab is a cluster-wide
compromise from a single stolen session.

Recommended path, browser to terminal:

```
Browser (xterm.js) <-- WSS, short-lived ticket --> Go control plane
Go control plane <-- exec/attach via in-cluster ServiceAccount --> lab pod
```

Mechanics:
1. Learner requests a terminal. The control plane authorizes them against the lab
   record in Postgres and checks the lab belongs to their tenant and their session.
2. Control plane mints a single-use WebSocket ticket, scoped to one lab, with a
   TTL of about 30 seconds, and returns the WSS URL. No credential crosses.
3. Browser opens the WSS connection to the control plane, spends the ticket, and the
   control plane opens the PTY on the lab pod using its own ServiceAccount.
4. Lab service endpoints for learners who need to inspect the distributed system
   (a broker to publish to, a node to curl) are exposed through the control plane
   under the same authorization check, not directly by Ingress.

Consequences worth accepting: every terminal byte is proxied through the control
plane, so it must be sized for 5k concurrent sessions and sits on the hot path for
lab interactivity. The alternative, giving the browser a kubeconfig-scoped ServiceAccount
token per lab, is a much worse failure mode.

Rejected: streaming the Kubernetes API's own exec/attach stream straight to the
browser. It is the least code, but it puts an API-server credential in the browser
and makes the API server part of the interactive data path.

## Risks

1. **Node virtualization support for Kata is unverified.** Our gVisor choice makes
   this non-blocking for MVP, but it should be checked before we promise any
   VM-grade isolation to an enterprise buyer.
2. **gVisor performance overhead against the p50 < 10s boot target.** The project
   states there is overhead without publishing a figure for our pod sizes. This needs
   a benchmark on our node types before we commit to the number in the vision doc.
3. **Lab compatibility with gVisor.** Some labs run real services: a broker, a
   database, possibly something doing low-level networking. gVisor maintains a
   compatibility list and not everything is on it. This should be validated per lab
   topic early, since it can invalidate a lab design rather than just slow it down.
4. **Control plane becomes a single interactive bottleneck.** Every terminal byte and
   proxied service call passes through it. Scale-out and connection limits need real
   numbers, not assumptions.
5. **Egress leakage through DNS or image pulls.** Image pulls happen at the node,
   outside lab NetworkPolicy. A lab that can influence what gets pulled is an egress
   path the policy cannot see.
6. **Abuse of labs as a compute or attack platform.** Egress restrictions limit
   reach but do not prevent resource consumption. Per-tenant concurrency caps and
   cost alerting are required, and an abuse policy is a product decision we have not
   made.
7. **Tenancy leaks through shared infrastructure.** Namespaces and RLS cover the
   obvious paths. Shared node pools, shared image registries, and log pipelines are
   the paths most likely to be missed, and none are solved here.
8. **Verification running inside the lab.** The `mvp.md` verification feature must
   not execute learner-controlled code inside the control plane. Where it runs, and
   with what privileges, is an open design question this document does not settle.

## Not settled here

Sandbox isolation is now recommended, not decided. Two things in particular need a
product or architecture decision before implementation planning: whether any lab type
requires VM-grade isolation in MVP, and whether verification runs inside the lab pod
or in a separate sandbox.
