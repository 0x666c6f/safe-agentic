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

## Real-world examples

### Example 1: Bug fix workflow

```bash
# 1. Spawn agent with the bug description
agent spawn claude --ssh --reuse-auth \
  --repo git@github.com:myorg/api.git \
  --prompt "Fix issue #42: users get 500 error on /api/profile when name contains unicode"

# 2. Monitor progress
agent peek --latest

# 3. Create checkpoint before agent makes changes
agent checkpoint create --latest "before-fix"

# 4. Review what the agent changed
agent diff --latest
agent diff --latest --stat

# 5. Not happy? Revert and let it try again
agent checkpoint revert --latest before-fix

# 6. Happy with the fix? Track remaining work
agent todo add --latest "Verify fix handles emoji names"
agent todo add --latest "Add regression test"
agent todo add --latest "Update API docs"

# 7. Check off items as the agent completes them
agent todo check --latest 1
agent todo check --latest 2
agent todo check --latest 3

# 8. Run AI code review
agent review --latest

# 9. Create the PR
agent pr --latest --title "fix: handle unicode in user profile names" --base main
```

### Example 2: Refactoring with checkpoints

```bash
agent spawn claude --ssh --reuse-auth \
  --repo git@github.com:myorg/monorepo.git \
  --name refactor-auth \
  --prompt "Refactor the auth middleware to use the new JWT library"

# Checkpoint at key milestones
agent checkpoint create refactor-auth "step-1-extract-interface"
agent checkpoint create refactor-auth "step-2-new-jwt-impl"
agent checkpoint create refactor-auth "step-3-migrate-tests"

# List all checkpoints
agent checkpoint list refactor-auth

# Something went wrong in step 3? Go back to step 2
agent checkpoint revert refactor-auth step-2-new-jwt-impl
```

### Example 3: Code review before PR

```bash
# Review uncommitted changes
agent review my-feature

# Review against a specific branch
agent review my-feature --base develop

# Check the cost so far
agent cost my-feature
```
