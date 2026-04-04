# Testing

The project has three test tiers. Run them in order from fastest to slowest:

| Tier | Command | Requires | Speed |
|------|---------|----------|-------|
| Unit | `make test` | Nothing | ~1s |
| Integration | `make test-integration` | `setup-envtest` binary | ~30s |
| Acceptance | `make test-acceptance` | Kind cluster (`make acceptance-up`) | ~5–10 min |

---

## Unit Tests

No cluster, no API server, no external dependencies. Uses controller-runtime's fake client (fully in-memory).

```bash
make test

# Verbose output
go test ./internal/controller/... -v

# Run a single test
go test ./internal/controller/... -run TestReconcilePending_CodingMode

# Coverage report
go test ./internal/controller/... -coverprofile=cover.out && go tool cover -html=cover.out
```

### What is covered (36 tests)

#### Reconciler phases

| Test | What it verifies |
|------|-----------------|
| `TestReconcilePending_CodingMode_CreatesPVCsAndInitJob` | team-state PVC, repo PVC, and init Job created in coding mode |
| `TestReconcilePending_CoworkMode_CreatesOutputPVC` | output PVC created; no repo PVC or init Job in Cowork mode |
| `TestReconcilePending_Idempotent` | calling reconcilePending twice does not error |
| `TestReconcileInitializing_WaitsForInitJob` | returns requeue when init Job is still running |
| `TestReconcileInitializing_InitJobFailed_SetsFailedPhase` | sets `Failed` when init Job exhausts backoff |
| `TestReconcileInitializing_DeploysPods` | deploys lead and teammate pods after init Job succeeds |
| `TestReconcileInitializing_DependsOnBlocks` | does not spawn a teammate whose `dependsOn` dep has not succeeded |
| `TestReconcileInitializing_ApprovalGateBlocksTeammate` | blocks a gated teammate and sets `pendingApproval` |
| `TestReconcileInitializing_ApprovalGrantedViaAnnotation` | spawns gated teammate when approval annotation is present |
| `TestReconcileRunning_Timeout_SetsTimedOut` | sets `TimedOut` when elapsed time exceeds `lifecycle.timeout` |
| `TestReconcileRunning_BudgetExceeded_SetsBudgetExceeded` | sets `BudgetExceeded` when estimated cost exceeds `lifecycle.budgetLimit` |
| `TestReconcileRunning_AllPodsSucceeded_SetsCompleted` | sets `Completed` when lead and all teammates are `Succeeded` |
| `TestReconcileRunning_PodFailed_SetsFailedPhase` | sets `Failed` when any pod enters `Failed` phase |
| `TestReconcileRunning_SpawnsNewlyUnblockedTeammate` | spawns a teammate mid-run once its `dependsOn` dep completes |

#### Pod builder

| Test | What it verifies |
|------|-----------------|
| `TestBuildAgentPod_CodingMode` | correct env vars, `WORKTREE_PATH`, and volume mounts for coding mode |
| `TestBuildAgentPod_LeadHasNoWorktreePath` | lead pod does not receive `WORKTREE_PATH` |
| `TestBuildAgentPod_WithSkills` | skill ConfigMaps mounted at `/var/claude-skills/{name}` |
| `TestBuildAgentPod_WithMCPServers` | `mcp-config` volume mounted at `/var/claude-mcp` |
| `TestBuildAgentPod_CoworkMode` | workspace-output and workspace-input volumes present; no repo volume |
| `TestBuildAgentPod_ScopeEnvVars` | `SCOPE_INCLUDE_PATHS` and `SCOPE_EXCLUDE_PATHS` set from `scope` spec |

#### Business logic

| Test | What it verifies |
|------|-----------------|
| `TestEstimateCost_ZeroWhenNoStartTime` | returns `"0.00"` when team has not started |
| `TestEstimateCost_OpusMoreExpensiveThanSonnet` | opus cost > sonnet cost for same elapsed time |
| `TestIsTimedOut_NotTimedOut` | returns false when elapsed < timeout |
| `TestIsTimedOut_TimedOut` | returns true when elapsed > timeout |
| `TestIsTimedOut_NoStartTime` | returns false when `startedAt` is unset |
| `TestIsBudgetExceeded_UnderBudget` | returns false when cost < limit |
| `TestIsBudgetExceeded_OverBudget` | returns true when cost >= limit |
| `TestIsBudgetExceeded_NoBudgetLimit` | returns false when no budget limit is configured |
| `TestDependenciesMet_NoDeps` | returns true for empty or nil dependency list |
| `TestDependenciesMet_DepSucceeded` | returns true when dependency pod is `Succeeded` |
| `TestDependenciesMet_DepNotSpawned` | returns false when dependency pod does not exist |
| `TestDependenciesMet_DepStillRunning` | returns false when dependency pod is `Running` |
| `TestCheckApprovalGate_NoGateDefined` | returns approved when no gate matches the event |
| `TestCheckApprovalGate_GatePresentNotApproved` | returns not approved when gate exists but annotation absent |
| `TestCheckApprovalGate_ApprovedViaAnnotation` | returns approved when annotation is set to `"true"` |

### Test helpers

```go
minimalTeam("my-team")               // minimal valid AgentTeam
withRepo(team)                        // add coding-mode repository config
withWorkspace(team)                   // add Cowork-mode workspace config
withLifecycle(team, "1h", "10.00")   // set timeout and budget

succeededPod("my-team-lead", "default", "my-team")
failedPod("my-team-lead", "default", "my-team")
runningPod("my-team-lead", "default", "my-team")

completedJob("my-team-init", "default")
failedJob("my-team-init", "default")

r := newReconciler(team, leadPod, workerPod)  // reconciler with pre-populated state
team = fetch(t, r, "my-team")                 // fetch fresh copy with ResourceVersion set
```

### Writing new unit tests

1. Build the initial state using helpers above.
2. Create the reconciler with `newReconciler(objs...)`.
3. Fetch the team with `fetch(t, r, name)`.
4. Set any pre-existing status directly on the fetched object.
5. Call the method under test (e.g. `r.reconcilePending(ctx, team)`).
6. Assert both in-memory state and cluster state via `r.Get`.

```go
func TestMyNewScenario(t *testing.T) {
    team := withRepo(minimalTeam("my-team"))
    r := newReconciler(team)
    team = fetch(t, r, "my-team")

    _, err := r.reconcilePending(context.Background(), team)
    require.NoError(t, err)

    assert.Equal(t, "Initializing", team.Status.Phase)

    var pvc corev1.PersistentVolumeClaim
    require.NoError(t, r.Get(context.Background(),
        types.NamespacedName{Name: "my-team-team-state", Namespace: "default"}, &pvc))
}
```

---

## Integration Tests

Uses [controller-runtime envtest](https://book.kubebuilder.io/reference/envtest.html) — a real in-process API server with CRDs installed. Tests the full watch/reconcile loop over multiple reconcile cycles. No Kind cluster needed.

```bash
# First time: install setup-envtest
make envtest

# Run all 25 integration tests
make test-integration
```

### What is covered (25 specs across 6 Describe blocks)

| Block | Specs |
|-------|-------|
| Pending — coding mode | PVCs + init Job created, phase transitions, owner refs |
| Pending — Cowork mode | output PVC only; no repo PVC or init Job |
| Initializing | waits while Job running; Failed on Job failure; deploys pods; Running phase |
| Running | Completed on all-succeeded; Failed on pod failure; completedAt stamped |
| DependsOn | second teammate blocked until first pod Succeeded |
| Approval gates | blocked + pendingApproval status; spawned after annotation |
| CRD validation | invalid model enum rejected; empty teammates rejected |

### Integration test helpers

```go
testNS()                                          // unique namespace + DeferCleanup
waitForPhase(name, namespace, "Running")
waitForPVC(name, namespace)
waitForJob(name, namespace)
waitForPod(name, namespace)
completeJob(name, namespace)
failJob(name, namespace)
succeedPod(name, namespace)
failPod(name, namespace)
advanceThroughInit(name, namespace)               // complete init Job + wait for Running
```

---

## Acceptance Tests

Runs against a **real Kind cluster** with the operator deployed as a Kubernetes Deployment. Tests exercise actual RBAC, real image scheduling, and real pod lifecycle events. This is the release gate — it runs automatically on every PR from `develop` → `main`.

### Local setup

```bash
# One-time: create Kind cluster + build + deploy operator
make acceptance-up

# Run all acceptance tests
make test-acceptance

# Tear down when done
make acceptance-down
```

The operator is deployed with `--agent-image=busybox:latest --skip-init-script` so containers actually exit 0 (driving real phase transitions) without needing an Anthropic API key or a real git repository.

### What is covered (test/acceptance/)

| Block | Specs |
|-------|-------|
| Operator health | controller-manager deployment is ready |
| CRD validation | invalid model rejected; empty teammates rejected |
| Cowork mode lifecycle | team-state + output PVC; no repo PVC/init Job; pods deployed; Completed phase; completedAt |
| Coding mode lifecycle | team-state + repo PVC + init Job; Initializing; pods deployed; Completed phase |
| Failure handling | Failed on exhausted init Job backoff; Failed on pod failure |
| DependsOn ordering | second teammate blocked until first pod Succeeded |
| Approval gates | teammate blocked; spawned after approval annotation |
| Owner references | PVCs and pods have correct owner references |
| RBAC | operator can manage resources in arbitrary namespaces |

### CI

Acceptance tests run in `.github/workflows/acceptance.yml` on PRs to `main`. The workflow:
1. Builds the operator Docker image
2. Creates a Kind cluster
3. Loads operator + busybox images into Kind
4. Installs CRDs + RBAC + deploys operator in acceptance mode
5. Runs `make test-acceptance`
6. Collects operator logs on failure
7. Deletes the Kind cluster

---

## CI Summary

| Workflow | Trigger | Tests run |
|----------|---------|-----------|
| `validate.yml` | All PRs | Unit + Integration |
| `acceptance.yml` | PRs to `main` | Acceptance (Kind) |
