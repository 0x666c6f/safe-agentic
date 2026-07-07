# Analytics

berth exposes lightweight operational data about each session.

## Summary

```bash
berth summary --latest
berth summary api-refactor
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
berth cost --latest
berth cost api-refactor
```

`cost` parses token usage from session data and estimates spend by model.

Use it for:
- long-running sessions
- cost visibility before enabling larger fleets
- rough usage reporting

## Audit log

```bash
berth audit
berth audit --lines 100
```

The audit log is append-only and records actions like:
- spawn
- attach
- stop
- cleanup

## Session export

```bash
berth sessions --latest
berth sessions api-refactor ~/tmp/sessions
```

Use this when you want the full conversation trail, not just the latest output or summary.
