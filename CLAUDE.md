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

`claude.amcheste.io/v1alpha1`

## Testing

- Unit tests for reconciler logic (mock client)
- Integration tests with envtest (controller-runtime test framework)
- E2E tests against Kind cluster with real Claude Code (requires API key)

## License

Apache 2.0 — CAM Labs LLC

---

## KubeCon NA 2026

This project is being developed with the goal of presenting at KubeCon NA 2026 (November 9–12, Salt Lake City). See `KUBECON.md` for the full talk framing.

### Release Timeline

All milestones and issues are tracked on GitHub. The CFP is **OPEN** with submissions due **May 31 2026 at 11:59pm MT** — see KUBECON.md.

| Version | GitHub Milestone | Due | What it unlocks |
|---------|-----------------|-----|-----------------|
| **v0.1.0** ✅ | Initial Release | Apr 13 2026 | Core operator, 50+25+19 tests, CI |
| **v0.2.0** ✅ | Foundation & Real Runner | Apr 19 2026 | Real `claude-code-runner` image, E2E test, mailbox PVC validation, talk-ready `describe` output |
| **v0.3.0** ✅ | Observability & Budget | Apr 24 2026 | Prometheus metrics, `internal/budget` package, webhook engine, Grafana dashboard (plus early Resilience/RBAC previews) |
| **v0.4.0** | Resilience & RBAC | Aug 31 2026 | Crash re-spawn ✅, per-agent ServiceAccounts ✅, `onComplete: create-pr` ✅, `onComplete: push-branch` |
| **v0.5.0** | Template Engine & Helm | Sep 30 2026 | `AgentTeamTemplate`/`AgentTeamRun` controllers, production Helm chart, CONTRIBUTING.md |
| **v0.6.0** | Operator Dashboard | Oct 5 2026 | Web UI for running AgentTeams: backend API, list + detail views (HTMX + Go templates), live SSE updates, Helm packaging |
| **v1.0.0** | KubeCon Demo Polish | Oct 26 2026 | Demo script, CFP submitted, OCI skill distribution, dashboard presentation mode for stage |

**KubeCon talk:** November 9–12 2026, Salt Lake City. CFP deadline: May 31 2026.

### Current Priority (post-v0.3.0)

The next highest-value issues:
1. **#16** — `onComplete: push-branch` — closes out v0.4.0 alongside the already-merged #13/#14/#15
2. **#17 / #18** — AgentTeamTemplate + AgentTeamRun controllers (v0.5.0)
3. **#137–#140** — the operator dashboard (v0.6.0)
4. **#23** — draft and submit the KubeCon CFP by May 31 — this is the hard deadline

### Ask of Claude Code

As you build, help capture the story. When you hit something non-obvious — a surprising constraint, a design tradeoff, an elegant solution, or something that broke in an unexpected way — add a short entry to the **"Interesting Problems Encountered"** section in `KUBECON.md`. One paragraph is enough. These notes become the raw material for the talk narrative.

Specifically worth logging:
- Anything awkward about modeling long-running agent state in a K8s reconciler
- Constraints imposed by the RWX PVC requirement
- Tradeoffs in the budget estimation approach
- Anything surprising about crash recovery / re-spawn behavior
- Moments where K8s primitives (RBAC, ServiceAccounts, worktrees) solved a problem elegantly
