# KubeCon NA 2026 — CFP Draft

> Draft submission for issue [#23](https://github.com/amcheste/claude-teams-operator/issues/23). Conference: KubeCon + CloudNativeCon North America 2026, Salt Lake City, Nov 9–12. CFP deadline: **May 31, 2026 at 11:59pm MT**. Submit at https://sessionize.com/kubecon-cloudnativecon-north-america-2026/.
>
> This is a starting draft. Every field below is meant to be edited. Open questions for the maintainer are listed at the bottom.

---

## Submission metadata

| Field | Recommendation | Rationale |
|-------|----------------|-----------|
| **Submission type** | Session Presentation (30 min) | The form's options are 5 / 30 / 75 minutes. 30 fits the demo-heavy structure without padding. Tutorial (75 min) is the alternative if the maintainer wants hands-on. |
| **Track (primary)** | AI Inference + Agentic | New track for 2026. Direct fit: this is a system for running agent workloads on K8s. |
| **Track (alternate)** | Platform Engineering | Reasonable alternate angle: the operator *is* platform infra for agent teams. Pick AI Inference + Agentic if both feel viable, since the program committee may load-balance between them. |
| **Audience level** | Intermediate | Assumes operator-pattern literacy (CRDs, reconcile loops, RBAC, PVC access modes). Does not assume Claude Code or LLM background. |
| **Case study?** | No | This is a project talk, not a deployment retrospective. |

---

## Abstract title (75 char max)

**Primary:**

```
Reconciling Agent Teams: A Kubernetes Operator for Claude Code
```

(62 characters)

**Alternates worth considering:**

```
Stateless Agents, Stateful Cluster: K8s for Claude Code Agent Teams
```

(67 characters — leans harder into the architectural narrative from KUBECON.md: the agent forgets, the cluster remembers.)

```
The Operator Pattern for Multi-Agent Coding Teams
```

(49 characters — most general, drops the Claude Code brand. Use if the program committee tends to read brand-named titles as vendor pitches.)

---

## Abstract (1,300 char max)

> Most multi-agent orchestration frameworks treat Kubernetes as deployment infrastructure: pods that happen to run an LLM. This talk shows what changes when the cluster becomes the coordination fabric. The claude-teams-operator runs Anthropic's Claude Code Agent Teams as a CRD-driven workload, preserving the native file-based mailbox protocol over a ReadWriteMany PVC instead of inventing a new one. An AgentTeam resource declares a lead, teammates, budget, quality gates, and lifecycle policy in a single spec. The reconciler provisions per-teammate git worktrees, scopes each pod with its own ServiceAccount, and re-spawns crashed agents using the durable task list as recovery state. The agent does not remember the conversation, but the task list on the PVC tells the fresh pod what work remains. The talk walks through the architectural choices that made this work in K8s: why agent state lives on a PVC instead of a CRD status field, why RestartPolicy is Never, what RWX storage you actually need in production versus on a laptop, and how Prometheus metrics, webhooks, and human approval gates plug into the reconcile loop. A live demo deploys a coding team to a Kind cluster, shows mailbox traffic between pods, kills a teammate, and watches the operator respawn it from the task list.

(~1,290 characters, against a 1,300 limit. The buffer is small. Trim if any field is added during iteration.)

---

## Audience

> Platform engineers, operator authors, and SREs who run Kubernetes and are evaluating how to host multi-agent LLM workloads without building a custom protocol. Attendees should be comfortable with the operator pattern (CRDs, controllers, reconcile loops), Kubernetes RBAC, and PVC access modes. Familiarity with Claude Code or Agent Teams is helpful but not required; the talk explains the native protocol and the K8s primitives it maps to. Attendees will leave with a clear picture of which Kubernetes building blocks translate cleanly to agent workloads (git worktrees as a concurrency primitive, ServiceAccounts as per-agent capability boundaries, owner references for cascade deletion of team state) and which assumptions break down at scale (CRD status as long-running state, single-node RWO fallbacks, real-time cost tracking).

---

## Benefits to the ecosystem (1,000 char max)

> Cloud-native multi-agent systems are a 2026 priority for both CNCF and individual platform teams, but most current solutions invent new orchestration protocols and layer them on top of Kubernetes. This talk demonstrates the alternative: model the agent team as a first-class Kubernetes resource and let existing primitives do the coordination work. The architectural patterns generalize beyond Claude Code; any multi-agent system with file-based or shared-state coordination can adopt the same approach. The talk surfaces the honest tradeoffs (ReadWriteMany storage cost, estimation-based budget tracking, the limits of CRD status for long-running state) so attendees can evaluate whether the pattern fits their workloads. The operator is open source under Apache 2.0, ships with a published Helm chart and Prometheus dashboard, and gates every release on a real-Claude end-to-end test in CI.

(~970 characters)

---

## Open source projects discussed

- [claude-teams-operator](https://github.com/amcheste/claude-teams-operator) — the operator itself (Apache 2.0)
- [Kubernetes](https://github.com/kubernetes/kubernetes) — the platform; specifically `controller-runtime`, `kubebuilder`, RBAC, PVC subsystem
- [Prometheus](https://github.com/prometheus/prometheus) and [Grafana](https://github.com/grafana/grafana) — metrics scraping and the published dashboard ConfigMap
- [Helm](https://github.com/helm/helm) — chart packaging and release distribution
- Anthropic's Claude Code Agent Teams protocol — the native file-based coordination format the operator preserves (Claude Code itself is not open source; the protocol behavior is documented and stable enough to wrap as-is)

---

## Reviewer-facing talk outline (~30 min)

This expands on the abstract — provided in case the Sessionize form exposes a longer description field, and to anchor the demo plan.

| Time | Beat |
|------|------|
| 0:00 | The problem framing. Most agent frameworks bolt onto Kubernetes; this talk argues for the inverse — Kubernetes primitives doing the coordination work. |
| 2:00 | Native Agent Teams in 60 seconds: file-based JSON mailboxes, shared task list, no session resumption. Why this protocol is unusually well-suited to a shared filesystem. |
| 5:00 | The `AgentTeam` CRD: one spec for a whole team (lead + teammates + lifecycle + budget). Contrast with agent-as-a-resource designs. |
| 8:00 | Phase state machine: `Pending → Initializing → Running → Completed/Failed/TimedOut/BudgetExceeded`. How state transitions map to actual K8s objects (PVCs, init Job, pods). |
| 11:00 | The ReadWriteMany requirement, in detail. Why coordination over a PVC actually works. What fails on RWO. The single-node RWO fallback used in CI and what it can and cannot prove. |
| 14:00 | Per-agent RBAC. Each pod gets its own ServiceAccount with `resourceNames`-restricted Roles on the secrets and PVCs it owns. A free security win that non-native orchestrators have to reinvent. |
| 16:00 | **Demo 1 — Crash recovery.** Deploy a coding team. Show mailbox files appearing on the PVC. Kill a teammate pod. Watch the reconciler respawn it. The fresh agent has no conversation memory, but the task list tells it what is left. |
| 21:00 | `onComplete` actions: `create-pr` opens a real GitHub PR via the REST API; `push-branch` consolidates per-teammate worktree branches into one head via a Job. The worktree-as-concurrency-primitive story. |
| 24:00 | **Demo 2 — Observability.** Prometheus metrics, the Grafana dashboard ConfigMap, an approval gate firing a webhook before a sensitive teammate spawns. |
| 27:00 | Honest tradeoffs we are still working through: estimation-based budget tracking, real multi-node test coverage, the limits of CRD status as a substitute for a workflow engine. |
| 29:00 | Wrap and pointers (repo, Helm chart, contributor docs). |
| 30:00 | Q&A. |

---

## Demo plan

Two demos, both runnable on a laptop with Kind:

1. **Crash recovery (5 min, on stage).** Deploy a 3-agent `AgentTeam` from a sample manifest, watch pods come up, observe mailbox JSON appearing on the shared PVC, `kubectl delete pod` one teammate, watch the reconciler respawn it. The point is to show the agent's lost context window does not lose the team's progress, because the task list is durable.

2. **Observability and gates (3 min, on stage).** Bring up the Grafana dashboard against the operator's Prometheus metrics. Trigger an approval gate so a webhook fires; grant approval via `kubectl annotate`; watch the gated teammate spawn.

Both demos run today against the shipped v0.5.0 release. Backup recordings will be prepared in case live demo bandwidth fails on the venue Wi-Fi.

---

## Speaker bio

> _TBD — see open questions._

---

## Prior speaking history

> _TBD — see open questions._

---

## Open questions for the maintainer

These are the items that need maintainer input before submission:

1. **Speaker bio** — short paragraph (≤ ~500 chars) covering current role, relevant background, and any past public talks or projects. Include a recent headshot upload-ready.
2. **Prior speaking history** — has the maintainer presented at a CNCF event in the past 12 months? The form asks for video links if so.
3. **Track preference** — primary recommendation here is **AI Inference + Agentic**; the alternate is **Platform Engineering**. Which one does the maintainer want as the primary track? (Submitting to one does not preclude the program committee from re-routing.)
4. **Title preference** — three candidates above. Maintainer's call.
5. **Co-speaker?** — solo or two-speaker? The form allows up to two on a Session Presentation.
6. **Tutorial alternate?** — if the talk lands strongly, a 75-minute Tutorial slot is also viable (deploy a team in real time, walk through the CRD field by field). Worth submitting both? The CFP allows up to three submissions per speaker.
7. **Demo cluster** — confirm the on-stage cluster is Kind on a laptop, vs. an actual cloud cluster. Bandwidth and predictability favor Kind; "real cluster" favors the multi-node RWX story.
8. **Release alignment** — the v0.6.0 (Operator Dashboard) and v1.0.0 (Demo Polish) milestones land before the conference. Should the dashboard be part of the demo, or kept as a parallel track? Including it strengthens the story but adds a moving piece to rehearse.

---

## Notes on substance

Everything in the abstract and the outline maps to shipped, tested code in v0.1.0–v0.5.0:

- AgentTeam CRD with single-spec team declaration → [api/v1alpha1/agentteam_types.go](../../api/v1alpha1/agentteam_types.go)
- Reconciler phase state machine → [internal/controller/agentteam_controller.go](../../internal/controller/agentteam_controller.go), see also [ARCHITECTURE.md § Phase State Machine](../../ARCHITECTURE.md#phase-state-machine)
- ReadWriteMany PVC coordination + single-node fallback → [ARCHITECTURE.md § Storage Requirements](../../ARCHITECTURE.md#storage-requirements), [hack/mailbox-smoke-test.sh](../../hack/mailbox-smoke-test.sh)
- Per-agent ServiceAccounts with `resourceNames`-restricted Roles — shipped in v0.4.0 (#14)
- Crash respawn with restart counters — v0.4.0 (#13)
- `onComplete: create-pr` — v0.4.0 (#15); `onComplete: push-branch` — v0.4.0 (#16)
- Prometheus metrics + Grafana dashboard ConfigMap — v0.3.0
- Webhook engine + approval gates — v0.3.0
- AgentTeamTemplate + AgentTeamRun controllers — v0.5.0 (#17, #18)
- Real-Claude E2E gate before release publishes — v0.4.0 (#150)

No claim in this draft refers to unshipped work.
