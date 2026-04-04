# KubeCon NA 2026 — Talk Prep

**Conference:** KubeCon + CloudNativeCon North America 2026
**Dates:** November 9–12, 2026 — Salt Lake City, Utah
**CFP:** Expected to open May/June 2026. Watch https://events.linuxfoundation.org/kubecon-cloudnativecon-north-america/

---

## Talk Concept

**Working Title:** *Cloud-Native AI Teams: Building a Kubernetes Operator for Orchestrating Claude Code Agent Squads*

**The pitch:** Most AI agent orchestration frameworks treat Kubernetes as an afterthought — they bolt LLM workloads onto K8s rather than building cloud-native. This talk shows what you gain by going the other way: designing a proper K8s operator where agent teams are first-class resources with CRDs, reconciliation loops, RBAC, and native observability.

**The dogfooding hook:** The operator was built *with* Claude Code agent teams — the same system it's designed to run. That recursive story is compelling and unusual.

**Target audience:** Platform engineers, SRE teams, and developers curious about running AI agent workloads at scale.

**Format:** 35-minute talk (preferred) or 25-minute slot. Demo-heavy.

---

## Key Themes to Surface During Development

These are the angles that will make the talk land. When you hit one of these in the code, log it in the "Interesting Problems" section below.

1. **State management is the hard part** — K8s reconcilers are designed for short, idempotent operations. Agent turns are long-running and stateful. How do you model "agent is mid-conversation" in a CRD status?

2. **The ReadWriteMany constraint** — Requiring RWX PVCs (NFS, EFS) is a real deployment cost. What does this mean for clusters that don't have it? Are there workarounds?

3. **Budget tracking without native APIs** — Claude Code doesn't expose real-time token counts externally. Estimation-based tracking is a pragmatic compromise worth explaining.

4. **Per-agent RBAC** — K8s ServiceAccounts naturally scope what each agent pod can touch. This is a free security win you don't get with non-native orchestrators.

5. **Crash recovery and re-spawn** — `RestartPolicy: Never` + operator re-spawn with fresh context. What does the agent "remember" via the task list vs. what's lost?

6. **Git worktrees as concurrency primitive** — Using worktrees to prevent merge conflicts between concurrent agents is an elegant solution worth highlighting.

7. **The operator-as-coordinator pattern** — The operator isn't just lifecycle management; it's the coordination bus. Contrast this with peer-to-peer agent architectures.

---

## Interesting Problems Encountered

*Claude Code: when you hit something non-obvious, surprising, or that required a design decision, log it here with a brief note. These are the raw materials for the talk narrative.*

<!-- FORMAT:
### [Date] Short title
What happened, what you tried, what you decided. One paragraph is enough.
-->


---

## Demo Milestones

Track what's working and could be shown on stage.

- [ ] CRDs install cleanly (`kubectl apply -f config/crd/`)
- [ ] Operator starts and watches `AgentTeam` resources
- [ ] PVC creation + init Job (Phase 1 reconciler)
- [ ] Lead + teammate pods spin up from a sample `AgentTeam`
- [ ] Mailbox files appear on the shared PVC between pods
- [ ] A real coding task runs end-to-end in Kind
- [ ] `kubectl describe agentteam` shows meaningful status
- [ ] Budget estimation visible in status / metrics
- [ ] Prometheus metrics scraping
- [ ] Clean demo script (2-minute happy path for on-stage use)

---

## Proposal Draft

*Fill this in once the demo milestones are mostly green.*

**Title:**

**Abstract (500 chars):**

**Talk outline:**

**Speaker bio:**

**Demo description:**

---

## Useful Links

- CNCF CFP: https://sessionize.com/kubecon-cloudnativecon-north-america-2026/
- LF Events page: https://events.linuxfoundation.org/kubecon-cloudnativecon-north-america/
- CNCF talk submission tips: https://www.cncf.io/blog/2023/11/13/tips-for-submitting-a-talk-to-kubecon-cloudnativecon/
- Prior art: KubeCon talks on AI/ML operators for reference framing
