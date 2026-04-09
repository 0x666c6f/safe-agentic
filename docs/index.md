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
agent setup                                      # one-time VM + image setup
agent-claude git@github.com:myorg/myrepo.git     # spawn an agent
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
# Spawn an agent with a task
agent spawn claude --ssh --repo git@github.com:org/api.git \
  --prompt "Fix the failing CI tests"

# Check what it's doing
agent peek --latest

# Review its changes
agent diff --latest
agent review --latest

# Track what's left before merging
agent todo add --latest "Run integration tests"
agent todo add --latest "Update changelog"
agent todo list --latest

# Create a PR when ready
agent todo check --latest 1
agent todo check --latest 2
agent pr --latest --title "fix: resolve CI failures"
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

  - name: frontend-lint
    type: codex
    repo: git@github.com:org/frontend.git
    prompt: "Fix all linting errors"
```

```bash
agent fleet fleet.yaml
agent tui                 # interactive dashboard to monitor all agents
```

Or orchestrate multi-step workflows:

```bash
agent pipeline pipeline.yaml   # sequential steps with retry + failure handlers
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
agent cost --latest        # estimate API spend from token usage
agent audit                # every spawn, stop, attach — timestamped JSONL
agent sessions --latest    # export full conversation history
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
