# Security Defaults

These are the defaults that shape a normal berth session.

| Area | Default |
|---|---|
| SSH agent | off |
| Claude/Codex auth reuse | off |
| GitHub auth reuse | off |
| Host Claude/Codex auth seeding | off |
| AWS credentials | off |
| Docker access | off |
| Root filesystem | read-only |
| Linux capabilities | all dropped |
| Privilege escalation | blocked |
| Network | managed per-agent bridge with private-range egress guardrails |
| Resources | 8g / 4 CPU / 512 PIDs |
| Host key trust | pinned GitHub host keys |
| Sudo | unavailable |

## Meaning

The default session is intentionally useful but narrow:
- enough for public repos and normal coding tasks
- not enough to silently inherit your broader host credentials or Docker control

## Opt-in examples

```bash
berth spawn claude --ssh --repo git@github.com:org/private.git
berth spawn claude --reuse-auth --repo https://github.com/org/repo.git
berth spawn claude --seed-auth --repo https://github.com/org/repo.git
berth spawn claude --aws my-profile --repo git@github.com:org/infra.git
berth spawn claude --docker --repo https://github.com/org/repo.git
```
