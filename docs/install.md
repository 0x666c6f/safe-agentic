# Installation

berth runs on macOS and needs two things: [Apple container](https://github.com/apple/container) (the VM runtime) and the `berth` CLI.

## 1. Install Apple container

Install the signed `.pkg` from Apple's GitHub Releases:

```bash
open https://github.com/apple/container/releases
```

## 2. Install berth

=== "Homebrew (recommended)"

    ```bash
    brew tap 0x666c6f/tap
    brew install berth
    ```

=== "From source"

    ```bash
    git clone git@github.com:0x666c6f/berth.git
    cd berth
    make build-all
    export PATH="$PWD/bin:$PATH"
    ```

This installs:

| Binary | Purpose |
|---|---|
| `berth` | the CLI — everything goes through it |
| `berth-claude` / `berth-codex` | one-line spawn shortcuts (auto-enable `--ssh` for `git@` URLs) |
| `berth-tui` | the dashboard binary, launched via `berth tui` |

Check the install:

```bash
berth --version
```

## 3. Build the sandbox

```bash
berth setup
```

`berth setup` is idempotent and safe to re-run. It:

- starts Apple container and creates the `berth` machine
- applies VM hardening (Docker `userns-remap`, masked host paths)
- configures host NAT so the VM and nested Docker have egress
- builds the local agent image

!!! note "Administrator prompt"
    Setup may ask for macOS administrator approval to enable IP forwarding and load a `com.apple/berth` PF NAT anchor. A macOS reboot resets these; `berth vm start` re-applies them.

Verify everything is healthy:

```bash
berth diagnose
```

`diagnose` checks the VM, egress, image, worktree posture, and flags risky spawn defaults. If anything fails later, its output is the first thing to look at.

## Keeping it up to date

```bash
brew upgrade berth      # new CLI release
berth update            # rebuild the agent image (cached)
berth update --quick    # rebuild only the AI CLI layer
berth update --full     # full rebuild, no cache
```

## Migrating from safe-agentic

berth is the renamed safe-agentic; old GitHub URLs redirect. Binaries went from `safe-ag*` to `berth*`, env vars from `SAFE_AGENTIC_*` to `BERTH_*`, the config home from `~/.safe-ag` to `~/.berth`, and the VM is now named `berth`.

```bash
brew uninstall safe-agentic && brew install 0x666c6f/tap/berth
mv ~/.safe-ag ~/.berth        # keeps config, templates, and state (merge if ~/.berth exists)
berth setup                   # creates the new VM and image
container machine stop safe-agentic && container machine rm safe-agentic
```

If `defaults.worktrees_dir` in `config.toml` points under `~/.safe-ag`, update it after the move.

## Next

- [Quickstart](quickstart.md) — spawn your first agent
- [Configuration](guide/configuration.md) — defaults, policy rules, profiles
