# claude-teams-operator

A Kubernetes operator that distributes [Claude Code Agent Teams](https://code.claude.com/docs/en/agent-teams) across a K8s cluster.

> **Status:** Early development — CRD types defined, controller stubbed, ready for implementation.

## What This Does

Claude Code Agent Teams let multiple Claude Code instances work together on a shared codebase — with a team lead coordinating work via a shared task list and peer-to-peer messaging. Natively, this runs on a single machine via tmux.

This operator lifts that pattern into Kubernetes:

- Each agent (lead + teammates) runs as a separate **Pod**
- The native file-based mailbox protocol is preserved via **shared PVCs**
- Each teammate gets an isolated **git worktree** to prevent merge conflicts
- Built-in **budget limits**, **quality gates**, and **webhook notifications**
- Define reusable team patterns with **AgentTeamTemplate** CRDs

## Quick Start

```bash
# 1. Create a Kind cluster with NFS support
make kind-create

# 2. Build and load images
make docker-build docker-build-runner kind-load

# 3. Install CRDs and deploy operator
make install deploy

# 4. Create your API key secret
kubectl create secret generic anthropic-api-key \
  --namespace dev-agents \
  --from-literal=ANTHROPIC_API_KEY=sk-ant-...

# 5. Deploy a sample team
kubectl apply -f config/samples/auth-refactor-team.yaml

# 6. Watch the team
kubectl get agentteams -n dev-agents -w
```

## CRDs

| CRD | Purpose |
|-----|---------|
| `AgentTeam` | Full team definition: lead + teammates + repo + budget |
| `AgentTeamTemplate` | Reusable team pattern |
| `AgentTeamRun` | Instantiate a template against a specific repo/branch |

## Requirements

- Kubernetes 1.28+
- ReadWriteMany PVC support (NFS, EFS, etc.)
- Claude Code CLI access (Max subscription or API key)
- Opus 4.6 model access (required for Agent Teams)

## License

Apache 2.0 — [CAM Labs LLC](https://github.com/camlabs)
