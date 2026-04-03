# Contributing

## Branch Model

| Branch | Purpose |
|--------|---------|
| `main` | Latest release — always stable, never directly committed to |
| `develop` | Integration branch — all PRs target here |
| `feature/*`, `fix/*` etc. | Short-lived branches off `develop` |

## Commit Convention

This project uses [Conventional Commits](https://www.conventionalcommits.org/):

| Prefix | Use |
|--------|-----|
| `feat:` | New feature or capability |
| `fix:` | Bug fix |
| `docs:` | Documentation only |
| `chore:` | Maintenance, dependencies, housekeeping |
| `refactor:` | Code change that neither fixes a bug nor adds a feature |

Breaking changes: append `!` after the type — e.g. `feat!: remove legacy API`.

One logical change per PR. Keep commits atomic and the history readable.

## Development Workflow

1. Branch from `develop`
2. Make changes, commit with conventional commit messages
3. Open a PR targeting `develop`
4. CI must pass before merging

## Release Process

> Only the repo owner publishes releases.

Releases are handled by the `/publish-release` Claude Code skill:

1. Bumps version on `develop`, commits `chore: release v<version>`
2. Opens a PR: `develop → main`
3. Owner approves and merges
4. Tags `main` and pushes — release pipeline fires automatically
