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
- direct internet egress, when `--network api-only` is on (below)

### api-only egress mode

`--network api-only` is the recommended mode for analyzing untrusted or suspicious file content. The container's direct internet egress is dropped by an iptables `REJECT` on its bridge (see [Architecture](architecture.md#api-only-egress-mode)). External DNS is closed separately: Docker's embedded resolver forwards queries from the VM's own network namespace, which would bypass the bridge `REJECT`, so the container's resolver upstream is pointed at a blackhole (`--dns 127.0.0.1`) — external name resolution fails while `/etc/hosts` entries (the proxy) still resolve. All HTTP(S) traffic must go through a VM-side `tinyproxy` instance reachable only via the injected `HTTPS_PROXY=http://berth-proxy:8119`; the proxy resolves allowlisted targets VM-side, so the container needs no resolver of its own. That proxy is default-deny and forwards only to an exact-host allowlist:

- `api.anthropic.com`
- `statsig.anthropic.com`
- `sentry.io`
- `github.com`
- `codeload.github.com`
- `objects.githubusercontent.com`

**Blocks:** direct internet access on arbitrary hosts/ports and the container's own DNS resolution — a malicious file in the workspace can't open a socket or resolve a hostname to anywhere but those six hosts.

**Residual channels (not blocked):**

- **The model conversation itself.** Claude Code's own traffic to `api.anthropic.com` is allowed (that's how the agent works at all), so a payload could in principle exfiltrate small amounts of data by getting the agent to echo it back through the conversation. That path is low-bandwidth and logged in session history, not covert — but it's not zero.
- **The allowlisted GitHub clone path.** `github.com`, `codeload.github.com`, and `objects.githubusercontent.com` are allowlisted so the agent can clone over HTTPS — which means `git clone`/`fetch` against *any* public GitHub repo works through the proxy, not just the one the agent was spawned with. That's a real pull channel: a file or the agent itself can pull arbitrary public data or instructions (a possible C2/instruction channel). Push-exfil back to GitHub needs write credentials, which `--ephemeral-auth` (no shared auth) avoids.

**api-only is not a malware detonation sandbox.** It narrows network blast radius for static analysis; it does not sandbox execution. Treat all file content as untrusted data and never execute it. SSH clone doesn't work in this mode (port 22 is dropped) — use an HTTPS repo URL.

**Known limitation — container startup window.** A brief window at container startup was observed once (immediately after re-provisioning the VM) where direct egress succeeded before the bridge drop was fully effective; it did not reproduce on later spawns. The DNS blackhole narrows any such window (a hostname can't be resolved without a working resolver), and the agent does not process untrusted file content during init. If you require provably-zero egress from the first instant, treat this as an open gap pending root-cause rather than a guarantee.

### Forensic triage

`berth spawn --forensic` (also `berth run --forensic`) is for static triage of untrusted/suspicious files. It selects the `berth:forensic` image — built ahead of time with `berth update --forensic` from `Dockerfile.forensic` — and defaults `--network` to `api-only` when no network is given (an explicit `--network` wins). If `berth:forensic` hasn't been built, spawn fails closed with a hint to run `berth update --forensic`.

The image pre-bakes a static-analysis tool set, since `api-only` blocks `apt`/`pip` at runtime: `file`, `binutils` (`strings`/`objdump`), `xxd`, `yara`, `binwalk`, `exiftool`, `radare2`, `ssdeep`, and `oletools` (`olevba`/`oleid`) for Office macro analysis. The `forensic-triage` template drives this tool set with never-execute rules.

`clamav` is deliberately excluded — its signature database needs network updates that `api-only` blocks, so a stale scanner would be dead weight rather than protection.

`Dockerfile.forensic` layers `FROM berth:latest`, so build the base image first (`berth setup`, or `berth update`) before `berth update --forensic`. Its apt tools come from the same GPG-signed Ubuntu repos as the base image; `oletools` is version-pinned via pip, though its transitive Python dependencies are not hash-locked (on par with the base image's package installs, not stricter).

This is static analysis, not detonation: the tools inspect file structure, strings, metadata, and embedded content without running anything. `api-only`'s network narrowing (above) does not sandbox execution — never execute or open the files under analysis.

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
