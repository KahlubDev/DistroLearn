# 002: Lab Isolation and Terminal Access

Isolation is decided in ADR 0002 and recorded here as the supporting research. Versions
verified 2026-10-07: gVisor `release-20260928.0`
([release](https://github.com/google/gvisor/releases/tag/release-20260928.0)),
Kata 4.2.0 ([release](https://github.com/kata-containers/kata-containers/releases/tag/4.2.0)),
Cilium 1.20.2 ([release](https://github.com/cilium/cilium/releases/tag/v1.20.2)).

## 1. Ephemeral per-user labs

Each lab is a set of pods on a dedicated node pool, in its own namespace, with a hard
TTL. Nodes are shared; labs are not.

| | Mechanism | Fit |
|---|---|---|
| runc | cgroups, namespaces | Kernel shared with host. A lab teaching exploitation is exactly when this fails. |
| gVisor | Userspace Sentry reimplements the Linux system ABI and intercepts syscalls. A sandbox can "manipulate virtualized system resources... and not underlying host system resources" ([security model](https://gvisor.dev/docs/architecture_guide/security/)). Selected per pod by `runtimeClassName` ([K8s quick start](https://gvisor.dev/docs/user_guide/quick_start/kubernetes/)). | Chosen. Documented for untrusted code and multi-tenancy, with "some performance overhead" ([production guide](https://gvisor.dev/docs/user_guide/production/)). |
| Kata 4.2.0 | Lightweight VM per container, hardware virtualization as "a second layer of defence" ([architecture](https://github.com/kata-containers/kata-containers/blob/main/docs/design/architecture/README.md)). | Stronger boundary. Needs virtualization on every lab node, unverified against our provider, and boot cost is tight against p50 under 10s. |

**gVisor is the choice.** It matches the threat, a curious learner working on a fragile
system rather than someone holding a VM escape exploit. Being a runtime class, specific
lab types can move to Kata later without changing the control plane contract.

Do not call it a VM. gVisor's own docs caution that virtualization hardware does not by
itself make a system "more or less secure", and that gVisor does not stop hardware side
channels ([FAQ](https://gvisor.dev/docs/architecture_guide/security/)).

Per-lab hardening:

- Dedicated namespace, deleted on teardown.
- Dedicated node pool with taints and tolerations, so lab pods never co-locate with the
  product.
- `automountServiceAccountToken: false`. A lab cannot read the Kubernetes API.
- PSS `restricted` at admission: `runAsNonRoot: true`, `allowPrivilegeEscalation: false`,
  no privileged containers, `seccompProfile: RuntimeDefault`
  ([PSS](https://kubernetes.io/docs/concepts/security/pod-security-standards/)). Same page
  disallows `hostNetwork`, `hostPID`, `hostIPC`.
- Read-only root filesystem where the lab allows it, `emptyDir` for scratch.
- Resource requests and limits, plus a per-node concurrency cap.
- TTL shorter than any learner session. A reaper deletes expired labs, with a second
  path for when the control plane itself is down. Leaked labs are the main cost risk.

## 2. Default-deny egress and tenant isolation

NetworkPolicy requires an enforcing CNI: "Your cluster must use a network plugin that
supports NetworkPolicy enforcement"
([Network Policies](https://kubernetes.io/docs/concepts/services-networking/network-policies/)).
**Cilium 1.20.2** is the choice, for Layer 7 and FQDN egress rules
([policy docs](https://docs.cilium.io/en/stable/security/policy/)) that let labs reach
specific services and nothing else. Calico 3.33.0 gives L3/L4 only.

Egress, layered:

- Default-deny per lab namespace. The docs warn a bare default-deny "also blocks DNS
  traffic", so DNS must be re-allowed or every lab fails silently.
- Allow the cluster DNS and any package mirror labs need.
- Nothing else. Public internet goes through an allow-listed proxy we operate.
- L7/FQDN rules over IP blocks wherever the destination is a hostname.

Tenant isolation runs on two independent mechanisms. Network: one namespace per lab,
default-deny both ways, so learners cannot reach each other even given each other's pod
IPs. Data: PostgreSQL RLS keyed on tenant, so a forgotten filter returns nothing. A
defect in one is contained by the other.

## 3. Browser to terminal without cluster credentials

The browser must never hold a kubeconfig, an API bearer token, or an SSH key. Any of
those in a tab is a cluster-wide compromise from one stolen session.

```
Browser (xterm.js) <-- WSS, single-use ticket --> Go control plane
Go control plane <-- exec/attach, own ServiceAccount --> lab pod
```

1. Learner requests a terminal. The control plane authorizes against the lab record
   and checks tenant and live session.
2. Control plane mints a single-use WebSocket ticket scoped to one lab, ~30s TTL, and
   returns the WSS URL. No credential crosses.
3. Browser spends the ticket; the control plane opens the PTY with its own
   ServiceAccount.
4. Lab service endpoints are proxied through the control plane under the same check,
   not exposed directly by Ingress.

Every terminal byte is proxied through the control plane, so it must be sized for 5k
concurrent sessions and sits on the hot path. A per-lab ServiceAccount token in the
browser is a worse failure mode. Rejected streaming the Kubernetes API's own exec to the
browser: least code, but it puts an API-server credential in the tab and makes the API
server part of the interactive path.

## Risks

1. Kata node virtualization unverified. Non-blocking under gVisor, but check before
   promising VM-grade isolation to a customer.
2. gVisor overhead unbenchmarked for our pod sizes, against a committed p50 boot target
   under 10s.
3. Lab compatibility with gVisor. Some labs run real services; incompatibility
   invalidates a lab design rather than slowing it.
4. The control plane is a single interactive bottleneck for terminal and proxied
   traffic. Needs measured connection limits.
5. Egress leakage via image pulls, which happen at the node outside lab NetworkPolicy.
6. Labs as a compute or attack platform. Egress limits do not stop resource
   consumption; per-tenant caps and cost alerting are required.
7. Tenancy leaks through shared infrastructure: node pools, image registries, log
   pipelines.
8. Verification placement was open here; settled in ADR 0002 as a separate runner pod
   outside the learner's lab.
