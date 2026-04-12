---
hide:
  - navigation
  - toc
---

<div class="hero" markdown>

# safe-agentic

**Let AI agents write code freely — inside a hardened sandbox.**

Claude Code and Codex get full autonomy inside isolated containers with read-only filesystems, dropped capabilities, and dedicated networks. Dangerous features require explicit opt-in.

```bash
safe-ag setup                                      # one-time VM + image setup
safe-ag spawn claude --repo git@github.com:myorg/myrepo.git     # spawn an agent
```

[Get Started](quickstart.md){ .md-button .md-button--primary }
[Usage Guide](usage.md){ .md-button }

</div>

---

## How it works

```
macOS Host
  └── OrbStack VM (Ubuntu 24.04, hardened)
       └── Docker container (ephemeral, per-agent)
            ├── Read-only rootfs, cap-drop ALL, no-new-privileges
            ├── Dedicated bridge network (egress: TCP 22/80/443 only)
            ├── 8 GB memory, 4 CPUs, 512 PIDs (configurable)
            └── SSH, auth, Docker, AWS — all OFF unless you opt in
```

Three isolation boundaries separate your machine from the agent. The agent has full freedom inside its container — but nothing leaks out.

---

## Spawn, work, review, merge

```bash
# Spawn with a task or a built-in template
safe-ag spawn claude --ssh --repo git@github.com:org/api.git \
  --prompt "Fix the failing CI tests"
safe-ag spawn claude --ssh --repo git@github.com:org/api.git \
  --template security-audit

# Check what it's doing
safe-ag peek --latest

# Quick status overview
safe-ag summary --latest

# Review its changes
safe-ag diff --latest
safe-ag review --latest

# Extract the last agent message
safe-ag output --latest

# Track what's left before merging
safe-ag todo add --latest "Run integration tests"
safe-ag todo add --latest "Update changelog"
safe-ag todo list --latest

# Create a PR when ready
safe-ag todo check --latest 1
safe-ag todo check --latest 2
safe-ag pr --latest --title "fix: resolve CI failures"
```

---

## Manage a fleet

Spawn multiple agents from a manifest:

```yaml
# fleet.yaml
agents:
  - name: api-tests
    type: claude
    repo: git@github.com:org/api.git
    ssh: true
    prompt: "Fix failing tests"

  - name: security-audit
    type: claude
    repo: git@github.com:org/api.git
    ssh: true
    template: security-audit   # built-in template, no prompt needed
    background: true
    auto_trust: true

  - name: frontend-lint
    type: codex
    repo: git@github.com:org/frontend.git
    prompt: "Fix all linting errors"
```

```bash
safe-ag fleet fleet.yaml
safe-ag tui                 # interactive dashboard to monitor all agents
```

Or orchestrate multi-step workflows:

```bash
safe-ag pipeline pipeline.yaml   # sequential steps with retry + failure handlers
```

---

## Safe by default

| | Default | Opt-in |
|---|---|---|
| SSH agent | OFF | `--ssh` |
| Auth tokens | Ephemeral (per-session) | `--reuse-auth` |
| AWS credentials | OFF | `--aws <profile>` |
| Docker access | OFF | `--docker` |
| Filesystem | Read-only | — |
| Capabilities | All dropped | — |
| Network | Dedicated bridge, filtered egress | `--network` |
| Sudo | Removed | — |

---

## Track everything

```bash
safe-ag cost --latest        # estimate API spend from token usage
safe-ag audit                # every spawn, stop, attach — timestamped JSONL
safe-ag sessions --latest    # export full conversation history
```

---

## What's inside the container

| Category | Tools |
|----------|-------|
| **AI agents** | Claude Code, Codex |
| **Infra** | Terraform, kubectl, Helm, AWS CLI, Vault, Docker, Compose |
| **CLI** | ripgrep, fd, bat, eza, zoxide, fzf, jq, yq, delta, gh |
| **Runtimes** | Node.js 22, pnpm, Bun, Python 3.12, Go 1.23 |

---

<div class="grid cards" markdown>

-   [**Quick Start** :material-arrow-right:](quickstart.md)

    Zero to running agent in 5 minutes.

-   [**Usage Guide** :material-arrow-right:](usage.md)

    Full command reference with examples.

-   [**Architecture** :material-arrow-right:](architecture.md)

    Isolation boundaries, sequence diagrams.

-   [**Security Model** :material-arrow-right:](security.md)

    Threat model and supply chain analysis.

</div>
