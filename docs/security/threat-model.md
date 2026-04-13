# Threat Model

This page is the short version of what safe-agentic is trying to defend and what it is not trying to defend.

## Protected well

- host filesystem access by default
- automatic credential inheritance
- broad container privileges
- shared network state between normal sessions
- accidental use of the VM Docker daemon

## Protected conditionally

- SSH-backed repo access when `--ssh` is enabled
- shared auth when `--reuse-auth` or `--reuse-gh-auth` is enabled
- AWS access when `--aws` is enabled

Those are explicit tradeoffs, not bugs.

## Not the goal

safe-agentic does not attempt to limit what the agent does inside its own workspace once you have given it a task.

If the agent is malicious or simply wrong, it can still:
- edit repo files
- delete workspace data
- generate bad code
- use the network it has been granted

The sandbox boundary is the product. Inside that boundary, the agent is intentionally powerful.
