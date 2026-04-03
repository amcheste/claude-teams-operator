# Testing

## Running Tests

```bash
# All tests
make test

# Unit tests only (fast, no cluster required)
go test ./internal/controller/...

# With verbose output
go test ./internal/controller/... -v

# Run a specific test
go test ./internal/controller/... -run TestReconcilePending_CodingMode

# With coverage report
go test ./internal/controller/... -coverprofile=cover.out
go tool cover -html=cover.out
```

## Test Structure

All tests live alongside the code they test:

```
internal/controller/
  agentteam_controller.go       # Reconciler implementation
  agentteam_controller_test.go  # Unit tests (36 tests)
```

Tests use [controller-runtime's fake client](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/client/fake) — a fully in-memory Kubernetes API server with no external dependencies. No Kind cluster, no real API server, no CRD installation required.

## What Is Covered

### Reconciler phases

| Test | What it verifies |
|------|-----------------|
| `TestReconcilePending_CodingMode_CreatesPVCsAndInitJob` | team-state PVC, repo PVC, and init Job are created in coding mode |
| `TestReconcilePending_CoworkMode_CreatesOutputPVC` | output PVC created; no repo PVC or init Job in Cowork mode |
| `TestReconcilePending_Idempotent` | calling reconcilePending twice does not error |
| `TestReconcileInitializing_WaitsForInitJob` | returns requeue when init Job is still running |
| `TestReconcileInitializing_InitJobFailed_SetsFailedPhase` | sets `Failed` phase when init Job exhausts backoff |
| `TestReconcileInitializing_DeploysPods` | deploys lead and teammate pods after init Job succeeds |
| `TestReconcileInitializing_DependsOnBlocks` | does not spawn a teammate whose `dependsOn` dep has not succeeded |
| `TestReconcileInitializing_ApprovalGateBlocksTeammate` | blocks a gated teammate and sets `pendingApproval` |
| `TestReconcileInitializing_ApprovalGrantedViaAnnotation` | spawns a gated teammate when the approval annotation is present |
| `TestReconcileRunning_Timeout_SetsTimedOut` | sets `TimedOut` phase when elapsed time exceeds `lifecycle.timeout` |
| `TestReconcileRunning_BudgetExceeded_SetsBudgetExceeded` | sets `BudgetExceeded` when estimated cost exceeds `lifecycle.budgetLimit` |
| `TestReconcileRunning_AllPodsSucceeded_SetsCompleted` | sets `Completed` when lead and all teammates are `Succeeded` |
| `TestReconcileRunning_PodFailed_SetsFailedPhase` | sets `Failed` when any pod enters `Failed` phase |
| `TestReconcileRunning_SpawnsNewlyUnblockedTeammate` | spawns a teammate mid-run once its `dependsOn` dep completes |

### Pod builder

| Test | What it verifies |
|------|-----------------|
| `TestBuildAgentPod_CodingMode` | correct env vars, `WORKTREE_PATH`, and volume mounts for coding mode |
| `TestBuildAgentPod_LeadHasNoWorktreePath` | lead pod does not receive `WORKTREE_PATH` |
| `TestBuildAgentPod_WithSkills` | skill ConfigMaps are mounted at `/var/claude-skills/{name}` |
| `TestBuildAgentPod_WithMCPServers` | `mcp-config` volume is mounted at `/var/claude-mcp` |
| `TestBuildAgentPod_CoworkMode` | workspace-output and workspace-input volumes present; no repo volume |
| `TestBuildAgentPod_ScopeEnvVars` | `SCOPE_INCLUDE_PATHS` and `SCOPE_EXCLUDE_PATHS` set from `scope` spec |

### Business logic

| Test | What it verifies |
|------|-----------------|
| `TestEstimateCost_ZeroWhenNoStartTime` | returns `"0.00"` when team has not started |
| `TestEstimateCost_OpusMoreExpensiveThanSonnet` | opus cost > sonnet cost for the same elapsed time |
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
| `TestCheckApprovalGate_GatePresentNotApproved` | returns not approved when gate exists but annotation is absent |
| `TestCheckApprovalGate_ApprovedViaAnnotation` | returns approved when the annotation is set to `"true"` |

## Test Helpers

The test file provides a set of composable helpers to reduce boilerplate:

```go
// Build an AgentTeam with just enough fields to be valid.
minimalTeam("my-team")

// Add coding-mode repository config.
withRepo(team)

// Add Cowork-mode workspace config.
withWorkspace(team)

// Set lifecycle timeout and budget.
withLifecycle(team, "1h", "10.00")

// Create pre-built pod objects in various phases.
succeededPod("my-team-lead", "default", "my-team")
failedPod("my-team-lead", "default", "my-team")
runningPod("my-team-lead", "default", "my-team")

// Create pre-built Job objects.
completedJob("my-team-init", "default")
failedJob("my-team-init", "default")
```

Create a reconciler with pre-populated objects:

```go
r := newReconciler(team, leadPod, workerPod)
```

Fetch a fresh copy of an object from the fake client (includes the ResourceVersion set by the fake client, which is required for status update calls):

```go
team = fetch(t, r, "my-team")
```

## Writing New Tests

Follow the existing patterns:

1. **Build the initial state** using the helpers above.
2. **Create the reconciler** with `newReconciler(objs...)`.
3. **Fetch the team** with `fetch(t, r, name)` to get a properly initialised object.
4. **Set any pre-existing status** directly on the fetched object (phase, startedAt, etc.).
5. **Call the method under test** directly (e.g. `r.reconcilePending(ctx, team)`).
6. **Assert both in-memory state** (the team object is mutated in place) and **cluster state** (use `r.Get` to verify created resources).

```go
func TestMyNewScenario(t *testing.T) {
    team := withRepo(minimalTeam("my-team"))
    r := newReconciler(team)
    team = fetch(t, r, "my-team")

    _, err := r.reconcilePending(context.Background(), team)
    require.NoError(t, err)

    // Assert in-memory phase change.
    assert.Equal(t, "Initializing", team.Status.Phase)

    // Assert cluster resource was created.
    var pvc corev1.PersistentVolumeClaim
    require.NoError(t, r.Get(context.Background(),
        types.NamespacedName{Name: "my-team-team-state", Namespace: "default"}, &pvc))
}
```

## Integration Tests (Future)

Unit tests cover the reconciler logic against a fake client. Integration tests using [controller-runtime's envtest](https://book.kubebuilder.io/reference/envtest.html) are planned to cover the full reconcile loop against a real (in-process) API server, including:

- CRD installation and validation
- Controller watch/trigger flow
- Multi-step state transitions over multiple reconcile calls
- Webhook validation

To run envtest tests when they exist:

```bash
# Download envtest binaries (first time only)
make envtest

# Run with envtest
KUBEBUILDER_ASSETS="$(./bin/setup-envtest use --bin-path)" go test ./internal/controller/... -tags=integration
```

## CI

Tests run automatically on every PR via the `Lint` workflow in `.github/workflows/validate.yml`. PRs cannot be merged to `develop` without passing CI.
