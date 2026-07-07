# Security

berth is designed around one rule:

> dangerous capabilities should be explicit, not ambient

It constrains **what the sandbox can reach**, **which credentials enter it**, and **how much of the host/runtime it can control**. It does not try to constrain what the agent does inside its own workspace — inside the boundary, the agent is intentionally powerful.

## Default posture

| Area | Default |
|---|---|
| SSH agent | off |
| Claude/Codex auth reuse | off |
| GitHub auth reuse | off |
| Host Claude/Codex auth seeding | off |
| AWS credentials | off |
| Docker access | off |
| Repo setup scripts | off |
| Root filesystem | read-only |
| Linux capabilities | all dropped, `no-new-privileges` |
| Network | managed per-agent bridge with private-range egress guardrails |
| Resources | 8g memory / 4 CPU / 512 PIDs |
| Host key trust | pinned GitHub host keys, `StrictHostKeyChecking yes` |
| Sudo | unavailable |
| Host home in the VM | not shared (`home-mount=none`) |

The default session is intentionally useful but narrow: enough for public repos and normal coding tasks, not enough to silently inherit host credentials or Docker control.

## What each opt-in flag widens

| Flag | Adds | Trade-off |
|---|---|---|
| `--ssh` | auth to SSH remotes; push to repos your identity can write to | container gains signing ability (never the key material) |
| `--reuse-auth` | Claude/Codex auth persists across sessions | any session reusing the volume reads the same token state |
| `--reuse-gh-auth` | persistent GitHub CLI auth | shared token state in the auth volume |
| `--seed-auth` | one-shot copy of host Claude/Codex auth into the session | the agent holds those credentials for the session's life |
| `--aws <profile>` | AWS API access for that profile | the agent can do whatever the profile can |
| `--docker` | Docker-in-Docker sidecar | broader runtime capability inside the session |
| `--docker-socket` | direct control of the VM Docker daemon | much larger blast radius than `--docker` |
| `--network <name>` | custom topology | you leave the managed guardrails and own the consequences |
| `--allow-setup-scripts` | repo-provided `berth.json` hooks run at clone time | the repo's author gets code execution in the sandbox |

Interactive spawns print a risk confirmation before widening anything (`--yes` skips it). To make limits non-negotiable regardless of flags, use [policy rules](guide/configuration.md#policy-rules):

```toml
# ~/.berth/rules.toml or .berth/rules.toml
[allow]
docker_modes = ["off"]
networks = ["managed"]
ssh = false
reuse_auth = false
seed_auth = false
```

## Threat model

**Protected by default:**

- host filesystem access
- automatic credential inheritance
- broad container privileges
- shared network state between normal sessions
- accidental use of the VM Docker daemon

**Protected conditionally** (explicit trade-offs, not bugs):

- SSH-backed repo access when `--ssh` is on
- shared auth when reuse/seed flags are on
- AWS access when `--aws` is on
- host home isolation, unless the worktree mount is enabled (below)

### The worktree mount trade-off

By default the machine is created with `--home-mount none`: the host home is never shared with the VM, so even a VM-root compromise or Docker escape cannot reach host files.

Apple's `container` has no way to mount a single host directory into a machine — the only knob is `--home-mount ro|rw|none`. Supporting [`--worktree`](guide/worktrees.md) therefore requires `home-mount=rw`. When you opt in with `berth setup --enable-worktrees`, `vm/setup.sh` binds **only** the worktrees root to `/worktrees`, detaches the rest of the home share, and tmpfs-masks `/Users`, `/Volumes`, `/private`, `/mnt/mac`. Agent containers only ever see their per-agent subdirectory.

What that means:

- **Against the agent:** unchanged — it's confined to its container and its own worktree.
- **Against a VM-level compromise:** weaker than the default. The virtiofs home share still exists at the hypervisor level; the detach and masks raise the bar but don't remove it. With `home-mount=none` it doesn't exist at all.

Leave it disabled unless you need it. When enabled, treat everything under the worktrees root as visible to the VM and keep secrets and unrelated projects out. `berth diagnose` reports the posture; `berth setup --disable-worktrees` restores the strong default.

## Supply chain

The image build is part of the security story:

| Source | Control |
|---|---|
| base image | pinned by digest |
| direct downloads | checksum verification |
| AWS CLI | signature verification |
| apt packages | signed repositories |
| Codex install | lockfile-based `npm ci` |

The build context is **tracked files only**, not your working directory — which keeps `.env` files, local secrets, and untracked scratch out of the image. Residual trust in upstream signing roots and registries remains; the controls narrow supply-chain risk, they don't eliminate it.

## Not the goal

Once you hand the agent a task, berth does not limit what it does inside its workspace. A malicious or simply wrong agent can still edit repo files, delete workspace data, generate bad code, and use whatever network it was granted. Two flags deserve respect accordingly:

- Claude runs with `--dangerously-skip-permissions`, Codex in `--yolo` mode — **the container is the sandbox**
- with `--ssh`, that includes pushing to any repo your SSH identity can write to

## Related

- [Architecture](architecture.md) — the three boundaries in detail
- [Worktrees](guide/worktrees.md) — enabling and using the opt-in mount
- [Configuration](guide/configuration.md#policy-rules) — hard policy limits
