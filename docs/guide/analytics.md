# Analytics

Track costs and audit agent operations.

## Cost estimation

```bash
agent cost api-refactor
agent cost --latest
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
agent audit               # last 50 entries
agent audit --lines 100   # more history
```

Every `agent spawn`, `agent stop`, and `agent attach` is recorded in an append-only JSONL file at `~/.config/safe-agentic/audit.jsonl`.

### Log format

```json
{"timestamp":"2026-04-09T14:01:23Z","action":"spawn","container":"agent-claude-api-refactor","details":"type=claude ssh=true auth=shared"}
{"timestamp":"2026-04-09T16:30:45Z","action":"stop","container":"agent-claude-api-refactor","details":""}
```

The audit log is never modified or truncated — it's append-only by design.
