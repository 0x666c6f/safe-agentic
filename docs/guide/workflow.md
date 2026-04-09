# Developer Workflow

Review, checkpoint, track, and ship agent work.

## Diff — review changes

```bash
agent diff api-refactor          # full git diff
agent diff --latest --stat       # summary only
```

Shows what the agent changed in its working tree.

## Checkpoints — snapshot and revert

```bash
# Save current state
agent checkpoint create api-refactor "before refactor"

# List all snapshots
agent checkpoint list api-refactor

# Revert to a previous state
agent checkpoint revert api-refactor checkpoint-1712678400
```

Checkpoints use git stash refs (`refs/safe-agentic/checkpoints/`) — no branch pollution.

## Todos — merge gates

```bash
agent todo add api-refactor "Run integration tests"
agent todo add api-refactor "Update changelog"
agent todo list api-refactor
agent todo check api-refactor 1
agent todo uncheck api-refactor 1
```

`agent pr` blocks if any todos are incomplete.

## Code review

```bash
agent review api-refactor               # codex review (or git diff fallback)
agent review --latest --base main       # compare against base branch
```

Runs `codex review` inside the container if available. Falls back to `git diff`.

## PR creation

```bash
agent pr api-refactor --title "feat: add caching" --base dev
agent pr --latest
```

Stages, commits, pushes, and creates a GitHub PR via `gh pr create`.

**Requirements:**
- Container must have been spawned with `--ssh` (for push)
- `gh` auth must be set up (via `--reuse-gh-auth` or `gh auth login` inside)
- All todos must be checked off

## Lifecycle scripts

Add a `safe-agentic.json` to your repo root:

```json
{
  "scripts": {
    "setup": "npm install && cp .env.example .env"
  }
}
```

The `setup` script runs automatically after the repo is cloned. Use it for dependency installation, environment setup, or any initialization.

## Typical workflow

```bash
# 1. Spawn with a task
agent spawn claude --ssh --reuse-auth \
  --repo git@github.com:org/api.git \
  --prompt "Fix the failing CI tests"

# 2. Monitor progress
agent peek --latest

# 3. Review changes
agent diff --latest
agent review --latest

# 4. Track remaining work
agent todo add --latest "Verify on staging"
agent todo check --latest 1

# 5. Ship it
agent pr --latest --title "fix: resolve CI failures"
```
