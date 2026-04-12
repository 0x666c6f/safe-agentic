# Supply Chain Hardening

The Docker image is the foundation of every agent container. safe-agentic takes supply chain integrity seriously — every binary is verified before installation.

## Verification methods

| Source | Method | Details |
|--------|--------|---------|
| Ubuntu base image | **Digest pinning** | `FROM ubuntu:24.04@sha256:<digest>` — not just a tag |
| Direct binary downloads | **SHA256 checksum** | Each binary pinned per-architecture in the Dockerfile |
| AWS CLI | **GPG signature** | Verified against the embedded AWS public key |
| Apt packages | **Signed repositories** | Pinned GPG keys for Node.js, Terraform, kubectl, GitHub CLI, etc. |
| Claude Code | **Version pinning** | Official installer, verified by `claude --version` post-install |
| Codex CLI | **npm lockfile** | `npm ci` with `package-lock.json` for reproducible installs |

## What's NOT done

- **No `curl | bash`** — every install is verified before execution
- **No floating tags** — the base image is pinned by digest, not just `ubuntu:24.04`
- **No unverified downloads** — every binary has a checksum or signature check
- **No runtime package installation** — the image is self-contained and read-only

## Build context safety

The build context sent to Docker is constructed from `git ls-files -c` filtered by `test -e`:

1. Only files tracked by git are included
2. Only files that exist on disk are included (handles `git rm` without commit)
3. `.env`, credentials, and untracked files are never sent
4. The context is tarred and copied to the VM, then extracted in a temp directory

This prevents accidental leakage of secrets or local-only files into the image.

## Image build modes

```bash
safe-ag update              # uses Docker layer cache
safe-ag update --quick      # busts only the AI CLI layer (fast)
safe-ag update --full       # no cache, rebuilds from scratch
```

- `--quick` is useful after Claude Code or Codex releases a new version — it rebuilds only the npm/installer layer
- `--full` picks up OS-level security patches from apt
- Default cached builds are fast but may use stale layers

## Dockerfile shell safety

The Dockerfile uses:

```dockerfile
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
```

This ensures pipe failures are caught during build. Without `pipefail`, a command like `curl ... | tar ...` would succeed even if `curl` fails (tar would just see empty input).

## Trust roots

safe-agentic trusts these signing roots:

- **Ubuntu apt** — Canonical's package signing keys
- **Node.js** — NodeSource GPG key
- **Terraform** — HashiCorp GPG key
- **kubectl** — Google Cloud GPG key
- **GitHub CLI** — GitHub GPG key
- **AWS CLI** — AWS GPG public key (embedded in Dockerfile)
- **npm registry** — for Codex CLI lockfile resolution

If any of these roots are compromised, the corresponding packages could be tampered with. This is a fundamental limitation of package manager trust chains.
