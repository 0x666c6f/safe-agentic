# Analytics

safe-agentic exposes lightweight operational data about each session.

## Summary

```bash
safe-ag summary --latest
safe-ag summary api-refactor
```

`summary` is the quickest "what is going on?" command. It includes:
- agent type
- repo
- status and activity
- elapsed time
- cost estimate
- last message
- changed files

## Cost

```bash
safe-ag cost --latest
safe-ag cost api-refactor
```

`cost` parses token usage from session data and estimates spend by model.

Use it for:
- long-running sessions
- cost visibility before enabling larger fleets
- rough usage reporting

## Audit log

```bash
safe-ag audit
safe-ag audit --lines 100
```

The audit log is append-only and records actions like:
- spawn
- attach
- stop
- cleanup

## Session export

```bash
safe-ag sessions --latest
safe-ag sessions api-refactor ~/tmp/sessions
```

Use this when you want the full conversation trail, not just the latest output or summary.
