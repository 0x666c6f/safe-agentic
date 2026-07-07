# Threat Model

This page is the short version of what berth is trying to defend and what it is not trying to defend.

## Protected well

- host filesystem access by default
- automatic credential inheritance
- broad container privileges
- shared network state between normal sessions
- accidental use of the VM Docker daemon

## Protected conditionally

- SSH-backed repo access when `--ssh` is enabled
- shared auth when `--reuse-auth` or `--reuse-gh-auth` is enabled
- AWS access when `--aws` is enabled
- host home isolation, unless the worktree mount is enabled (see below)

Those are explicit tradeoffs, not bugs.

## The worktree mount (`--worktree`) trade-off

By default the Apple container machine is created with `--home-mount none`: **the host home directory is never shared with the VM.** A VM-root compromise or a Docker escape inside the machine cannot reach host files, because the host filesystem is not exposed to the machine at any level.

Apple's `container` provides no way to mount a single host directory into a machine — the only host-sharing knob is `--home-mount ro|rw|none`. So supporting `--worktree` (which needs a host git worktree bind-mounted into the agent container) requires sharing the home directory. This is **opt-in** and off by default:

- `berth setup --enable-worktrees` (or `berth config set defaults.worktrees_mount true`) sets the machine to `home-mount=rw`.
- `vm/setup.sh` then bind-mounts **only** the worktrees root (`~/.berth/worktrees`, or `defaults.worktrees_dir`) to a stable `/worktrees`, **detaches** the rest of the home share (lazy unmount), and tmpfs-masks `/Users`, `/Volumes`, `/private`, `/mnt/mac`.
- Agent containers only ever bind-mount a per-agent subdirectory of `/worktrees` into `/workspace`, so an agent never sees `/Users` or the rest of your home.

**What this protects, and what it does not:**

- Against the *agent* (the untrusted party): the agent is confined to its Docker container and only receives `/worktrees/<name>`. It cannot reach the rest of your home. This is unchanged from the default posture.
- Against a *VM-level* compromise (VM-root, or a Docker escape to the machine): weaker than the default. Because `home-mount=rw` shares the whole home with the machine at the virtiofs level, a sufficiently privileged actor inside the VM could re-mount the home share and reach host files. The detach + mask raise the bar (the guest-side mount is removed, not merely hidden), but the hypervisor-level share still exists. With `home-mount=none` it does not exist at all.

**Guidance:** leave the worktree mount disabled unless you need it. When enabled, keep secrets, credentials, and unrelated projects out of the worktrees root — treat everything under it as visible to the VM. `berth diagnose` reports whether the mount is enabled and flags the weakened posture; `berth setup --disable-worktrees` restores `home-mount=none`.

## Not the goal

berth does not attempt to limit what the agent does inside its own workspace once you have given it a task.

If the agent is malicious or simply wrong, it can still:
- edit repo files
- delete workspace data
- generate bad code
- use the network it has been granted

The sandbox boundary is the product. Inside that boundary, the agent is intentionally powerful.
