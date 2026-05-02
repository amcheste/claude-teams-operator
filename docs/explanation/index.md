# Explanation

The "why" behind kagents — architecture, design tradeoffs, the choices that shaped the project. Read these when you want to understand what's actually happening, not just how to use it.

## Coming in v0.7.0

- **Resource model** — how `AgentTeam`, `AgentTeamTemplate`, and `AgentTeamRun` relate; when to reach for which
- **Coordination protocol** — the file-based mailbox model, why ReadWriteMany is required, per-teammate git worktrees as a concurrency primitive
- **Operations** — budget estimation, per-agent RBAC, observability via Prometheus + Grafana

Until these land, [ARCHITECTURE.md in the repo](https://github.com/amcheste/claude-teams-operator/blob/main/ARCHITECTURE.md) is the most complete design document.
