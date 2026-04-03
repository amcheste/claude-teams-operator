# claude-teams-operator

A Kubernetes operator that runs Claude Code Agent Teams as distributed pods on a K8s cluster.

## Project Overview

This operator brings Anthropic's native Claude Code Agent Teams feature — which normally runs locally via tmux — into Kubernetes. It preserves the native coordination protocol (file-based JSON mailboxes + shared task list) by mounting shared ReadWriteMany PVCs across agent pods.

### Architecture

- **Operator** (Go, controller-runtime) watches `AgentTeam` CRDs and reconciles pods
- **Lead pod** runs Claude Code as team lead with `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1`
- **Teammate pods** run Claude Code instances that communicate via shared filesystem
- **Shared PVCs** hold the mailbox JSON files (`~/.claude/teams/`) and task list (`~/.claude/tasks/`)
- **Repo PVC** holds the git clone with per-teammate worktrees

### CRDs

- `AgentTeam` — primary resource: defines lead + teammates + repo + budget + quality gates
- `AgentTeamTemplate` — reusable team patterns (e.g., "3-agent security review")
- `AgentTeamRun` — instantiates a template against a specific repo/branch

## Build & Test Commands

```bash
make build          # Build operator binary
make test           # Run tests
make lint           # Run linter
make manifests      # Generate CRD manifests from Go types
make generate       # Generate deepcopy methods
make docker-build   # Build operator container image
make docker-build-runner  # Build Claude Code runner image
make kind-create    # Create Kind dev cluster with NFS
make kind-load      # Load images into Kind
make install        # Install CRDs
make deploy         # Deploy operator
make sample         # Deploy sample AgentTeam
```

## Key Design Decisions

1. **File-based mailbox on shared PVC** — matches native Agent Teams protocol exactly. Each agent reads/writes JSON inbox files at `~/.claude/teams/{team}/inboxes/{agent}.json`. No protocol translation needed.

2. **ReadWriteMany PVC required** — multiple pods must read/write the same mailbox files. Requires NFS, EFS, or similar CSI driver. The Kind setup script installs an NFS provisioner.

3. **Per-teammate git worktrees** — prevents merge conflicts between concurrent agents. Each teammate works on an isolated worktree branched from the target.

4. **Budget tracking is estimation-based** — Claude Code doesn't expose real-time token counts externally. Operator estimates from session duration × model cost rates.

5. **Pods use RestartPolicy: Never** — Agent Teams can't resume sessions. If a pod crashes, the operator re-spawns with fresh context and the task list tells it what's left to do.

## File Structure

```
api/v1alpha1/           # CRD type definitions (kubebuilder markers)
  agentteam_types.go    # AgentTeam spec + status
  template_types.go     # AgentTeamTemplate + AgentTeamRun
  groupversion_info.go  # Scheme registration

internal/controller/    # Reconciliation logic
  agentteam_controller.go  # Main reconciler (TODO: implement phases)

internal/claude/        # Claude Code interaction helpers (TODO)
  session.go            # Session lifecycle
  mailbox.go            # Mailbox JSON I/O
  tasklist.go           # Task list JSON I/O
  worktree.go           # Git worktree management

internal/budget/        # Token usage + cost estimation (TODO)
internal/webhook/       # Slack/webhook notifications (TODO)
internal/metrics/       # Prometheus metrics (TODO)

cmd/manager/main.go     # Operator entrypoint
docker/                 # Dockerfiles for operator + runner
hack/                   # Dev scripts (Kind setup)
config/samples/         # Example CRs
charts/                 # Helm chart (TODO)
```

## Implementation Priority

The reconciler in `internal/controller/agentteam_controller.go` has TODO stubs for each phase:

1. **reconcilePending** — Create PVCs, run init Job (clone repo, create worktrees)
2. **reconcileInitializing** — Wait for init, deploy lead + teammate pods
3. **reconcileRunning** — Monitor health, track budget, handle crashes, check completion
4. **reconcileTerminal** — Cleanup pods, archive logs, create PR

Start with Phase 1 (PVC creation + init Job), then work through sequentially.

## Dependencies

- Go 1.23+
- controller-runtime v0.20
- kubebuilder markers for CRD generation
- Kind + Helm for local development
- NFS provisioner for ReadWriteMany PVCs

## API Group

`claude.camlabs.dev/v1alpha1`

## Testing

- Unit tests for reconciler logic (mock client)
- Integration tests with envtest (controller-runtime test framework)
- E2E tests against Kind cluster with real Claude Code (requires API key)

## License

Apache 2.0 — CAM Labs LLC
