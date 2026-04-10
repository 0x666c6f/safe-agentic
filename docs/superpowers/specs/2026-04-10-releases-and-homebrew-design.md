# Releases & Homebrew Distribution

**Date:** 2026-04-10
**Status:** Approved

## Overview

Automated release pipeline for safe-agentic: every push to `main` with user-facing changes creates a semver-tagged GitHub Release with a universal macOS tarball, then updates the Homebrew tap so users can `brew install safe-agentic`.

## Versioning

Semver starting at `v0.1.0`. Version bumps are determined automatically from conventional commit prefixes since the last tag:

| Prefix | Bump | Example |
|--------|------|---------|
| `fix:` | patch | `v0.1.0` → `v0.1.1` |
| `feat:` | minor | `v0.1.0` → `v0.2.0` |
| `feat!:` or `BREAKING CHANGE` in body | major | `v0.1.0` → `v1.0.0` |
| `chore:`, `docs:`, `ci:`, `test:` only | skip | no release |

## Release Workflow

Single workflow `.github/workflows/release.yml`, triggered on push to `main` (after CI passes):

1. **Compute version** — scan commits since last tag, determine bump type, skip if no user-facing commits
2. **Build TUI** — cross-compile `tui/agent-tui` for `darwin-amd64` and `darwin-arm64`, combine with `lipo` into a universal binary
3. **Package tarball** — `safe-agentic-<version>-darwin-universal.tar.gz` containing:
   - `bin/agent`, `bin/agent-lib.sh`, `bin/agent-claude`, `bin/agent-codex`, `bin/agent-alias`, `bin/agent-session.sh`, `bin/docker-runtime.sh`, `bin/repo-url.sh`
   - `config/` (bashrc, seccomp.json, security-preamble.md, tmux.conf)
   - `templates/` (all `.md` templates)
   - `vm/setup.sh`
   - `Dockerfile`, `entrypoint.sh`, `package.json`, `package-lock.json`, `op-env.sh`
4. **Inject version** — `sed` replaces `VERSION="dev"` with `VERSION="vX.Y.Z"` in the tarball's `bin/agent`
5. **Create GitHub Release** — push tag, upload tarball, attach changelog grouped by type
6. **Update Homebrew tap** — push updated formula to `0x666c6f/homebrew-tap` with new URL and SHA256

## Tarball Contents

The tarball preserves the repo directory structure so `REPO_DIR` resolution works unchanged. The TUI binary is placed at `tui/agent-tui` (where `agent tui` expects it).

```
safe-agentic-vX.Y.Z/
  bin/
    agent
    agent-lib.sh
    agent-claude
    agent-codex
    agent-alias
    agent-session.sh
    docker-runtime.sh
    repo-url.sh
  config/
    bashrc
    seccomp.json
    security-preamble.md
    tmux.conf
  templates/
    *.md
  tui/
    agent-tui          # universal binary
  vm/
    setup.sh
  Dockerfile
  entrypoint.sh
  package.json
  package-lock.json
  op-env.sh
```

## Homebrew Distribution

### Tap Repository

`0x666c6f/homebrew-tap` with structure:

```
homebrew-tap/
  Formula/
    safe-agentic.rb
  README.md
```

### Installation

```bash
brew tap 0x666c6f/tap
brew install safe-agentic
```

### Formula Design

- Downloads the universal tarball from the GitHub Release
- Installs everything into `libexec/` preserving directory structure (so `REPO_DIR` resolves to `libexec/`)
- Symlinks `bin/agent`, `bin/agent-claude`, `bin/agent-codex` into Homebrew's `bin/` for PATH availability
- `agent-tui` lives at `libexec/tui/agent-tui` where `agent tui` expects it
- No runtime dependencies declared (bash, git, OrbStack are user-managed)

### Tap Updates

The release workflow updates the tap automatically:

- Computes SHA256 of the tarball
- Checks out `0x666c6f/homebrew-tap`
- Updates `url` and `sha256` in `Formula/safe-agentic.rb`
- Pushes directly to `main`
- Uses a `HOMEBREW_TAP_TOKEN` secret (GitHub PAT with `repo` scope on the tap repo)

## `agent --version`

- `bin/agent` gets a `VERSION="dev"` variable near the top
- The release workflow injects the real version via `sed` in the tarball copy
- `agent --version` prints `safe-agentic vX.Y.Z` (or `safe-agentic dev` for source checkouts)
- Handled in the CLI dispatch section alongside `help`

## Secrets Required

| Secret | Purpose |
|--------|---------|
| `HOMEBREW_TAP_TOKEN` | GitHub PAT with `repo` scope on `0x666c6f/homebrew-tap` for cross-repo push |

## Files Changed

| File | Change |
|------|--------|
| `.github/workflows/release.yml` | New — release workflow |
| `bin/agent` | Add `VERSION="dev"` variable and `--version` flag handling |
| `0x666c6f/homebrew-tap` (new repo) | New — tap repo with `Formula/safe-agentic.rb` and `README.md` |
