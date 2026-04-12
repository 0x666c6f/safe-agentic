# Architecture Overview

See [../architecture.md](../architecture.md) for the current system diagram.

Short version:

- `safe-ag` and `safe-ag-tui` are the host entrypoints.
- OrbStack provides the Linux VM boundary.
- Docker provides the per-agent container boundary.
- `vm/setup.sh`, `entrypoint.sh`, `bin/agent-session.sh`, and `bin/repo-url.sh` are the retained shell runtime pieces.
- Legacy host-side bash wrappers are gone; orchestration lives in Go.
