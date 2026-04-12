# Developer Workflow

Review, checkpoint, track, and ship agent work.

## Output — extract agent results

```bash
safe-ag output api-refactor          # last agent message
safe-ag output --latest              # latest container
safe-ag output --diff api-refactor   # git diff
safe-ag output --files api-refactor  # list changed files
safe-ag output --commits api-refactor # git log
safe-ag output --json api-refactor   # all of the above as JSON
```

Useful for piping into CI, scripts, or `--on-exit` callbacks.

```bash
# Capture result as JSON after a background agent finishes
safe-ag spawn claude --background --ssh --repo git@github.com:org/api.git \
  --prompt "Fix tests" \
  --on-exit "safe-ag output --latest --json > /tmp/fix-result.json"
```

## Summary — one-screen overview

```bash
safe-ag summary api-refactor
safe-ag summary --latest
```

Prints a compact overview: agent type, repo, status, activity, elapsed time, cost estimate, last message, and changed files. Good for a quick status check before reviewing or creating a PR.

## Retry — re-run with the same config

```bash
safe-ag retry api-refactor                       # re-run with same config
safe-ag retry --latest                           # retry latest container
safe-ag retry --latest --feedback "Focus only on src/, skip test files"
```

`safe-ag retry` stops the container, respawns it with the original spawn flags, and optionally appends feedback to the original prompt.

## Diff — review changes

```bash
safe-ag diff api-refactor          # full git diff
safe-ag diff --latest --stat       # summary only
```

Shows what the agent changed in its working tree.

## Checkpoints — snapshot and revert

```bash
# Save current state
safe-ag checkpoint create api-refactor "before refactor"

# List all snapshots
safe-ag checkpoint list api-refactor

# Revert to a previous state
safe-ag checkpoint revert api-refactor checkpoint-1712678400
```

Checkpoints use git stash refs (`refs/safe-agentic/checkpoints/`) — no branch pollution.

## Todos — merge gates

```bash
safe-ag todo add api-refactor "Run integration tests"
safe-ag todo add api-refactor "Update changelog"
safe-ag todo list api-refactor
safe-ag todo check api-refactor 1
safe-ag todo uncheck api-refactor 1
```

`safe-ag pr` blocks if any todos are incomplete.

## Code review

```bash
safe-ag review api-refactor               # codex review (or git diff fallback)
safe-ag review --latest --base main       # compare against base branch
```

Runs `codex review` inside the container if available. Falls back to `git diff`.

## PR creation

```bash
safe-ag pr api-refactor --title "feat: add caching" --base dev
safe-ag pr --latest
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
safe-ag spawn claude --ssh --reuse-auth \
  --repo git@github.com:org/api.git \
  --prompt "Fix the failing CI tests"

# 2. Monitor progress
safe-ag peek --latest

# 3. Quick status check
safe-ag summary --latest

# 4. Review changes
safe-ag diff --latest
safe-ag review --latest

# 5. Extract last message to see what was done
safe-ag output --latest

# 6. Track remaining work
safe-ag todo add --latest "Verify on staging"
safe-ag todo check --latest 1

# 7. Ship it
safe-ag pr --latest --title "fix: resolve CI failures"
```

## Real-world examples

### Example 1: Bug fix workflow

```bash
# 1. Spawn agent with the bug description
safe-ag spawn claude --ssh --reuse-auth \
  --repo git@github.com:myorg/api.git \
  --prompt "Fix issue #42: users get 500 error on /api/profile when name contains unicode"

# 2. Monitor progress
safe-ag peek --latest

# 3. Create checkpoint before agent makes changes
safe-ag checkpoint create --latest "before-fix"

# 4. Review what the agent changed
safe-ag diff --latest
safe-ag diff --latest --stat

# 5. Not happy? Revert and let it try again
safe-ag checkpoint revert --latest before-fix

# 6. Happy with the fix? Track remaining work
safe-ag todo add --latest "Verify fix handles emoji names"
safe-ag todo add --latest "Add regression test"
safe-ag todo add --latest "Update API docs"

# 7. Check off items as the agent completes them
safe-ag todo check --latest 1
safe-ag todo check --latest 2
safe-ag todo check --latest 3

# 8. Run AI code review
safe-ag review --latest

# 9. Create the PR
safe-ag pr --latest --title "fix: handle unicode in user profile names" --base main
```

### Example 2: Refactoring with checkpoints

```bash
safe-ag spawn claude --ssh --reuse-auth \
  --repo git@github.com:myorg/monorepo.git \
  --name refactor-auth \
  --prompt "Refactor the auth middleware to use the new JWT library"

# Checkpoint at key milestones
safe-ag checkpoint create refactor-auth "step-1-extract-interface"
safe-ag checkpoint create refactor-auth "step-2-new-jwt-impl"
safe-ag checkpoint create refactor-auth "step-3-migrate-tests"

# List all checkpoints
safe-ag checkpoint list refactor-auth

# Something went wrong in step 3? Go back to step 2
safe-ag checkpoint revert refactor-auth step-2-new-jwt-impl
```

### Example 3: Code review before PR

```bash
# Review uncommitted changes
safe-ag review my-feature

# Review against a specific branch
safe-ag review my-feature --base develop

# Check the cost so far
safe-ag cost my-feature
```
