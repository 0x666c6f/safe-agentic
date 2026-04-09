# Threat Model

This page describes what safe-agentic protects against, what it doesn't, and the assumptions behind the security model.

## What's protected

### Malicious agent behavior

An AI agent running with `--dangerously-skip-permissions` (Claude) or `--yolo` (Codex) has full autonomy inside the container. It can run any command, modify any file, and make any network request. safe-agentic ensures this freedom is contained:

| Threat | Protection |
|--------|-----------|
| Agent reads your SSH keys | SSH off by default; even with `--ssh`, keys stay in 1Password |
| Agent reads macOS files | VM mounts blocked with tmpfs overlays |
| Agent modifies system binaries | Read-only rootfs |
| Agent escalates to root | cap-drop ALL + no-new-privileges + no sudo |
| Agent attacks other agents | Separate network namespaces + separate volumes |
| Agent exhausts host resources | Memory, CPU, PID limits |
| Agent exfiltrates to internal services | Egress filtered to TCP 22/80/443, private IPs blocked |
| Agent persists a backdoor | Container destroyed on stop; rootfs is immutable |
| Agent steals credentials | No credentials by default; opt-in flags are explicit |

### Malicious repository content

When working with untrusted repos:

| Threat | Protection |
|--------|-----------|
| Repo contains malicious build scripts | Scripts run inside the sandboxed container |
| Repo tries path traversal in clone | `repo_clone_path()` validates and rejects `../`, dot-prefixed names |
| Repo tries to access host via network | Per-container bridge with egress filtering |
| Repo needs full isolation | `--network agent-isolated` blocks all internet |

### Supply chain attacks

| Threat | Protection |
|--------|-----------|
| Tampered base image | Digest-pinned `FROM` |
| Tampered binary downloads | SHA256 checksums per architecture |
| Tampered AWS CLI | GPG signature verification |
| Tampered apt packages | Signed repositories with pinned GPG keys |
| Build context includes secrets | Only git-tracked files sent |

## What's NOT protected

### Agent within its sandbox

safe-agentic does not restrict what the agent does **inside** its container. The agent can:

- Delete all files in `/workspace/`
- Generate and commit malicious code
- Make API calls to external services (within egress rules)
- Consume all allocated CPU/memory/PIDs

The container is the sandbox. Everything inside it is the agent's domain.

### With `--ssh` enabled

An agent with SSH access can push to any repository your key can access. If the agent is compromised or generates malicious code, it could push that code to your repos.

**Mitigation:** Use branch protection rules, required reviews, and CI checks on your repositories.

### With `--reuse-auth` enabled

Multiple containers sharing an auth volume can read each other's OAuth tokens. A compromised container could exfiltrate the token.

**Mitigation:** Use ephemeral auth (the default) for untrusted work. Run `agent cleanup --auth` to revoke tokens.

### OrbStack VM escape

safe-agentic's VM hardening is best-effort. OrbStack is designed for developer convenience, not as a security boundary. A sophisticated attacker who escapes the container AND the Docker userns-remap AND bypasses the tmpfs mounts could potentially access host resources.

**Mitigation:** The three-boundary defense-in-depth model means all three layers would need to be breached simultaneously.

## Assumptions

1. **OrbStack is trusted** — it manages the VM and provides the Linux kernel
2. **The Docker daemon is trusted** — container isolation depends on it
3. **Apt signing keys are not compromised** — Ubuntu, HashiCorp, Google, GitHub, NodeSource
4. **1Password SSH agent is secure** — private keys never leave 1Password
5. **The user understands opt-in flags** — each flag explicitly expands the attack surface

## Risk levels by configuration

| Configuration | Risk level | Use case |
|--------------|-----------|----------|
| Default (no flags) | **Low** | Public repos, untrusted code |
| `--ssh` | **Medium** | Private repos you own |
| `--ssh --reuse-auth` | **Medium** | Daily development on trusted repos |
| `--ssh --reuse-auth --aws <profile>` | **High** | Infrastructure work |
| `--ssh --docker-socket` | **Very high** | Full VM Docker control |
| `--network agent-isolated` (no SSH) | **Minimal** | Maximum isolation for untrusted code |
