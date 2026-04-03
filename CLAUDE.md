# CLAUDE.md

This file is read by Claude Code at the start of every session in this repo.

---

## About This Repo

<!-- TODO: describe what this project does -->

---

## Developer Preferences

### Editor
- Primary: Vim
- AI editor: Cursor

### Shell
- zsh, minimal prompt

### Git & GitHub Workflow
- **Branch model:** `main` = latest release. `develop` = integration branch.
- Always branch from `develop`, never commit directly
- PRs always target `develop`
- `main` is only updated via the develop→main release PR opened by `/publish-release`
- Conventional commits: `feat:`, `fix:`, `docs:`, `chore:`

### Scripting Standards
- Shell scripts must pass `shellcheck`
- Use `set -euo pipefail`
- Scripts should be idempotent

---

## Learned Preferences

<!-- Claude Code will suggest additions here as patterns emerge across sessions -->
