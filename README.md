<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/brand/berth-readme-header-b2-dark-800x200.png">
    <img src="docs/assets/brand/berth-readme-header-b2-light-800x200.png" alt="berth" width="400">
  </picture>
</p>

<p align="center">
  <a href="https://github.com/0x666c6f/berth/actions/workflows/ci.yml"><img src="https://github.com/0x666c6f/berth/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/go-1.25.5-00ADD8?logo=go" alt="Go Version"></a>
  <a href="https://github.com/0x666c6f/berth/actions/workflows/coverage.yml"><img src="https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2F0x666c6f%2Fberth%2Fbadges%2F.github%2Fbadges%2Fgoreport.json" alt="Go Report"></a>
  <a href="https://github.com/0x666c6f/berth/actions/workflows/coverage.yml"><img src="https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2F0x666c6f%2Fberth%2Fbadges%2F.github%2Fbadges%2Fcoverage.json" alt="Coverage"></a>
</p>

<p align="center">
  Run Claude Code and Codex in hardened sandboxes on macOS — one isolated container per agent,<br>
  inside a dedicated Apple container machine. Everything risky is opt-in.
  <br><br>
  <a href="https://0x666c6f.github.io/berth/"><b>Documentation</b></a> ·
  <a href="https://0x666c6f.github.io/berth/install/">Install</a> ·
  <a href="https://0x666c6f.github.io/berth/quickstart/">Quickstart</a> ·
  <a href="https://0x666c6f.github.io/berth/security/">Security model</a>
</p>

---

The agent operates freely **inside** its sandbox; the sandbox is the boundary. Host access, shared auth, SSH, AWS credentials, and Docker access all stay off until you flip a flag.

```text
macOS host (berth CLI / TUI / desktop app)
  -> Apple container machine "berth" (hardened)
    -> Docker daemon (userns-remap)
      -> one container per agent
```

## Why berth

- **Real isolation, not editor policy.** Read-only rootfs, `cap-drop ALL`, `no-new-privileges`, resource limits, per-agent bridge networks with egress guardrails — three boundaries deep (host → VM → container → container).
- **Safe by default.** No SSH, no shared auth, no host credentials, no Docker access unless you opt in — and hard [policy rules](https://0x666c6f.github.io/berth/guide/configuration/#policy-rules) can deny risky spawns entirely.
- **Built for daily work.** tmux-backed sessions you can reattach, steer, diff, review, checkpoint, and turn into PRs — from the CLI, a k9s-style TUI, or a native macOS app.
- **Scales past one agent.** Fleet manifests for parallel fan-out, pipelines with dependencies, judge stages that pick the best of N candidates, scheduled runs, and a JSON-RPC state server.

## Install

Install [Apple container](https://github.com/apple/container/releases) from the signed pkg, then:

```bash
brew tap 0x666c6f/tap
brew install berth
berth setup       # creates the VM, hardens it, builds the image
berth diagnose    # verify everything is green
```

From source: `git clone`, `make build-all`, add `bin/` to your PATH. Migrating from safe-agentic? See the [migration notes](https://0x666c6f.github.io/berth/install/#migrating-from-safe-agentic).

## First agent

```bash
# Quick start with smart defaults
berth run https://github.com/myorg/myrepo.git "Fix the failing CI tests"

# Full control
berth spawn claude --ssh --repo git@github.com:myorg/myrepo.git --prompt "Fix the failing CI tests"
```

Then work the loop:

```bash
berth status --latest     # blocked / working / done / idle / exited
berth peek --latest       # snapshot the live terminal
berth steer --latest "keep the fix narrow"
berth diff --latest       # what changed
berth review --latest     # AI review pass
berth pr --latest         # push branch + open a PR
berth stop --latest
```

Multiple agents? `berth tui` for a live dashboard, `berth inbox` for what needs attention, or the [desktop app](https://0x666c6f.github.io/berth/guide/app/) for embedded terminals and native notifications.

## Going further

```bash
berth fleet fleet.yaml                       # parallel agents from one manifest
berth pipeline pipeline.yaml                 # staged execution with depends_on + judge stages
berth pr-review                              # one-shot dual Claude+Codex PR review
berth cron add nightly "daily 02:00" pipeline review   # scheduled runs
berth spawn claude --worktree --prompt ...   # work on your local checkout (opt-in)
```

## Safety model

If you only need a public repo and a prompt, don't add flags you don't need. Each widener is explicit:

| Flag | Why you'd use it | What it widens |
|---|---|---|
| `--ssh` | private repos, pushes | repo access through your SSH agent |
| `--reuse-auth` | avoid re-auth | shared agent auth volume |
| `--reuse-gh-auth` | `gh` inside containers | shared GitHub auth volume |
| `--seed-auth` | skip first login | one-shot copy of host Claude/Codex auth |
| `--aws <profile>` | infra work | AWS API access |
| `--docker` | build/test containers | DinD sidecar |
| `--docker-socket` | full Docker control | direct VM daemon access |
| `--network <name>` | custom connectivity | leaves managed network policy |
| `--worktree` | local checkout in the sandbox | VM home-mount boundary ([trade-off](https://0x666c6f.github.io/berth/security/#the-worktree-mount-trade-off)) |

Full threat model, defaults, and supply-chain notes: [Security](https://0x666c6f.github.io/berth/security/).

## Documentation

| | |
|---|---|
| [Installation](https://0x666c6f.github.io/berth/install/) | toolchain, VM setup, migration |
| [Quickstart](https://0x666c6f.github.io/berth/quickstart/) | first agent in five minutes |
| [Guides](https://0x666c6f.github.io/berth/guide/spawning/) | spawning, managing, review & ship, worktrees, fleets, automation, TUI, desktop app, configuration |
| [CLI reference](https://0x666c6f.github.io/berth/reference/cli/) | every command and flag |
| [Architecture](https://0x666c6f.github.io/berth/architecture/) | the three isolation boundaries |
| [Security](https://0x666c6f.github.io/berth/security/) | defaults, wideners, threat model |

## Notes

- containers persist after the agent exits; `berth attach` restarts stopped containers
- `berth cleanup` keeps auth volumes; `berth cleanup --auth` is the full reset
- a macOS reboot resets the host NAT the VM needs — `berth vm start` re-applies it
- `BERTH_VM_NAME`, `BERTH_CONFIG_HOME`, `BERTH_STATE_HOME` relocate the VM/config/state

## License

[MIT](LICENSE)
