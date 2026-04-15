# Quickstart

Get from zero to a live agent session with the fewest moving parts.

If you want the shortest working path, this is it:

```bash
brew install orbstack
brew tap 0x666c6f/tap
brew install safe-agentic
safe-ag setup
safe-ag spawn claude --repo https://github.com/myorg/myrepo.git
safe-ag peek --latest
```

Use `--ssh` instead of a public HTTPS repo when the repository is private.

## 1. Install the prerequisites

Homebrew:

```bash
brew install orbstack
brew tap 0x666c6f/tap
brew install safe-agentic
```

From source:

```bash
brew install orbstack
git clone git@github.com:0x666c6f/safe-agentic.git
cd safe-agentic
make build-all
export PATH="$PWD/bin:$PATH"
```

Canonical CLI: `safe-ag`.

Agent-facing shortcuts also ship in `bin/`:
- `safe-ag-claude`
- `safe-ag-codex`

## 2. Build the sandbox once

```bash
safe-ag setup
safe-ag diagnose
```

`safe-ag setup` will:

- create the OrbStack VM if needed
- reapply VM hardening
- build the local Docker image

If setup fails, keep `safe-ag diagnose` output. It is the fastest next debugging step.

## 3. Spawn the first agent

Public repo:

```bash
safe-ag spawn claude --repo https://github.com/myorg/myrepo.git
```

Private repo:

```bash
safe-ag spawn claude --ssh --repo git@github.com:myorg/myrepo.git
```

Immediate task:

```bash
safe-ag spawn claude \
  --ssh \
  --repo git@github.com:myorg/myrepo.git \
  --prompt "Fix the failing CI tests"
```

## 4. Check that the session is alive

```bash
safe-ag list
safe-ag peek --latest
safe-ag attach --latest
```

Expect:

- first auth flow may open a browser or print a device-code login
- agents run inside tmux in the container
- `Ctrl-b d` detaches from the session
- stopped containers can be reattached later

## 5. Review what changed

```bash
# inspect what the current agent changed
safe-ag diff --latest
safe-ag output --latest
safe-ag review --latest
```

Typical loop:

1. `peek` to see current activity
2. `diff` to inspect filesystem changes
3. `review` before you ship anything

If your current checkout is a GitHub PR branch, you can run a one-shot PR review directly in a separate workflow:

```bash
safe-ag pr-review
safe-ag pr-review claude
safe-ag pr-fix
```

These infer:
- `${repo}` from `git remote get-url origin`
- `${pr}` from `gh pr view --json number`

## 6. Clean up when you are done

```bash
safe-ag stop --latest
safe-ag cleanup
safe-ag cleanup --auth
```

Use `cleanup --auth` only when you want to remove shared auth volumes too.

## Next pages

- [Command Map](usage.md)
- [Spawning Agents](guide/spawning.md)
- [Managing Agents](guide/managing.md)
- [Workflow](guide/workflow.md)
- [Security](security.md)
