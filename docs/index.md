---
hide:
  - navigation
  - toc
---

<div class="landing-hero" markdown="1">

<p class="landing-eyebrow">berth</p>

# Sandbox coding agents without handing them your host

Run Claude Code and Codex inside a hardened Apple container machine, one isolated container per agent. Everything risky — SSH, shared auth, AWS, Docker — stays off until you opt in.

<div class="landing-terminal" markdown="1">

```bash
brew tap 0x666c6f/tap
brew install berth
berth setup
berth run https://github.com/org/repo.git "Fix the failing CI tests"
```

</div>

[Get Started](install.md){ .md-button .md-button--primary }
[Quickstart](quickstart.md){ .md-button }
[GitHub](https://github.com/0x666c6f/berth){ .md-button }

<div class="landing-meta">
  <span>macOS host</span>
  <span>hardened Apple container machine</span>
  <span>one container per agent</span>
</div>

</div>

<div class="landing-callout" markdown="1">

`berth` is a sandbox runner for autonomous coding agents. The container is the boundary — not an editor permission dialog.

</div>

## Pick your surface

<div class="landing-panel-grid">
  <a class="landing-panel" href="reference/cli/">
    <span class="landing-panel-kicker">CLI</span>
    <strong>Spawn, steer, review, and ship from the terminal. Scriptable end to end.</strong>
    <code>berth spawn claude --repo ...</code>
  </a>
  <a class="landing-panel" href="guide/tui/">
    <span class="landing-panel-kicker">TUI</span>
    <strong>A k9s-style dashboard: every agent, live state, keyboard-first actions.</strong>
    <code>berth tui</code>
  </a>
  <a class="landing-panel" href="guide/app/">
    <span class="landing-panel-kicker">Desktop app</span>
    <strong>Native macOS app with embedded terminals, diff review, and notifications.</strong>
    <code>make -C app bundle</code>
  </a>
  <a class="landing-panel" href="guide/fleet/">
    <span class="landing-panel-kicker">Orchestrate</span>
    <strong>Fleets for parallel fan-out, pipelines for staged work, judges to pick winners.</strong>
    <code>berth pipeline review.yaml</code>
  </a>
</div>

## Start with the page that matches the job

<div class="grid cards" markdown>

-   :material-download-outline: __Install & set up__

    Install the toolchain, create the hardened VM, build the agent image.

    [Installation](install.md)

-   :material-rocket-launch-outline: __First agent in five minutes__

    Spawn an agent, watch it work, review the diff, clean up.

    [Quickstart](quickstart.md)

-   :material-source-branch: __Daily loop__

    Spawn, monitor, steer, review, and open PRs — the full working cycle.

    [Guides](guide/spawning.md)

-   :material-shield-lock-outline: __Trust boundaries__

    What the sandbox protects, what each opt-in flag widens, and why.

    [Security](security.md)

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
    <span>read-only</span>
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
    <span>off until <code>--docker</code></span>
  </div>
</div>

Full posture and every widener: [Security](security.md).

## Common flows

=== "Single agent"

    ```bash
    berth spawn claude \
      --ssh \
      --repo git@github.com:org/repo.git \
      --prompt "Fix the failing CI tests"

    berth peek --latest
    berth diff --latest
    berth review --latest
    berth pr --latest
    ```

=== "Parallel fleet"

    ```bash
    berth fleet examples/fleet-review-and-fix.yaml
    berth tui
    ```

=== "Staged pipeline"

    ```bash
    berth pipeline examples/pipeline-consolidate-and-fix.yaml
    ```

=== "Scheduled run"

    ```bash
    berth cron add nightly-review "daily 06:00" \
      pipeline review --repo git@github.com:org/repo.git
    berth cron daemon
    ```

## Read in this order

1. [Installation](install.md) — toolchain, VM, image
2. [Quickstart](quickstart.md) — first successful run
3. [Spawning Agents](guide/spawning.md) — repos, auth, prompts, resources
4. [Review & Ship](guide/workflow.md) — diff, steer, review, PR
5. [Fleets & Pipelines](guide/fleet.md) — orchestration at scale
6. [Architecture](architecture.md) & [Security](security.md) — how the boundaries work
