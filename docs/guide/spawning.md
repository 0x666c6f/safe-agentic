# Spawning Agents

`berth spawn` is the full-control form: every isolation and auth default is off unless you opt in.

```bash
berth spawn <claude|codex|shell> [flags]
```

Quicker forms when you don't need every knob:

```bash
berth run <repo-url> [repo-url...] [prompt]    # smart defaults, always Claude
berth-claude <repo-url> [repo-url...]          # shortcut binaries;
berth-codex <repo-url> [repo-url...]           # auto --ssh for git@ / ssh:// URLs
berth profile run <name> [prompt]              # saved spawn preset
```

## Most common cases

```bash
# Public repo
berth spawn claude --repo https://github.com/myorg/myrepo.git

# Private repo
berth spawn claude --ssh --repo git@github.com:myorg/myrepo.git

# Immediate task
berth spawn codex --ssh --repo git@github.com:org/repo.git --prompt "Fix the failing tests"

# Detached background run
berth spawn claude --background --ssh --repo git@github.com:org/repo.git \
  --prompt "Review the latest changes"
```

## Choose the agent

| Type | Use it for |
|---|---|
| `claude` | general coding, review, analysis |
| `codex` | coding sessions where you want the Codex CLI |
| `shell` | an interactive shell in the same sandbox model |

## Repos

`--repo` is repeatable — all repos clone into `/workspace`:

```bash
berth spawn claude \
  --repo git@github.com:org/frontend.git \
  --repo git@github.com:org/backend.git
```

Use `https://...` when you don't need SSH, `git@...` with `--ssh` for private repos or pushes. To work on your *current local checkout* instead of cloning, see [Worktrees](worktrees.md).

## Auth flags

| Flag | Effect |
|---|---|
| `--ssh` | forward your SSH agent into the container |
| `--reuse-auth` | share one persistent Claude/Codex auth volume across runs |
| `--reuse-gh-auth` | share GitHub CLI (`gh`) auth across runs |
| `--seed-auth` | one-shot copy of host Claude/Codex auth into this session |
| `--ephemeral-auth` | throwaway auth volume, never touch shared state |
| `--aws <profile>` | inject an AWS profile as env credentials (tmpfs-backed) |
| `--allow-setup-scripts` | let repo-provided `berth.json` setup hooks run |

Each has a `--no-*` twin (`--no-ssh`, `--no-reuse-auth`, `--no-reuse-gh-auth`, `--no-seed-auth`, `--no-docker`, `--no-docker-socket`) to override a `config.toml` default for one session.

By default, Claude/Codex auth starts empty in a per-container mount. What each flag widens is spelled out in [Security](../security.md).

### The risk confirmation

When a spawn would enable something risky (SSH, shared auth, seeded auth, AWS, Docker), berth prints what's being widened and asks for confirmation. Pass `--yes` to skip the prompt in scripts. Hard limits that no flag can cross are set with [policy rules](configuration.md#policy-rules).

## Prompt, instructions, template

```bash
berth spawn claude --repo ... --prompt "Fix the flaky tests"
berth spawn claude --repo ... --instructions "Only touch docs and tests"
berth spawn claude --repo ... --instructions-file ./role.md
berth spawn claude --repo ... --template security-audit --var area=payments
```

- `--prompt` — the task
- `--instructions` / `--instructions-file` — standing role/constraints
- `--template` — a reusable prompt ([built-in or custom](configuration.md#templates))
- `--var key=value` — fills `${key}` placeholders in the template

Templates can reference `${repo}`. If `--repo` is omitted, berth infers it from the current checkout's `origin` remote.

## Runtime and network

```bash
berth spawn claude --docker --repo ...              # Docker-in-Docker sidecar
berth spawn claude --docker-socket --repo ...       # direct VM Docker daemon (broad)
berth spawn claude --aws my-profile --repo ...      # AWS credentials
berth spawn claude --network my-net --repo ...      # custom Docker network
berth spawn claude --memory 12g --cpus 6 --pids-limit 1024 --repo ...
```

Defaults: 8g memory, 4 CPUs, 512 PIDs, dedicated managed bridge network. For an internet-free sandbox, use an internal network — see [Architecture](../architecture.md#networking).

## Naming and behavior

```bash
berth spawn claude --name api-fix --repo ...     # stable container name
berth spawn claude --background --repo ...       # don't attach after launch
berth spawn claude --auto-trust --repo ...       # auto-accept the agent's trust prompt
berth spawn claude --dry-run --repo ...          # print the launch command, don't run
```

## Lifecycle hooks and budgets

```bash
berth spawn claude --background --repo ... \
  --prompt "Fix the flaky tests" \
  --on-complete "cd /workspace/*/ && go test ./... > /workspace/test-results.txt 2>&1" \
  --notify terminal,system \
  --max-cost 5.00
```

- `--on-exit` / `--on-complete` / `--on-fail` — shell commands run **inside the container** when the session ends / exits 0 / exits non-zero
- `--notify` — comma-separated targets delivered **on the host**: `terminal`, `system`, `slack:<webhook>`, `command:<cmd>`
- `--max-cost` — budget in USD, recorded on the container (advisory — surfaced in `summary`/`cost`, not enforced)

More patterns in [Automation](automation.md).

## What happens after spawn

1. berth resolves defaults, validates flags, and checks [policy rules](configuration.md#policy-rules)
2. it creates or joins the Docker network
3. it starts the container with the hardened runtime flags
4. it clones repos into `/workspace`
5. if `--allow-setup-scripts` is set, it runs `berth.json` setup hooks
6. it launches the agent inside tmux
7. it attaches unless you used `--background`

## Related

- [Monitor & Manage](managing.md) — `list`, `status`, `peek`, `attach`
- [Worktrees](worktrees.md) — spawn on a local checkout instead of a clone
- [CLI reference](../reference/cli.md) — every flag, exhaustively
