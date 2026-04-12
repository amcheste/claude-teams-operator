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

## Branch Model

| Branch | Purpose |
|--------|---------|
| `main` | Latest release — always stable, never directly committed to |
| `develop` | Integration branch — all PRs target here |
| `feat/*`, `fix/*`, `docs/*` etc. | Short-lived branches off `develop` |

## Commit Convention

This project uses [Conventional Commits](https://www.conventionalcommits.org/):

| Prefix | Use |
|--------|-----|
| `feat:` | New feature or capability |
| `fix:` | Bug fix |
| `docs:` | Documentation only |
| `test:` | Adding or updating tests |
| `chore:` | Maintenance, dependencies, housekeeping |
| `refactor:` | Code change that neither fixes a bug nor adds a feature |
| `ci:` | CI/CD configuration |

Scopes (optional but encouraged): `feat(controller):`, `fix(crd):`, `docs(readme):`

Breaking changes: append `!` — e.g. `feat(crd)!: rename budgetLimit field`.

Keep commits atomic. One logical change per commit, one logical change per PR.

## Development Workflow

1. Branch from `develop`: `git checkout -b feat/my-feature develop`
2. Make changes
3. Commit with conventional commit messages
4. Run `make manifests generate fmt vet test` — all must pass
5. Open a PR targeting `develop`
6. CI must pass before merging

## Release Process

> Only the repo owner publishes releases.

Releases are handled by the `/publish-release` Claude Code skill:

1. Bumps the version in relevant files on `develop`, commits `chore: release v<version>`
2. Opens a PR: `develop → main`
3. Owner approves and merges
4. Tags `main` with the version — release pipeline fires automatically
