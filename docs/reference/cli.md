# CLI Reference

This page is the exhaustive command reference for `berth` as it exists today.

Conventions used below:
- `<required>` means a required positional argument
- `[optional]` means an optional positional argument
- `--latest` means "target the most recently started container"

## Top-level

```bash
berth [command]
berth [command] --help
```

Global flags:

| Flag | Meaning |
|---|---|
| `-h`, `--help` | show help |
| `-v`, `--version` | print version |

Top-level commands:

| Command | Purpose |
|---|---|
| `action` | run configured project or user actions inside agents |
| `attach` | attach to an agent tmux session |
| `audit` | show audit log entries |
| `aws-refresh` | refresh AWS credentials in a running container |
| `browser` | capture browser verification artifacts |
| `checkpoint` | manage workspace snapshots |
| `cleanup` | remove containers, networks, and optional auth volumes |
| `config` | manage persistent defaults |
| `config-sync` | push host Claude settings into an agent |
| `cost` | estimate API cost from session data |
| `cron` | manage scheduled jobs |
| `diagnose` | run environment health checks |
| `diff` | show git diff from an agent workspace |
| `fleet` | spawn agents from a fleet manifest |
| `handoff` | copy or locate an agent workspace for handoff |
| `inbox` | show events that may need attention |
| `list` | list agent containers |
| `logs` | show session conversation logs |
| `mcp-login` | authenticate an MCP service |
| `output` | show agent output or derived views |
| `peek` | show the latest visible output |
| `pipeline` | run staged pipelines |
| `pr` | create a GitHub PR from agent work |
| `pr-fix` | fix review feedback on the current or given PR |
| `pr-review` | run a one-shot PR review workflow |
| `profile` | run reusable agent profiles |
| `replay` | replay a session event log |
| `retry` | retry a failed agent with the same config |
| `review` | run an AI review over the diff |
| `review-comments` | store local file/line review comments |
| `run` | quick-start wrapper around `spawn` |
| `search` | search agent session logs |
| `server` | serve berth state over JSON protocol |
| `sessions` | export session data |
| `setup` | initialize VM and build the image |
| `spawn` | start a new agent container |
| `status` | show live agent state (blocked/working/done/idle/exited) |
| `steer` | send a follow-up message into an agent tmux session |
| `stop` | stop agent containers |
| `summary` | show a compact agent summary |
| `template` | manage prompt templates |
| `timeline` | show recent events and audit entries |
| `todo` | manage merge-gate todos |
| `tui` | launch the terminal UI |
| `update` | rebuild the image |
| `vm` | manage the Apple container machine |
| `workspace` | stage, unstage, or revert files in an agent workspace |
| `worktree` | manage host worktrees created by `--worktree` |

## Container-targeting convention

Many commands take one of these forms:

```bash
berth <command> <name>
berth <command> --latest
```

`<name>` may be a full container name or a unique substring. Ambiguous substrings fail and print the matching container names.

Commands in this family:
- `attach`
- `aws-refresh`
- `cost`
- `diff`
- `logs`
- `output`
- `peek`
- `pr`
- `replay`
- `retry`
- `review`
- `sessions`
- `status`
- `stop`
- `summary`
- most `checkpoint` and `todo` subcommands

## `spawn`

Usage:

```bash
berth spawn <claude|codex|shell> [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--auto-trust` | bool | skip the trust prompt |
| `--aws` | string | AWS profile for credential injection |
| `--background` | bool | run detached instead of attaching |
| `--allow-setup-scripts` | bool | allow repo-provided `berth.json` setup hooks |
| `--cpus` | string | CPU limit |
| `--docker` | bool | enable Docker-in-Docker |
| `--docker-socket` | bool | mount the VM Docker socket directly |
| `--dry-run` | bool | print the resolved launch command only; sensitive env and labels are redacted |
| `--ephemeral-auth` | bool | use a per-container auth volume |
| `--fleet-volume` | string | shared fleet volume name |
| `--identity` | string | git identity in `Name <email>` form |
| `--instructions` | string | task instructions |
| `--instructions-file` | string | read instructions from a file |
| `--max-cost` | string | USD budget recorded on the container (advisory; surfaced by `summary`/`cost`, not enforced) |
| `--memory` | string | memory limit, e.g. `8g` |
| `--name` | string | explicit container name suffix |
| `--no-docker` | bool | disable default Docker-in-Docker |
| `--no-docker-socket` | bool | disable default host Docker socket |
| `--no-reuse-auth` | bool | disable default shared auth volume |
| `--no-reuse-gh-auth` | bool | disable default GitHub CLI auth reuse |
| `--no-seed-auth` | bool | disable default host auth seeding |
| `--no-ssh` | bool | disable default SSH agent forwarding |
| `--network` | string | custom Docker network |
| `--notify` | string | notification targets, comma-separated, delivered host-side (see [Notify targets](#notify-targets)) |
| `--on-complete` | string | command run inside the container on success |
| `--on-exit` | string | command run inside the container on exit |
| `--on-fail` | string | command run inside the container on failure |
| `--pids-limit` | int | PIDs limit, minimum 64 |
| `--prompt` | string | initial prompt |
| `--repo` | strings | repository URL to clone; repeatable |
| `--reuse-auth` | bool | reuse shared auth volume |
| `--reuse-gh-auth` | bool | reuse GitHub CLI auth |
| `--seed-auth` | bool | copy host Claude/Codex auth into this session |
| `--ssh` | bool | enable SSH agent forwarding |
| `--template` | string | prompt template name |
| `--var` | strings | template variable assignment `key=value`; repeatable |
| `--worktree` | bool | create and mount a managed git worktree from the current checkout |
| `--worktree-branch` | string | branch name for `--worktree` |
| `--worktree-include` | string | include file for ignored local files; default `.berthinclude` |
| `--worktree-path` | string | destination path for `--worktree` |
| `--yes` | bool | skip the host-side risk confirmation prompt |

Worktree mode:

```bash
berth spawn claude --worktree --name auth-fix --prompt "Fix auth tests"
```

`--worktree` must run from inside a git checkout and cannot be combined with `--repo`. It creates a branch under `berth/<container>` by default, bind-mounts that checkout at `/workspace`, and copies ignored local files listed in `.berthinclude`. A `--worktree-path` outside the worktrees root is rejected before launch.

**`--worktree` is opt-in and off by default** — it requires `berth setup --enable-worktrees`, which deliberately weakens the VM boundary. How the mount works: [Worktrees guide](../guide/worktrees.md). Why it's a trade-off: [Security — threat model](../security.md#the-worktree-mount-trade-off).

Spawn policy:

```toml
# ~/.berth/rules.toml or .berth/rules.toml
[allow]
docker_modes = ["off", "dind"]
networks = ["managed"]
aws_profiles = ["dev"]
ssh = false
reuse_auth = false
reuse_gh_auth = false
seed_auth = false
setup_scripts = false
```

Policy is enforced after config defaults are applied and before network/container creation. User and nearest project rules both apply; any deny blocks the spawn. Omitted keys are unrestricted.

### Notify targets

`--notify` takes a comma-separated list of targets. The whole string is
persisted (base64) on the container and reconstructed for later delivery.

| Target | Form | Delivery |
|---|---|---|
| `terminal` | `terminal` | print to the terminal |
| `slack` | `slack:<webhook-url>` | POST to a Slack incoming webhook |
| `command` | `command:<path>` | run an executable with the event |
| `system` | `system` | native macOS notification |

The `system` target posts a macOS notification titled `berth: <container>`
via `terminal-notifier` when it is on `PATH`, otherwise via
`osascript -e 'display notification …'`. The sound conveys severity:
attention-worthy events (`blocked`, `failed`, `needs-auth`, `stuck`) play a
harsh sound (`Basso`); successful ones (`ready-for-review`, `ready-for-pr`,
`done`) play a soft sound (`Glass`); everything else is silent. On non-macOS
hosts the `system` target is a no-op.

Example:

```bash
berth spawn claude --notify terminal,system --repo git@github.com:org/repo.git
```

## `run`

Usage:

```bash
berth run <repo-url> [repo-url...] [prompt] [flags]
```

`run` is a convenience wrapper around `spawn`.

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--allow-setup-scripts` | bool | allow repo-provided `berth.json` setup hooks |
| `--background` | bool | run detached |
| `--cpus` | string | CPU limit |
| `--dry-run` | bool | print the resolved launch command only; sensitive env and labels are redacted |
| `--ephemeral-auth` | bool | use a per-container throwaway auth volume |
| `--instructions` | string | task instructions |
| `--max-cost` | string | cost budget |
| `--no-docker` | bool | disable default Docker-in-Docker |
| `--no-docker-socket` | bool | disable default host Docker socket |
| `--no-reuse-auth` | bool | disable default shared auth volume |
| `--no-reuse-gh-auth` | bool | disable default GitHub CLI auth reuse |
| `--no-seed-auth` | bool | disable default host auth seeding |
| `--no-ssh` | bool | disable default SSH agent forwarding |
| `--memory` | string | memory limit |
| `--name` | string | container name |
| `--network` | string | custom Docker network |
| `--seed-auth` | bool | copy host Claude/Codex auth into this session |
| `--template` | string | prompt template |
| `--var` | strings | template variable assignment `key=value`; repeatable |
| `--worktree` | bool | create and mount a managed git worktree from the current checkout |
| `--worktree-branch` | string | branch name for `--worktree` |
| `--worktree-path` | string | destination path for `--worktree` |
| `--yes` | bool | skip the host-side risk confirmation prompt |

## `list`

Usage:

```bash
berth list [flags]
```

The human-readable output includes a **STATE** column next to each agent —
the same `agentstate` classification used by `berth status` and the TUI
(`blocked` / `working` / `done` / `idle` / `exited`). Running tmux agents are
detected from their live pane; stopped containers map to `done` (clean exit) or
`exited` (non-zero) by exit code.

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--json` | bool | output raw JSON-like line format from Docker listing |

With `--json`, each Docker line gains an added `"state"` field. All existing
Docker fields are preserved unchanged, so the output stays backward compatible.

## `action`

Usage:

```bash
berth action list
berth action show <name>
berth action run <name> [agent|--latest]
```

Actions are loaded from `~/.berth/actions.toml`, then `.berth/actions.toml` in the current directory. Project actions override user actions with the same name.

Schema:

```toml
[actions.test]
description = "Run unit tests"
command = "go test ./..."

[actions.lint]
command = "npm run lint"
cwd = "frontend"
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--file` | strings | additional actions.toml file; repeatable |
| `--latest` | bool | for `action run`, target the latest container |

## `profile`

Usage:

```bash
berth profile list
berth profile show <name>
berth profile run <name> [prompt]
```

Profiles are loaded from `~/.berth/agents/*.toml`, then `.berth/agents/*.toml` in the current directory. Project profiles override user profiles with the same name.

Schema:

```toml
agent_type = "codex"
repo = ["git@github.com:org/repo.git"]
container_name = "reviewer"
prompt = "Review this repo and report actionable issues"
ssh = true
reuse_auth = true
reuse_gh_auth = true
background = true
```

Useful fields mirror spawn flags: `template`, `template_vars`, `instructions`, `network`, `memory`, `cpus`, `pids_limit`, `aws`, `max_cost`, `docker`, `docker_socket`, `seed_auth`, `auto_trust`, and lifecycle callbacks.

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--dir` | strings | additional profile directory; repeatable |
| `--dry-run` | bool | for `profile run`, show the resolved spawn command |

## `search`

Usage:

```bash
berth search <query> [agent|--latest]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--case-sensitive` | bool | use case-sensitive matching |
| `--latest` | bool | target the latest container |
| `--lines` | int | session lines to scan per agent; default `500` |

## `server`

Usage:

```bash
berth server --stdio
BERTH_SERVER_TOKEN=secret berth server --listen 127.0.0.1:8765
```

Reads newline-delimited JSON requests from stdin and writes JSON responses to stdout. With `--listen`, accepts authenticated `POST /rpc` requests with `Authorization: Bearer <token>`. HTTP listen addresses must be loopback-only (`localhost`, `127.0.0.1`, or `::1`).

Methods: `schema`, `ping`, `timeline`, `inbox`, `agents.list`, `agent.logs`, `agent.diff`, `actions.list`, and `actions.run`.

Example:

```json
{"jsonrpc":"2.0","id":1,"method":"timeline","params":{"lines":20}}
```

## `browser`

Usage:

```bash
berth browser capture <url> [--mode auto|http|chrome] [--annotation NOTE] [--out DIR] [--timeout 30s]
```

Captures browser artifacts under `~/.berth/state/browser/<timestamp>` by default. `http` mode captures DOM and headers. `chrome` mode uses headless Chrome/CDP to capture DOM, screenshot, console, and network artifacts. `--annotation` writes notes into `annotations.json` for handoff to agents. `auto` tries Chrome when available, then falls back to HTTP. It does not mount or reuse host browser profiles or cookies.

## `attach`

Usage:

```bash
berth attach <name|--latest> [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--latest` | bool | target the latest container |
| `--resume` | bool | continue the agent's previous conversation instead of starting a fresh session |

With `--resume`, the agent continues in continue mode (`claude --continue` /
`codex resume --last`) rather than a fresh prompt. If the container is still
running with a live session, `attach --resume` simply reconnects to the ongoing
conversation. If it exited, the container is restarted and the entrypoint
resumes automatically. If it is running but has no attachable tmux session,
`attach --resume` **refuses** rather than relaunch — a headless agent (e.g.
`--background`) may still be alive, and starting a second agent against the same
workspace and auth volume would be unsafe; use `berth steer` to send input, or
`berth stop` then `attach --resume` to restart. Resume works only when the
conversation transcript survived: it lives under `~/.claude` / `~/.codex`, so a
session that used `--ephemeral-auth` (tmpfs) loses its transcript once the
container stops — on a stopped ephemeral container `attach --resume` **refuses**
(restarting would auto-continue against an empty auth dir and error), so use
plain `berth attach <name>` or `berth retry` for a fresh run. Use
`--reuse-auth` (a persistent named volume) if you want conversations to survive
stops. `--resume` supports claude and codex agents only.

## `config-sync`

Usage:

```bash
berth config-sync <name|--latest> [--restart]
```

Pushes your current host Claude settings into an agent container. With `--restart`, the container restarts so the agent relaunches with the synced settings.

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--latest` | bool | target the latest container |
| `--restart` | bool | restart the container to apply the synced settings now |

## `steer`

Usage:

```bash
berth steer <name|--latest> "follow-up message"
```

If the target container is stopped, `steer` starts it first and waits for the tmux session.

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--latest` | bool | target the latest container |

## `peek`

Usage:

```bash
berth peek [name|--latest] [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--latest` | bool | target the latest container |
| `--lines` | int | number of lines to show; default `30` |

## `logs`

Usage:

```bash
berth logs [name|--latest] [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--follow`, `-f` | bool | follow log output |
| `--latest` | bool | target the latest container |
| `--lines` | int | number of log entries; default `50` |

## `status`

Reports the live state of an agent inferred from its tmux pane: `blocked`
(waiting on a permission / trust / approval prompt or an interactive login),
`working` (actively streaming), `idle` (sitting at an empty prompt), `done`
(stopped, exit 0), `exited` (stopped, non-zero), or `unknown`. Detection is
deliberately conservative about `blocked` — a false positive is worse than a
miss — so ambiguous panes resolve to `working` or `unknown` rather than
`blocked`.

Usage:

```bash
berth status [name|--latest] [flags]
berth status --all
berth status agent-foo --json
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--all` | bool | show every berth container |
| `--json` | bool | output as JSON |
| `--latest` | bool | target the latest container |

A blocked agent also surfaces in [`inbox`](#inbox) as a needs-attention item,
and can drive the [`system` notify target](#notify-targets).

## `summary`

Usage:

```bash
berth summary [name|--latest] [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--latest` | bool | target the latest container |

The summary includes a `State:` line with the same detection used by
[`status`](#status).

## `output`

Usage:

```bash
berth output [name|--latest] [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--commits` | bool | show git commit log |
| `--diff` | bool | show git diff |
| `--files` | bool | show changed files |
| `--json` | bool | emit JSON |
| `--latest` | bool | target the latest container |

## `diff`

Usage:

```bash
berth diff [name|--latest] [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--stat` | bool | show diffstat only |
| `--side-by-side`, `-s` | bool | render the diff side-by-side with `delta` (baked into the agent image) |

`--stat` and `--side-by-side` are mutually exclusive. `--side-by-side` sizes delta's columns to the host terminal width (falls back to `$COLUMNS`, then 160). If `delta` is missing from an older agent image, it prints a one-line warning to stderr and falls back to a plain diff.

## `review`

Usage:

```bash
berth review [name|--latest] [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--base` | string | base branch for diff; default `main` |

The review prompt requires every finding to carry a risk tag (`[HIGH]`, `[MEDIUM]`, or `[LOW]`) with a `file:line` location, and a closing `VERDICT:` line. After the raw review text, `berth review` prints a grouped HIGH → LOW summary (untagged findings are kept under `UNTAGGED`, never dropped) followed by the verdict.

## `review-comments`

Usage:

```bash
berth review-comments list [agent|--latest]
berth review-comments add [agent|--latest] <file> <line> <body>
berth review-comments resolve <id>
berth review-comments clear <agent|--latest>
```

Comments are stored locally in `~/.berth/state/review-comments.jsonl`. Use them to keep file/line review notes attached to an agent while you steer fixes or prepare handoff.

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--all` | bool | for `list`, include resolved comments |
| `--file` | string | override the review-comments storage file |
| `--latest` | bool | target the latest container |

## `handoff`

Usage:

```bash
berth handoff <agent|--latest> --to-local ./workspace-copy
berth handoff <agent|--latest> --to-worktree
```

`--to-local` copies `/workspace` out of the container. `--to-worktree` prints the managed host worktree path for agents spawned with `--worktree`.

## `workspace`

Usage:

```bash
berth workspace stage <agent> <path...>
berth workspace unstage <agent> <path...>
berth workspace revert <agent> <path...> --yes
berth workspace stage-patch <agent> selected.patch
berth workspace revert-patch <agent> selected.patch --yes
```

Unlike most commands, `workspace` subcommands take an explicit agent name — the `--latest` flag is not supported here. Paths must stay relative to the workspace. `revert` and `revert-patch` discard changes and require `--yes` when stdin is not interactive. Patch commands accept selected hunks from a normal unified diff and reject workspace-escaping paths.

## `worktree`

Usage:

```bash
berth worktree list
berth worktree snapshot <agent|--latest> [label]
berth worktree restore <agent|--latest> <stash-ref>
berth worktree cleanup [--dry-run] [--all]
```

`list` reads `~/.berth/state/worktrees.jsonl`. `snapshot` and `restore` operate on the git worktree attached to an agent. `cleanup` drops missing registry entries by default; `--all` also removes registered worktrees with `git worktree remove --force`.

## `timeline`

Usage:

```bash
berth timeline
```

Shows recent events from `~/.berth/state/events.jsonl` and audit entries from `~/.berth/state/audit.jsonl`.

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--lines` | int | number of recent entries; default `50` |

## `inbox`

Usage:

```bash
berth inbox
berth inbox --all
```

Shows events likely to need attention, such as failed cron jobs or entries marked `needs-auth`, `blocked`, `stuck`, `failed-tests`, `ready-for-review`, or `ready-for-pr`. In addition to logged events, `inbox` sweeps running agents live and adds a `blocked` item for any agent currently waiting on a prompt.

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--all` | bool | include informational entries |

## `pr`

Usage:

```bash
berth pr [name|--latest] [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--base` | string | base branch; default `main` |
| `--title` | string | PR title |

## `retry`

Usage:

```bash
berth retry <name|--latest> [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--feedback` | string | additional guidance appended to the retry prompt (or, with `--resume`, sent as a follow-up message) |
| `--resume` | bool | continue the source conversation instead of re-running the original prompt |

By default `retry` reconstructs the original spawn options into a fresh
container and re-injects the prompt. With `--resume`, it instead reuses the
source container's exact session/auth volume by restarting that container in
continue mode, so the prior conversation is preserved. When combined with
`--feedback`, the feedback text is delivered as a follow-up message through the
same input path as `steer` (rather than appended to the prompt). If the source
session used `--ephemeral-auth`, its transcript did not survive the stop and
`retry --resume` fails with an actionable error — retry without `--resume` for a
fresh attempt, or re-run the task with `--reuse-auth` so future sessions
persist. If the source container is still running but has no live tmux session
(a headless agent may still be active), `retry --resume` refuses rather than
risk a second agent — use `berth steer`, or `berth stop` it first.
`--resume` supports claude and codex agents only.

## `replay`

Usage:

```bash
berth replay [name|--latest] [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--latest` | bool | target the latest container |
| `--tools-only` | bool | show only tool calls |

## `sessions`

Usage:

```bash
berth sessions [name|--latest] [dest] [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--latest` | bool | target the latest container |

## `aws-refresh`

Usage:

```bash
berth aws-refresh [name|--latest] [profile] [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--latest` | bool | target the latest container |

## `cost`

Usage:

```bash
berth cost [name|--latest] [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--history` | string | show historical costs, e.g. `7d`, `30d` |
| `--latest` | bool | target the latest container |

## `audit`

Usage:

```bash
berth audit [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--lines` | int | number of entries to show; default `50` |

## `checkpoint`

Subcommands:

### `checkpoint create`

```bash
berth checkpoint create <name|--latest> [label]
```

### `checkpoint list`

```bash
berth checkpoint list <name|--latest>
```

### `checkpoint restore`

```bash
berth checkpoint restore <name|--latest> <ref>
```

No additional flags beyond `--help`.

## `todo`

Subcommands:

### `todo add`

```bash
berth todo add <name|--latest> <text>
```

### `todo list`

```bash
berth todo list <name|--latest>
```

### `todo check`

```bash
berth todo check <name|--latest> <index>
```

### `todo uncheck`

```bash
berth todo uncheck <name|--latest> <index>
```

No additional flags beyond `--help`.

## `fleet`

Usage:

```bash
berth fleet <manifest.yaml> [flags]
berth fleet status
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--dry-run` | bool | print what would run without executing |
| `--repo` | strings | default repo URL for agents missing `repo` or `repos` |
| `--var` | strings | manifest variable assignment `key=value`; repeatable |

Subcommands:

### `fleet status`

```bash
berth fleet status
```

## `pipeline`

Usage:

```bash
berth pipeline <pipeline.yaml|name> [flags]
berth pipeline list
berth pipeline show <name>
berth pipeline inspect <name>
berth pipeline render <name>
berth pipeline validate <name>
berth pipeline create <name>
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--background` | bool | run the pipeline in the background and return immediately |
| `--dry-run` | bool | print the execution plan without running |
| `--repo` | strings | default repo URL for agents missing `repo` or `repos` |
| `--var` | strings | manifest variable assignment `key=value`; repeatable |

Saved user pipelines live in `~/.berth/pipelines/`. Built-in review presets ship under the same catalog surface.

### Judge stages (best-of-N "crown")

A pipeline stage can select the single best result among two or more candidate
runs instead of spawning an agent of its own. Add a `judge` block to a step
(flat form) or stage (stages form). The judge depends on the candidate stages,
collects each candidate container's working-tree diff and final message, runs a
one-shot Claude judge to pick a winner, and records a strict-JSON verdict.

```yaml
name: judge-fanout
defaults:
  repo: ${repo}
  ssh: true
  reuse_auth: true
  auto_trust: true
steps:
  - name: implement            # fan out across engines → 2 candidates
    models: [claude, codex]
    prompt: "${task}\n\nYou are candidate ${model}. Commit focused, tested changes."
  - name: pick-winner
    judge:
      criteria: "correctness and tests first, then minimal diff"  # optional
      auto_pr: true                                               # optional (default false)
      base: main                                                  # optional PR base (default main)
    depends_on: implement
```

Judge block fields:

| Field | Type | Meaning |
|---|---|---|
| `criteria` | string | optional free-text ranking guidance; a quality-first default is used when empty |
| `auto_pr` | bool | when true, open a GitHub PR from the winning container (default false) |
| `base` | string | PR base branch used with `auto_pr` (default `main`) |
| `max_diff` | int | per-candidate diff byte cap embedded in the judge prompt (default 12000; truncation is noted inline) |

Rules (validated at parse time):

- A judge stage must `depends_on` stages that collectively produce **at least
  two** candidate runs. Fan out either with `models: [...]` on the parent step/
  stage, or with two or more `agents:` in a single candidate stage.
- A judge stage/step must **not** carry `prompt`, `template`, `repo`, `type`,
  `instructions`, `profile`, `models`, or `agents` of its own.
- A judge cannot depend on a sub-pipeline stage or on another judge stage.

Notes on candidate fan-out: `models: [...]` expands one step/stage into one
candidate container **per entry**, and each entry becomes that candidate's agent
`type` — so today the practical values are the agent engines `claude` and
`codex` (per-model selection like `opus`/`sonnet` is future work). A single
stage listing two or more `agents:` is the other way to reach ≥2 candidates.

The judge agent is instructed to emit exactly:

```json
{"winner":"<container-name>","reason":"...","summary":"<PR-style summary of the winning change>"}
```

The verdict is parsed leniently (the first well-formed JSON object whose
`winner` is a real candidate wins), printed at pipeline end, and persisted to
`~/.berth/state/judge/<pipeline>-<stage>-<timestamp>.json`. If the judge
produces no usable verdict, the stage fails and the raw judge output is saved to
that same file for inspection.

With `auto_pr: true`, berth opens a PR from the winning candidate. The winner
container has already exited by then; berth does **not** restart it (that would
re-run its agent and could mutate the workspace). Instead it launches a
short-lived helper container that mounts the winner's volumes with
`--volumes-from` (carrying `/workspace` and the gh auth volume) but overrides the
entrypoint. The helper creates a dedicated head branch
(`berth/judge-<pipeline>-<stage>-<timestamp>`) from the candidate's committed
work — never the cloned default branch — pushes it, and opens the PR (base
`base`, body = the judge summary with the reason appended).

Because SSH-agent auth is per-container and cannot reach the helper, **`auto_pr`
requires the winning candidate to have GitHub HTTPS auth** — spawn candidates
with `reuse_gh_auth: true`. Candidates that can only push over SSH will fail the
push, and the helper surfaces a clear error. A PR failure is always reported as a
warning without discarding the verdict.

See `examples/pipeline-judge-fanout.yaml` for a complete runnable manifest.

## `config`

Subcommands:

### `config keys`

```bash
berth config keys
```

Lists every config key with its current and default value. Legacy env-style keys (`BERTH_*`) work as aliases for the canonical dotted form in `get`, `set`, and `reset`.

### `config show`

```bash
berth config show
```

Reads `~/.berth/config.toml`.
Set `BERTH_CONFIG_HOME` to read from another berth config home
without changing the process `HOME`.

### `config get`

```bash
berth config get <key>
```

Examples:

```bash
berth config get defaults.memory
berth config get BERTH_DEFAULT_MEMORY
```

### `config set`

```bash
berth config set <key> <value>
```

Examples:

```bash
berth config set defaults.memory 16g
berth config set defaults.identity "Your Name <you@example.com>"
```

### `config reset`

```bash
berth config reset <key>
```

No additional flags beyond `--help`.

## `template`

Subcommands:

### `template list`

```bash
berth template list
```

User templates live in `~/.berth/templates/`.

### `template show`

```bash
berth template show <name>
```

### `template render`

```bash
berth template render <name>
```

### `template create`

```bash
berth template create <name>
```

No additional flags beyond `--help`.

## `pipeline` saved catalog

Saved user pipelines live in `~/.berth/pipelines/`.

## `pr-review`

Usage:

```bash
berth pr-review [claude|codex|dual] [pr]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--dry-run` | bool | print the resolved review pipeline without running |
| `--repo` | strings | default repo URL; inferred from current checkout when omitted |
| `--var` | strings | workflow variable assignment `key=value`; repeatable |

Behavior:
- defaults to `dual`
- infers current PR via `gh pr view --json number` when omitted
- runs one-shot review presets without the watcher loop

## `pr-fix`

Usage:

```bash
berth pr-fix [pr]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--dry-run` | bool | print the resolved fix pipeline without running |
| `--repo` | strings | default repo URL; inferred from current checkout when omitted |
| `--var` | strings | workflow variable assignment `key=value`; repeatable |

## `mcp-login`

Usage:

```bash
berth mcp-login <service> [container]
```

No additional flags beyond `--help`.

## `cron`

Subcommands:

### `cron add`

```bash
berth cron add <name> <schedule> <command...>
```

Accepted schedule styles:
- `every 1h`
- `every 6h`
- `every 30m`
- `daily 09:00`
- standard cron expressions like `0 */6 * * *`

### `cron list`

```bash
berth cron list
```

### `cron remove`

```bash
berth cron remove <name>
```

### `cron enable`

```bash
berth cron enable <name>
```

### `cron disable`

```bash
berth cron disable <name>
```

### `cron run`

```bash
berth cron run <name>
```

### `cron daemon`

```bash
berth cron daemon
```

No additional flags beyond `--help`.

## `tui`

Usage:

```bash
berth tui
```

No command-specific flags beyond `--help`.

See [TUI Reference](tui.md) for keybindings, modes, and interaction model.

## `setup`

Usage:

```bash
berth setup
```

No command-specific flags beyond `--help`.

## `update`

Usage:

```bash
berth update [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--full` | bool | full rebuild without cache |
| `--quick` | bool | bust only the AI CLI layer |

## `diagnose`

Usage:

```bash
berth diagnose
```

No command-specific flags beyond `--help`.

`diagnose` also prints the effective spawn defaults from `~/.berth/config.toml` and warns when defaults widen the sandbox, such as default SSH forwarding, shared auth, Docker access, setup hooks, or custom networking.

## `cleanup`

Usage:

```bash
berth cleanup [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--auth` | bool | also remove shared and isolated auth volumes |

## `stop`

Usage:

```bash
berth stop <name|--latest|--all> [flags]
```

Flags:

| Flag | Type | Meaning |
|---|---|---|
| `--all` | bool | stop and remove all agent containers |
| `--latest` | bool | target the latest container |

## `vm`

Subcommands:

### `vm start`

```bash
berth vm start
```

### `vm stop`

```bash
berth vm stop
```

### `vm ssh`

```bash
berth vm ssh
```

No additional flags beyond `--help`.
