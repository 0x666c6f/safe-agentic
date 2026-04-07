#!/usr/bin/env bash
# Verify all bash scripts parse without syntax errors.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
errors=0

for script in \
  bin/agent \
  bin/agent-alias \
  bin/agent-lib.sh \
  bin/docker-runtime.sh \
  bin/agent-claude \
  bin/agent-codex \
  bin/repo-url.sh \
  entrypoint.sh \
  vm/setup.sh \
  config/bashrc \
; do
  path="$REPO_DIR/$script"
  if [ ! -f "$path" ]; then
    echo "MISSING: $script" >&2
    ((errors++))
    continue
  fi
  if ! bash -n "$path" 2>&1; then
    echo "SYNTAX ERROR: $script" >&2
    ((errors++))
  fi
done

[ "$errors" -eq 0 ] && echo "syntax check passed" || exit 1
