# Worktrees

`--worktree` gives an agent an isolated git worktree of your **current local checkout** — Codex-app-style task branches — while keeping the container sandbox. Instead of cloning from a remote, the agent works on a branch of the repo you're standing in, and the results are immediately available on your host.

Worktrees are **opt-in** because they widen the VM boundary — see [the trade-off](#security-trade-off) below.

## Enable once

```bash
berth setup --enable-worktrees
```

(or `berth config set defaults.worktrees_mount true` followed by `berth setup`). Disable again with `berth setup --disable-worktrees`. `berth diagnose` reports the current posture.

## Spawn on a worktree

Run from inside the source git checkout — don't pass `--repo`:

```bash
berth spawn claude --worktree --name auth-fix --prompt "Fix the auth tests"
berth spawn codex --worktree --worktree-branch berth/api-review --background
```

Defaults:

- path: `~/.berth/worktrees/<container-name>`
- branch: `berth/<container-name>`
- gitignored files listed in `.berthinclude` are copied into the worktree (env files, local fixtures)

`--worktree-path` must sit under the worktrees root; anything outside is rejected before launch.

## Snapshot, restore, clean up

```bash
berth worktree list                              # registered worktrees
berth worktree snapshot auth-fix "before review fixes"
berth worktree restore auth-fix stash@{0}
berth worktree cleanup --dry-run                 # prune missing entries
berth worktree cleanup --all                     # remove everything registered
```

## Hand off the result

```bash
berth handoff auth-fix --to-worktree     # print the host path of the worktree
berth handoff auth-fix --to-local ./copy # or copy /workspace anywhere
```

Since the worktree lives on your host, you can also just `cd ~/.berth/worktrees/auth-fix` and use normal git.

## How the mount works

Apple's `container` can't mount a single host directory into a machine — the only host-sharing knob is `--home-mount ro|rw|none`. berth's default is `none` (nothing shared). Enabling worktrees switches the machine to `home-mount=rw`, then `vm/setup.sh`:

1. binds **only** the worktrees root (`~/.berth/worktrees`, or `defaults.worktrees_dir`) to a stable `/worktrees` in the VM
2. detaches the rest of the home share (lazy unmount)
3. tmpfs-masks `/Users`, `/Volumes`, `/private`, `/mnt/mac`

Agent containers bind-mount only their per-agent subdirectory of `/worktrees` into `/workspace`. `berth setup` / `berth vm start` reconcile the machine to match the config in either direction.

## Security trade-off

Enabling the worktree mount is a deliberate weakening of the VM boundary:

- **Against the agent**: unchanged — it's confined to its container and sees only `/worktrees/<name>`.
- **Against a VM-level compromise** (VM-root or Docker escape): weaker than the default. `home-mount=rw` shares your whole home with the machine at the virtiofs level; the detach + masks raise the bar, but the hypervisor-level share still exists. With `home-mount=none` it doesn't exist at all.

**Guidance:** leave it disabled unless you need it, and keep secrets, credentials, and unrelated projects out of the worktrees root. Full analysis in [Security — threat model](../security.md#the-worktree-mount-trade-off).
