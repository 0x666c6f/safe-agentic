# Security

safe-agentic is designed around one rule:

> dangerous capabilities should be explicit, not ambient

## Default posture

Without opt-in flags, a spawned agent does not get:
- your SSH agent
- shared Claude/Codex auth
- shared GitHub auth
- AWS credentials
- Docker daemon access

It does get:
- a writable workspace
- installed tooling
- internet access suitable for normal package/repo/API work

## Security reading order

- [Security Defaults](security/defaults.md)
- [Threat Surface](security/threat-surface.md)
- [Supply Chain](security/supply-chain.md)
- [Threat Model](security/threat-model.md)

## Important framing

safe-agentic does not try to constrain what the agent does inside the sandbox.

It constrains:
- what the sandbox can reach
- which credentials enter it
- how much of the host/runtime it can control
