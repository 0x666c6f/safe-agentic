# Security Defaults

safe-agentic's security philosophy is **safe by default, dangerous on demand**. Every capability that expands the attack surface requires an explicit opt-in flag. There are no hidden permissions, no ambient credentials, and no implicit trust.

## Defaults vs opt-in

| Feature | Default (safe) | Opt-in flag | What changes |
|---------|---------------|-------------|-------------|
| SSH agent | **OFF** | `--ssh` | Your SSH keys become available inside the container |
| Auth tokens | **Ephemeral** | `--reuse-auth` | OAuth tokens persist across container restarts |
| GitHub CLI auth | **Ephemeral** | `--reuse-gh-auth` | `gh` login persists across container restarts |
| AWS credentials | **OFF** | `--aws <profile>` | AWS credentials injected (tmpfs-backed) |
| Docker access | **OFF** | `--docker` / `--docker-socket` | Container can build/run Docker images |
| Root filesystem | **Read-only** | — | Cannot be overridden |
| Linux capabilities | **All dropped** | — | Cannot be overridden |
| Privilege escalation | **Blocked** | — | `no-new-privileges` cannot be overridden |
| Network | **Dedicated bridge** | `--network <name>` | Joins custom network, bypasses egress filtering |
| Resources | **8g / 4 CPU / 512 PIDs** | `--memory`, `--cpus`, `--pids-limit` | Higher limits |
| GitHub host keys | **Baked + pinned** | — | `StrictHostKeyChecking yes`, cannot be overridden |
| Sudo | **Removed** | — | Not installed in image |
| Unsafe Docker flags | **Blocked** | — | `--privileged`, `host` network, `container:*` rejected |

## What "safe by default" means in practice

**Without any flags**, an agent container:

- Has no access to your SSH keys, AWS credentials, or browser tokens
- Cannot read any file from your Mac
- Cannot reach your local network or other containers
- Cannot modify its own filesystem
- Cannot escalate privileges
- Cannot survive a `docker rm` (everything is ephemeral)
- Is limited to 8 GB RAM, 4 CPUs, and 512 processes

The agent can:

- Read/write its workspace (`/workspace/`)
- Access the internet on ports 22, 80, 443
- Clone public repos
- Use installed tools (terraform, kubectl, ripgrep, etc.)

**Each flag you add expands the surface area.** The sections in [Threat Surface](threat-surface.md) explain exactly what each flag exposes.

## Blocked operations

These Docker flags are explicitly rejected by the Go CLI runtime:

- `--privileged` — gives the container full host capabilities
- `--network host` — shares the VM's network namespace
- `--network bridge` — the default Docker bridge (shared with all containers)
- `--network container:*` — shares another container's network namespace
- `--` passthrough — arbitrary Docker flags cannot be injected

These are blocked regardless of flags passed by the user.
