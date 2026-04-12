# Usage Guide

## Commands at a glance

| Command | Purpose |
|---|---|
| `safe-ag setup` | Create/harden VM, build image |
| `safe-ag spawn <claude|codex|shell>` | Start container |
| `safe-ag run <repo...> [prompt]` | Quick-start wrapper around `spawn` |
| `safe-ag list` | List containers |
| `safe-ag attach <name>` | Reattach to tmux session |
| `safe-ag peek <name>` | Tail latest visible output |
| `safe-ag summary <name>` | Show safe-ag summary |
| `safe-ag logs <name>` | Show conversation log |
| `safe-ag output <name>` | Show final output / derived view |
| `safe-ag diff <name>` | Show git diff |
| `safe-ag checkpoint ...` | Save / list / revert checkpoints |
| `safe-ag todo ...` | Track merge requirements |
| `safe-ag review <name>` | Run review over changes |
| `safe-ag pr <name>` | Push + open PR |
| `safe-ag aws-refresh <name>` | Refresh AWS credentials in a running container |
| `safe-ag mcp-login <service> [container]` | Authenticate an MCP service |
| `safe-ag config ...` | Manage persistent defaults |
| `safe-ag template ...` | Manage prompt templates |
| `safe-ag fleet manifest.yaml` | Parallel fleet |
| `safe-ag pipeline pipeline.yaml` | Sequential orchestration |
| `safe-ag tui` | Launch TUI |
| `safe-ag dashboard` | Launch web dashboard |
| `safe-ag cleanup [--auth]` | Remove containers and optional auth volumes |

## Typical flows

### Spawn

```bash
safe-ag spawn claude --repo https://github.com/myorg/myrepo.git
safe-ag spawn claude --ssh --repo git@github.com:myorg/private.git
safe-ag spawn codex --ssh --reuse-auth --repo git@github.com:myorg/private.git
```

### Quick-start

```bash
safe-ag run git@github.com:org/api.git "Fix the failing tests"
safe-ag run https://github.com/org/repo.git --background
```

### Inspect

```bash
safe-ag list
safe-ag peek --latest
safe-ag summary --latest
safe-ag diff --latest
safe-ag output --latest
safe-ag logs --latest
```

### Manage

```bash
safe-ag attach --latest
safe-ag stop --latest
safe-ag stop --all
safe-ag cleanup
safe-ag cleanup --auth
```

### TUI

```bash
safe-ag tui
safe-ag dashboard --bind localhost:8420
```

### Orchestration

```bash
safe-ag fleet manifest.yaml
safe-ag pipeline pipeline.yaml
```

## Important flags

| Flag | Effect |
|---|---|
| `--ssh` | Forward SSH agent |
| `--reuse-auth` | Persist Claude/Codex auth |
| `--reuse-gh-auth` | Persist GitHub auth |
| `--aws PROFILE` | Inject AWS credentials |
| `--docker` | DinD sidecar |
| `--docker-socket` | Mount VM Docker socket |
| `--network NAME` | Join custom network |
| `--background` | Detach immediately |
| `--prompt TEXT` | Initial prompt |
| `--template NAME` | Template prompt |
| `--instructions TEXT` | Extra context |
| `--auto-trust` | Skip trust prompt |
| `--dry-run` | Print plan only |

## Defaults

Persistent defaults live in:

```bash
${XDG_CONFIG_HOME:-~/.config}/safe-agentic/defaults.sh
```

Useful keys:

```bash
SAFE_AGENTIC_DEFAULT_IDENTITY="Your Name <you@example.com>"
SAFE_AGENTIC_DEFAULT_MEMORY="12g"
SAFE_AGENTIC_DEFAULT_CPUS="6"
SAFE_AGENTIC_DEFAULT_REUSE_AUTH="1"
```

## VM override

```bash
SAFE_AGENTIC_VM_NAME=safe-agentic-alt safe-ag list
```

Use this when you want the CLI to target a different OrbStack VM than the default `safe-agentic`.

## Troubleshooting

```bash
safe-ag diagnose
safe-ag vm start
safe-ag update
```

If `safe-ag-tui` is missing:

```bash
make build-tui
```
