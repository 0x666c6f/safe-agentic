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

## Examples

```bash
# Check cost of a long-running agent
agent cost my-agent
#   Total tokens: 1,234,567
#     Input:  987,654
#     Output: 246,913
#
#   claude-sonnet-4-6: 1,234,567 tokens (~$6.67)
#
#   Estimated total: ~$6.67

# View recent operations
agent audit --lines 10
#   2026-04-09T14:01  spawn       agent-claude-api-refactor       type=claude ssh=true auth=shared
#   2026-04-09T14:30  attach      agent-claude-api-refactor
#   2026-04-09T16:45  stop        agent-claude-api-refactor

# Full audit history export
agent audit --lines 1000 > audit-export.jsonl
```
