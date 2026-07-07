# Quickstart

From zero to a reviewed change in a few commands. This assumes you finished [Installation](install.md) (`berth setup` succeeded and `berth diagnose` is green).

## The shortest path

```bash
berth run https://github.com/myorg/myrepo.git "Fix the failing CI tests"
```

`berth run` is the quick-start form: it spawns a Claude agent with smart defaults and auto-enables `--ssh` when you pass a `git@` URL. For explicit control over agent type, auth, and isolation, use [`berth spawn`](guide/spawning.md).

## 1. Spawn an agent

=== "Public repo"

    ```bash
    berth spawn claude --repo https://github.com/myorg/myrepo.git
    ```

=== "Private repo"

    ```bash
    berth spawn claude --ssh --repo git@github.com:myorg/myrepo.git
    ```

=== "With a task"

    ```bash
    berth spawn claude \
      --ssh \
      --repo git@github.com:myorg/myrepo.git \
      --prompt "Fix the failing CI tests"
    ```

The first auth flow may open a browser or print a device-code login. Agents run inside tmux in the container — `Ctrl-b d` detaches without stopping anything.

## 2. Watch it work

```bash
berth list                # all agents, running and stopped
berth status --latest     # blocked / working / done / idle / exited
berth peek --latest       # snapshot of the live terminal
berth attach --latest     # jump into the session (restarts stopped containers)
```

Need to correct course without attaching?

```bash
berth steer --latest "Keep the fix narrow and add one regression test"
```

## 3. Review what changed

```bash
berth output --latest     # the agent's last message
berth diff --latest       # git diff of the workspace
berth review --latest     # AI review pass over the changes
```

Happy with it? Push a branch and open a PR (needs `--ssh` and GitHub auth in the container):

```bash
berth pr --latest --title "fix: stabilize CI"
```

## 4. Clean up

```bash
berth stop --latest       # stop + remove this agent
berth cleanup             # remove all agents + managed networks (keeps auth)
berth cleanup --auth      # full reset including auth volumes
```

Containers persist until you stop them — you can walk away and `berth attach` later.

## Where to go next

- [Spawning Agents](guide/spawning.md) — every repo/auth/runtime option
- [Monitor & Manage](guide/managing.md) — status, logs, search, inbox, cost
- [Review & Ship](guide/workflow.md) — steer, checkpoints, review comments, PRs
- [Terminal UI](guide/tui.md) — all of the above from one dashboard
- [Security](security.md) — what the sandbox does and doesn't protect
