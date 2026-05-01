# Contributing

## Prerequisites

- **Go 1.23+** — `brew install go` or [go.dev/dl](https://go.dev/dl)
- **Docker** — for building container images
- **Kind** — `brew install kind` (local cluster)
- **kubectl** — `brew install kubectl`
- **Helm** — `brew install helm`
- **golangci-lint** — `brew install golangci-lint`

Verify your Go installation:

```bash
go version   # go1.23.x or later required
```

## Local Development Setup

```bash
# Clone the repo
git clone git@github.com:amcheste/claude-teams-operator.git
cd claude-teams-operator

# Download dependencies
go mod tidy

# Generate CRD manifests and deepcopy methods
make generate manifests

# Build the operator binary
make build

# Run tests
make test
```

## Make Targets

| Target | Description |
|--------|-------------|
| `make build` | Compile the operator binary to `bin/manager` |
| `make test` | Run all tests with coverage |
| `make lint` | Run `golangci-lint` |
| `make fmt` | Run `go fmt` |
| `make vet` | Run `go vet` |
| `make generate` | Regenerate `zz_generated.deepcopy.go` |
| `make manifests` | Regenerate CRD YAML from Go type markers |
| `make docker-build` | Build the operator Docker image |
| `make docker-build-runner` | Build the Claude Code runner image |
| `make kind-create` | Create a Kind cluster with NFS provisioner |
| `make kind-delete` | Delete the Kind cluster |
| `make kind-load` | Load local images into Kind |
| `make install` | Install CRDs into the current cluster |
| `make deploy` | Deploy the operator to the current cluster |
| `make sample` | Apply sample AgentTeam CRs |

After any code change, run:

```bash
make manifests generate fmt vet test
```

## Running the Operator Locally

You can run the operator against any cluster (including Kind) without building a Docker image:

```bash
# Point kubectl at your cluster, then:
make install   # install CRDs
go run cmd/manager/main.go
```

The operator uses your current kubeconfig context.

## Modifying CRD Types

The CRD types live in `api/v1alpha1/`. After modifying them:

1. Run `make generate` to regenerate `zz_generated.deepcopy.go`
2. Run `make manifests` to regenerate the CRD YAML in `config/crd/bases/`
3. Run `make install` to apply the updated CRDs to your cluster
4. Commit both the Go source changes **and** the generated files

Do not edit `zz_generated.deepcopy.go` or `config/crd/bases/*.yaml` by hand — they are always regenerated.

## Testing

See [TESTING.md](TESTING.md) for a full guide. Quick reference:

```bash
make test                                              # all tests
go test ./internal/controller/... -v                  # verbose
go test ./internal/controller/... -run TestMyTest      # specific test
```

## Branching, Commits, and Releases

The branching strategy, commit convention, and release process follow the canonical rules documented in my engineering handbook:

- **Why:** [Branching Strategy philosophy](https://github.com/amcheste/engineering-handbook/blob/main/docs/philosophies/branching-strategy.md)
- **How:** [Branching & Releases workflow](https://github.com/amcheste/engineering-handbook/blob/main/docs/workflows/branching-and-releases.md)

In short: branch from `develop`, one logical change per PR, [Conventional Commits](https://www.conventionalcommits.org/) (`feat:` / `fix:` / `docs:` / `chore:` / `refactor:`, `!` for breaking), and releases are cut by `/publish-release` with a CLI merge from `develop` to `main` (never GitHub's merge button).

### Repo-local conventions

This repo extends the canonical commit types with:

- `test:` — adding or updating tests
- `ci:` — CI/CD configuration changes

Scopes are encouraged (optional but helpful): `feat(controller):`, `fix(crd):`, `docs(readme):`, `feat(crd)!: rename budgetLimit field`.

### Pre-push checklist

Before pushing a PR, run:

```bash
make manifests generate fmt vet test
```

All must pass. CI will re-run them.

---

## How to add a new reconciler feature

The most common contribution path is "add a new field to an `AgentTeam` and have the operator do something with it." Use this worked example as a template — it's the path #13–#16 followed for crash respawn, RBAC, create-pr, and push-branch.

### 1. Decide where the field belongs

Most lifecycle-related fields live on `LifecycleSpec`; pod-level configuration lives on `LeadSpec`/`TeammateSpec`; cluster-wide defaults live on the Helm chart's `values.yaml`. When in doubt, look at how `MaxRestarts` or `GitCredentialsSecret` are wired — they're representative.

### 2. Extend the CRD type

Edit `api/v1alpha1/agentteam_types.go` (or `template_types.go`). Add the field with full kubebuilder markers:

```go
// MaxRestarts bounds how many times each teammate pod may be re-spawned
// after a Failed phase before the team itself is marked Failed. The lead
// pod is not subject to this limit — a lead crash always fails the team.
// +kubebuilder:default=3
// +kubebuilder:validation:Minimum=0
// +optional
MaxRestarts *int32 `json:"maxRestarts,omitempty"`
```

The doc comment becomes the CRD's OpenAPI description — write it for someone reading `kubectl explain agentteam.spec.lifecycle.maxRestarts`.

### 3. Regenerate manifests + deepcopy

```bash
make manifests generate
```

This rewrites `config/crd/bases/*.yaml`, `charts/claude-teams-operator/crds/*.yaml`, and `api/v1alpha1/zz_generated.deepcopy.go`. Commit them with the source change in the same PR.

### 4. Implement the reconciler change

Find the right phase function — `reconcilePending`, `reconcileInitializing`, `reconcileRunning`, or `reconcileTerminal` — in `internal/controller/agentteam_controller.go`. The phases are documented in [ARCHITECTURE.md § State Machine](ARCHITECTURE.md).

Add a small helper rather than inlining new logic. The convention is `func (r *AgentTeamReconciler) handleX(ctx, team) (...)` for stateful behavior, and free functions for pure logic. See `handleTeammateFailures` and `newTeamTracker` for examples.

If the feature needs Kubernetes API permissions the operator doesn't already have, add a `+kubebuilder:rbac:` marker on the `Reconcile` function and re-run `make manifests`.

### 5. Wire metrics + webhook + events

Three observability surfaces, in order of importance:

| Surface | When to use | API |
|---|---|---|
| Kubernetes Event | Operator did something a human should know about | `r.recordEvent(team, EventTypeNormal, "Reason", "fmt", args...)` |
| Webhook | Subscribers care about this lifecycle moment | `teamNotifier(team).SendEvent(ctx, "team.foo", payload)` |
| Prometheus metric | Time-series matters | Add to `internal/metrics`, call `metrics.RecordX(...)` |

If the existing webhook event types don't fit, add a new one to `internal/webhook/doc.go` and gate any new label cardinality on the metric carefully.

### 6. Tests, in three layers

Each PR should add tests at the layers it changes:

- **Unit tests** — fast, fake-client based. Cover validation, branch coverage in your helper, error paths. Add to `internal/controller/agentteam_<feature>_test.go`. See [TESTING.md](TESTING.md) for the suite breakdown.
- **Integration tests** — envtest-backed Ginkgo specs in `internal/controller/agentteam_integration_test.go` (or a new `agentteam_<feature>_integration_test.go`). Use these when the behavior depends on the real API server's optimistic concurrency, status subresource handling, or owner references.
- **Acceptance tests** — Kind-cluster Ginkgo specs under `test/acceptance/`. Use when the behavior involves pod lifecycle, PVC mounting, or anything that fake-client can't simulate. Real-API E2E (`test/e2e/`) is reserved for end-to-end verification against Anthropic's API.

A good rule: if your feature has a state machine, your test count should be ≥ the number of branches in the state machine.

### 7. Update the chart if there's a new default

Cluster-wide defaults belong on the operator's CLI flags (read from a ConfigMap mounted via `envFrom`). Add a default to `charts/claude-teams-operator/values.yaml`, surface it in `templates/configmap.yaml`, and document it in [`docs/helm-values.md`](docs/helm-values.md).

### 8. Document the field

- Update CRD docstrings (auto-render into `kubectl explain`)
- If it's a Helm-tunable, update `docs/helm-values.md`
- If the user-facing semantics are non-obvious, add a paragraph to `ARCHITECTURE.md`
- Open the PR with a "Testing" section showing how a maintainer can reproduce the change end-to-end

### Reference PRs

These are good examples to skim before opening your first reconciler PR — each one followed this exact recipe:

- [#13 Crash respawn](https://github.com/amcheste/claude-teams-operator/pull/133) — controller state machine + metrics + webhook + tests across all three layers
- [#14 Per-agent RBAC](https://github.com/amcheste/claude-teams-operator/pull/134) — CRD-less feature: just controller logic + scoped Roles + RBAC markers
- [#15 create-pr](https://github.com/amcheste/claude-teams-operator/pull/135) — new internal package (`internal/github`) + controller wiring + httptest-backed tests
- [#16 push-branch](https://github.com/amcheste/claude-teams-operator/pull/148) — async terminal Job + status mirror + envtest integration spec
