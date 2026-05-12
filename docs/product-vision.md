# Product Vision: Knowledge Work Orchestrator

**Document owner:** Alan Chester, CAM Labs LLC
**Last updated:** 2026-05-12
**Status:** Draft
**Linear milestone:** v0.8.0 — Knowledge Work Orchestrator

---

## The Problem

Knowledge work in organizations is manual, unscalable, and unrepeatable. Teams spend hours every week on recurring tasks that follow predictable patterns: compiling reports from multiple data sources, drafting follow-up communications after meetings, analyzing competitive landscapes, onboarding new clients with standard document packages, and summarizing research across departments.

These tasks share three characteristics that make them candidates for AI orchestration:

1. **They decompose into parallel subtasks.** A quarterly report requires a data analyst, a writer, and a reviewer. These roles can work concurrently on different parts of the deliverable.

2. **They follow repeatable pipelines.** The steps are the same every quarter: gather data, analyze trends, write narrative, review, distribute. The content changes but the workflow does not.

3. **They require human judgment at specific gates.** Not everything should be autonomous. Sending external emails, publishing reports, and making financial projections all benefit from a human checkpoint before execution.

Today, the options for orchestrating these tasks with AI are either purely local (Claude Code, ChatGPT) or framework-heavy (LangGraph, CrewAI) with no native deployment model. There is no Kubernetes-native platform that lets you define a knowledge work team declaratively and run it on your cluster with built-in cost controls, observability, and human-in-the-loop gates.

## The Vision

**claude-teams-operator is a Kubernetes-native platform for orchestrating AI knowledge work teams.** You define what your teams do. The cluster runs them, tracks costs, keeps humans in the loop, and delivers results automatically.

The operator treats AI agent teams the way Kubernetes treats applications: as declarative resources with well-defined lifecycle, health monitoring, and scaling characteristics. An `AgentTeam` is a first-class Kubernetes resource, just like a Deployment or a CronJob.

## Positioning

Gas Town orchestrates coding agents. We orchestrate knowledge work.

Gas Town's question: "How do I run 50 coding agents against my codebase?"

Our question: "How do I define a team of AI specialists that produces a quarterly report every Monday at 6am, with human approval before the email goes out?"

The one-liner: **"Gas Town runs your coding agents. We run your business."**

## Target Users

**Platform engineers** who need to deploy and manage AI agent workloads alongside existing Kubernetes infrastructure. They care about CRDs, Helm charts, RBAC, Prometheus metrics, and resource limits.

**Operations teams** who define and run recurring business processes. They care about scheduling, approval workflows, delivery targets, and cost visibility.

**Knowledge workers** (via platform team enablement) who consume AI-generated deliverables: reports, summaries, analyses, drafted communications. They interact through approval gates and output delivery, not kubectl.

## Product Principles

1. **Cloud-native first.** Every capability is expressed through Kubernetes primitives. Teams are CRDs. Communication is shared PVCs. Scheduling is CronJob-patterned. Observability is Prometheus. No sidecar frameworks, no external databases required for basic operation.

2. **Human-in-the-loop by design.** Approval gates are first-class. The default posture is that consequential actions (sending emails, publishing documents, making external API calls) require human approval. Autonomous execution is opt-in, not opt-out.

3. **Observable and auditable.** Every team run is a Kubernetes resource with full status, events, logs, and cost tracking. Platform teams can see exactly what their AI teams are doing, what they cost, and what they produced.

4. **Composable.** Templates let you define team patterns once and instantiate them against different contexts. Pipelines compose stages. Skills are pluggable. MCP servers connect to external tools. The system is designed to be assembled, not prescribed.

5. **Coding is a supported mode, not the only mode.** The operator's heritage is Claude Code Agent Teams. Coding workflows (git worktrees, PR creation, test validation) remain fully supported. But the primary positioning is knowledge work.

## Competitive Landscape

| Product | What it does | Gap we fill |
|---------|-------------|-------------|
| Gas Town + Operator | Coding agent swarms on K8s | No knowledge work mode, no pipelines, no scheduling, no delivery |
| Claude Code Agent Teams | Local multi-agent coding | Single machine only, no K8s, no scheduling, no Cowork mode |
| Claude Cowork | Desktop knowledge work | Single user, no K8s, no scheduling, no team coordination |
| CrewAI | Role-based agent framework | No K8s-native deployment, no CRDs, no built-in scheduling |
| LangGraph | Graph-based agent workflows | Framework, not platform. No K8s operator, no built-in lifecycle |
| Multiclaude | Multi-agent coding orchestrator | Local only, coding-focused, no knowledge work features |

## Use Cases

**Recurring reports:** A weekly standup summary team runs every Monday morning. It reads Slack channels, synthesizes highlights, and delivers a summary to the team lead via email.

**Event-triggered workflows:** When a new deal closes in the CRM, a webhook triggers an onboarding team that generates a welcome packet, drafts introductory emails, and prepares a project kickoff document.

**Multi-stage analysis:** A competitive intelligence pipeline fans out three analysts (market, financial, competitive) in parallel, then merges their findings into a synthesis report with executive summary.

**Document pipelines:** A research team gathers data, a writer produces a draft, a reviewer provides feedback, and a final version is delivered to Google Drive with a Slack notification.

## Success Metrics

- Number of AgentTeam runs per week (adoption)
- Artifacts delivered successfully (reliability)
- Cost per artifact (efficiency)
- Mean time from team start to delivery (speed)
- Approval gate response time (human-in-the-loop friction)
- Template reuse rate (composability)

## Roadmap Context

This vision is implemented primarily in the **v0.8.0 milestone** (target: 2026-08-15), building on the foundation of v0.1.0 through v0.7.0 which established the core operator, Cowork mode, Skills, MCP servers, approval gates, the dashboard, and the documentation site. The v1.0.0 milestone (KubeCon Demo Polish) follows with production hardening and the conference presentation.

## Related Documents

- [Knowledge Work PRD](./knowledge-work-prd.md) — detailed requirements
- [Knowledge Work Design](./knowledge-work-design.md) — technical architecture
- [ARCHITECTURE.md](../ARCHITECTURE.md) — current operator architecture
- [KUBECON.md](../KUBECON.md) — conference talk framing
