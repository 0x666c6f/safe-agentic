# safe-agentic

Run Claude Code and Codex inside a hardened OrbStack VM with one container per agent.

The docs are organized around the questions users usually have:

- how do I get this running?
- how do I spawn and manage an agent?
- how do fleet/pipeline manifests work?
- what is safe by default and what is explicitly opt-in?

## Start here

If you are new:

1. [Quickstart](quickstart.md)
2. [Command Map](usage.md)
3. [Spawning Agents](guide/spawning.md)
4. [Managing Agents](guide/managing.md)

If you are evaluating the design:

1. [Architecture](architecture.md)
2. [Security](security.md)

## What safe-agentic actually is

```text
macOS host
  -> OrbStack VM
    -> Docker daemon
      -> one hardened container per agent
```

It is not a policy engine inside your editor. It is a host-side sandbox runner for autonomous coding agents.

## Core defaults

| Area | Default |
|---|---|
| SSH agent | off |
| Claude/Codex auth reuse | off |
| GitHub auth reuse | off |
| AWS credentials | off |
| Docker access | off |
| Root filesystem | read-only |
| Linux capabilities | all dropped |
| Network | dedicated managed bridge |
| Resource limits | 8g / 4 CPU / 512 PIDs |

## Main entrypoints

```bash
safe-ag setup
safe-ag spawn claude --repo https://github.com/myorg/myrepo.git
safe-ag list
safe-ag attach --latest
safe-ag diff --latest
safe-ag tui
```

## Main workflows

Single agent:

```bash
safe-ag spawn claude --ssh --repo git@github.com:org/api.git \
  --prompt "Fix the failing tests"
safe-ag peek --latest
safe-ag diff --latest
safe-ag review --latest
```

Parallel agents:

```bash
safe-ag fleet fleet.yaml
safe-ag tui
```

Sequential stages:

```bash
safe-ag pipeline pipeline.yaml
```

## Reading order

- [Quickstart](quickstart.md): installation and first successful run
- [Usage](usage.md): command map by task
- [Workflow](guide/workflow.md): review, retry, PR flow
- [Fleet and Pipelines](guide/fleet.md): manifests and staged runs
- [Configuration](guide/configuration.md): defaults, templates, image rebuilds
- [CLI Reference](reference/cli.md): top-level command inventory
- [TUI Reference](reference/tui.md): keybindings, modes, and dashboard entrypoints
