# Supply Chain

safe-agentic treats the image build as part of the security story.

## Controls in place

| Source | Control |
|---|---|
| base image | pinned by digest |
| direct downloads | checksum verification |
| AWS CLI | signature verification |
| apt packages | signed repositories |
| Codex install | lockfile-based `npm ci` |

## Important property

The build context is based on tracked files, not your whole working directory.

That reduces accidental leakage of:
- `.env` files
- local secrets
- unrelated untracked files

## What this does not solve

It does not remove trust in upstream signing roots and registries. It narrows supply-chain risk; it does not eliminate it.
