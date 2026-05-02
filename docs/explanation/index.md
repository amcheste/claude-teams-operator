# Explanation

The "why" behind kagents — architecture, design tradeoffs, the choices that shaped the project. Read these when you want to understand what's actually happening, not just how to use it.

## Pages

- **[Resource model](resources.md)** — the three CRDs (`AgentTeam`, `AgentTeamTemplate`, `AgentTeamRun`), how they relate, and when to reach for which.
- **[Coordination protocol](coordination.md)** — the file-based mailbox model, why ReadWriteMany is required, per-teammate git worktrees as a concurrency primitive.
- **[Operations](operations.md)** — budget estimation, per-agent RBAC, observability via Prometheus + Grafana + webhooks.

## Going deeper

The repo's [`ARCHITECTURE.md`](https://github.com/amcheste/claude-teams-operator/blob/main/ARCHITECTURE.md) is the design doc — denser, more focused on rationale than on usage. It overlaps with these pages but goes further into the file-by-file structure of the codebase.

The [KubeCon NA 2026 talk](https://github.com/amcheste/claude-teams-operator/blob/main/KUBECON.md) frames the same architecture from the conference angle (interesting problems encountered, competitive landscape, design decisions worth surfacing on stage).
