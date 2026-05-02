# kagents

**Run Claude Code Agent Teams as a Kubernetes operator.**

`kagents` brings Anthropic's native Claude Code Agent Teams pattern into Kubernetes. A lead agent coordinates work via a shared task list while teammate agents communicate through file-based mailboxes — the same coordination protocol as the local tmux experience, just running as pods on your cluster.

!!! note "Site under construction"
    This site is being populated as part of the [v0.7.0 milestone](https://github.com/amcheste/claude-teams-operator/milestone/8). Until then, see the [README on GitHub](https://github.com/amcheste/claude-teams-operator) for installation and usage details.

## Quick install

```bash
helm install kagents \
  oci://ghcr.io/amcheste/charts/claude-teams-operator \
  --namespace claude-teams-system --create-namespace
```

## Why kagents

- **Native protocol fidelity** — wraps Anthropic's file-based mailbox protocol exactly as designed; no custom RPC layer to maintain
- **Team as a first-class resource** — one `AgentTeam` CRD declares roles, budget, quality gates, coordination topology
- **Kubernetes as coordination fabric** — ServiceAccounts scope agent capabilities, RWX PVCs hold the shared mailboxes, RBAC enforces per-agent boundaries
- **Dogfooded** — built with the same agent-teams system it operates

## Repository

[github.com/amcheste/claude-teams-operator](https://github.com/amcheste/claude-teams-operator) — Apache 2.0
