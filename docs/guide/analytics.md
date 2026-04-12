# Analytics

Track costs and audit agent operations.

## Summary — one-screen overview

```bash
safe-ag summary api-refactor
safe-ag summary --latest
```

Prints a compact overview of a single agent:

- Agent type, container name, status
- Repo URL and active branch
- Elapsed time since spawn
- Activity (Working / Idle / Stopped)
- Cost estimate (same as `safe-ag cost`)
- Last agent message (from session JSONL)
- List of changed files (`git diff --name-only`)

Useful as a quick pre-PR sanity check or to get context before reattaching.

## Cost estimation

```bash
safe-ag cost api-refactor
safe-ag cost --latest
```

Parses session JSONL files inside the container for token usage data. Outputs:

- Total tokens (input + output)
- Per-model breakdown
- Estimated cost based on approximate rates

### Pricing used

| Model | Input (per 1M) | Output (per 1M) |
|-------|----------------|-----------------|
| claude-opus-4-6 | $15.00 | $75.00 |
| claude-sonnet-4-6 | $3.00 | $15.00 |
| claude-haiku-4-5 | $0.80 | $4.00 |
| default | $3.00 | $15.00 |

## Audit log

```bash
safe-ag audit               # last 50 entries
safe-ag audit --lines 100   # more history
```

Every `safe-ag spawn`, `safe-ag stop`, and `safe-ag attach` is recorded in an append-only JSONL file at `~/.config/safe-agentic/audit.jsonl`.

### Log format

```json
{"timestamp":"2026-04-09T14:01:23Z","action":"spawn","container":"safe-ag spawn claude --repo-api-refactor","details":"type=claude ssh=true auth=shared"}
{"timestamp":"2026-04-09T16:30:45Z","action":"stop","container":"safe-ag spawn claude --repo-api-refactor","details":""}
```

The audit log is never modified or truncated — it's append-only by design.

## Examples

```bash
# Check cost of a long-running agent
safe-ag cost my-agent
#   Total tokens: 1,234,567
#     Input:  987,654
#     Output: 246,913
#
#   claude-sonnet-4-6: 1,234,567 tokens (~$6.67)
#
#   Estimated total: ~$6.67

# View recent operations
safe-ag audit --lines 10
#   2026-04-09T14:01  spawn       safe-ag spawn claude --repo-api-refactor       type=claude ssh=true auth=shared
#   2026-04-09T14:30  attach      safe-ag spawn claude --repo-api-refactor
#   2026-04-09T16:45  stop        safe-ag spawn claude --repo-api-refactor

# Full audit history export
safe-ag audit --lines 1000 > audit-export.jsonl
```
