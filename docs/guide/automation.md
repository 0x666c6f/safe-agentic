# Automation

Run agents unattended: background sessions with lifecycle hooks, scheduled runs, and a machine-readable state server.

## Background runs with hooks

`--background` launches detached; hooks and notifications close the loop:

```bash
berth spawn claude --background --ssh \
  --repo git@github.com:org/repo.git \
  --prompt "Fix the flaky test suite" \
  --on-complete "cd /workspace/*/ && go test ./... > /workspace/test-results.txt 2>&1" \
  --notify system,slack:https://hooks.slack.com/services/XXX
```

| Flag | Fires | Runs |
|---|---|---|
| `--on-exit` | when the session ends, any exit code | **inside the container** |
| `--on-complete` | agent exited 0 | **inside the container** |
| `--on-fail` | agent exited non-zero | **inside the container** |
| `--notify` | when the session ends | on the host |

The `--on-*` hooks execute in the agent container (workspace at `/workspace`) — use them for validation, artifacts, or git operations on the agent's work. For host-side reactions, use `--notify`: `terminal`, `system`, `slack:<webhook>`, or `command:<cmd>`, which runs on the host with `$BERTH_CONTAINER` and `$BERTH_MESSAGE` set:

```bash
berth spawn claude --background --repo https://github.com/org/repo.git \
  --prompt "Write a migration plan" \
  --notify "command:berth output \$BERTH_CONTAINER > /tmp/plan.txt"
```

Add `--max-cost N.NN` to record a USD budget on the container (advisory — visible in `summary`/`cost`).

Check on background work with `berth inbox`, `berth status --all`, or the [TUI](tui.md).

## Scheduled runs — `berth cron`

Any berth command can run on a schedule:

```bash
berth pipeline create nightly-review   # save a pipeline first (~/.berth/pipelines/)

berth cron add nightly-review "daily 06:00" \
  pipeline nightly-review --repo git@github.com:org/repo.git

berth cron add hourly-triage "every 1h" \
  run https://github.com/org/repo.git "Triage new issues"

berth cron list
berth cron run nightly-review       # trigger now
berth cron disable nightly-review   # keep but pause
berth cron remove nightly-review
```

Schedule formats: `"every 30m"` / `"every 1h"`, `"daily HH:MM"`, or a cron expression like `"0 */6 * * *"`. Note: `daily HH:MM` currently behaves as a plain 24-hour interval — a new job fires on the next daemon poll and then every 24h; it is not yet aligned to the clock time.

Jobs only fire while the scheduler daemon is running (it polls every 60s):

```bash
berth cron daemon
```

Jobs are stored in `~/.berth/cron.json`.

## Machine-readable state — `berth server`

Expose berth state over JSON-RPC 2.0 for editors, bots, or dashboards:

```bash
berth server                          # newline-delimited JSON over stdio
berth server --listen 127.0.0.1:8765 --token s3cret   # HTTP on loopback
```

`--token` (or `BERTH_SERVER_TOKEN`) protects the HTTP listener. Methods: `schema`, `ping`, `timeline`, `inbox`, `agents.list`, `agent.diff`, `agent.logs`, `actions.list`, `actions.run`.

```bash
curl -s -H "Authorization: Bearer s3cret" \
  -d '{"jsonrpc":"2.0","id":1,"method":"inbox"}' \
  http://127.0.0.1:8765/rpc | jq
```

## Putting it together

Nightly self-review that opens a PR when something needs fixing (using a pipeline you saved with `berth pipeline create review-and-fix`):

```bash
berth cron add nightly "daily 02:00" \
  pipeline review-and-fix --repo git@github.com:org/api.git --var base=main
berth cron daemon &
```

Morning routine:

```bash
berth inbox          # anything blocked or failed overnight?
berth timeline       # what actually happened
berth cost --history 24h
```

## Related

- [Spawning Agents](spawning.md) — all spawn flags
- [Fleets & Pipelines](fleet.md) — the manifests cron typically triggers
- [Configuration](configuration.md#policy-rules) — hard policy limits for unattended spawns
