---
hide:
  - navigation
  - toc
---

<div class="landing-hero" markdown="1">

<p class="landing-eyebrow">safe-agentic</p>

# Sandbox coding agents without handing them your host

Launch Claude Code and Codex inside a hardened OrbStack VM, with one isolated container per agent and risky capabilities kept opt-in.

<div class="landing-terminal" markdown="1">

```bash
brew install orbstack
brew tap 0x666c6f/tap
brew install safe-agentic
safe-ag setup
```

</div>

[Get Started](quickstart.md){ .md-button .md-button--primary }
[Command Map](usage.md){ .md-button }
[GitHub](https://github.com/0x666c6f/safe-agentic){ .md-button }

<div class="landing-meta">
  <span>macOS host</span>
  <span>hardened OrbStack VM</span>
  <span>one container per agent</span>
</div>

</div>

<div class="landing-callout" markdown="1">

`safe-agentic` is a sandbox runner for autonomous coding agents. It is not an editor policy layer pretending to be isolation.

</div>

## Start with the page that matches the job

<div class="grid cards" markdown>

-   :material-rocket-launch-outline: __First successful run__

    Install the toolchain, harden the VM, spawn an agent, inspect the session.

    [Open Quickstart](quickstart.md)

-   :material-console-network-outline: __Daily command map__

    Jump straight to the command you need for setup, attach, diff, retry, or PR flow.

    [Open Command Map](usage.md)

-   :material-source-fork: __Parallel and staged workflows__

    Use fleet manifests, pipelines, retries, and TUI tooling for multi-agent work.

    [Open Fleet Guide](guide/fleet.md)

-   :material-shield-lock-outline: __Architecture and security__

    Understand trust boundaries, default isolation, and every flag that widens access.

    [Open Security Model](security.md)

</div>

## What you can do with it

<div class="landing-panel-grid">
  <a class="landing-panel" href="guide/spawning/">
    <span class="landing-panel-kicker">Spawn</span>
    <strong>Launch Claude Code, Codex, or a shell in an isolated container.</strong>
    <code>safe-ag spawn codex --repo ...</code>
  </a>
  <a class="landing-panel" href="guide/workflow/">
    <span class="landing-panel-kicker">Review</span>
    <strong>Peek at output, diff the workspace, review changes, then open a PR.</strong>
    <code>safe-ag peek --latest</code>
  </a>
  <a class="landing-panel" href="guide/fleet/">
    <span class="landing-panel-kicker">Orchestrate</span>
    <strong>Run fleets and pipelines for fan-out review, staged fixes, and consolidation.</strong>
    <code>safe-ag fleet manifest.yaml</code>
  </a>
  <a class="landing-panel" href="guide/tui/">
    <span class="landing-panel-kicker">Observe</span>
    <strong>Use the TUI to monitor live sessions without dropping into Docker.</strong>
    <code>safe-ag tui</code>
  </a>
</div>

## Defaults that matter

<div class="landing-defaults">
  <div class="landing-default">
    <strong>SSH agent</strong>
    <span>off until <code>--ssh</code></span>
  </div>
  <div class="landing-default">
    <strong>Shared auth</strong>
    <span>off until reuse flags</span>
  </div>
  <div class="landing-default">
    <strong>Container rootfs</strong>
    <span>read-only by default</span>
  </div>
  <div class="landing-default">
    <strong>Linux caps</strong>
    <span><code>cap-drop ALL</code></span>
  </div>
  <div class="landing-default">
    <strong>Network</strong>
    <span>dedicated managed bridge</span>
  </div>
  <div class="landing-default">
    <strong>Docker access</strong>
    <span>off until explicitly requested</span>
  </div>
</div>

## Common flows

=== "Single agent"

    ```bash
    safe-ag spawn claude \
      --ssh \
      --repo git@github.com:org/repo.git \
      --prompt "Fix the failing CI tests"

    safe-ag peek --latest
    safe-ag diff --latest
    safe-ag review --latest
    ```

=== "Parallel review"

    ```bash
    safe-ag fleet examples/fleet-review-and-fix.yaml
    safe-ag tui
    ```

=== "Staged pipeline"

    ```bash
    safe-ag pipeline examples/pipeline-consolidate-and-fix.yaml
    ```

## Read in this order

1. [Quickstart](quickstart.md): install, setup, first successful run
2. [Command Map](usage.md): shortest path to the right command
3. [Workflow](guide/workflow.md): diff, retry, review, PR flow
4. [Fleet and Pipelines](guide/fleet.md): orchestration and manifests
5. [Architecture](architecture.md): how the boundary layers fit together
6. [Security](security.md): defaults, wideners, and threat model
