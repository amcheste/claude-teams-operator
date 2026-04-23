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
