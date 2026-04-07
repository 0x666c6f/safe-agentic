#!/usr/bin/env bash
# Static checks for entrypoint.sh behavior.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENTRYPOINT="$REPO_DIR/entrypoint.sh"

pass=0
fail=0

assert_present() {
  local pattern="$1" label="$2"
  if grep -qE "$pattern" "$ENTRYPOINT"; then ((++pass)); else
    echo "FAIL: $label" >&2; ((++fail))
  fi
}

assert_absent() {
  local pattern="$1" label="$2"
  if grep -qE "$pattern" "$ENTRYPOINT"; then
    echo "FAIL: $label" >&2; ((++fail))
  else ((++pass)); fi
}

# --- Git identity uses env vars with safe fallbacks ---
assert_present 'GIT_CONFIG_GLOBAL:=/home/agent/.config/git/config' "git config path fallback"
assert_present 'mkdir -p "\$\(dirname "\$GIT_CONFIG_GLOBAL"\)"' "git config dir created"
assert_present 'GIT_AUTHOR_NAME'                         "git name from env var"
assert_present 'GIT_AUTHOR_EMAIL'                        "git email from env var"
assert_present 'Agent'                                   "git name fallback"
assert_present 'agent@localhost'                         "git email fallback"
assert_absent  'user\.name\s+"AI Agent"'                 "no stale hardcoded AI Agent name"
assert_absent  'user\.email\s+"agent@safe-agentic'       "no stale hardcoded safe-agentic email"

# --- No global HTTPS→SSH rewrite ---
assert_absent 'insteadOf'                                "no git insteadOf rewrite"

# --- Clone path validation exists and unsafe paths are rejected ---
assert_present 'repo-url\.sh'                            "repo url helper sourced"
assert_present 'repo_clone_path "\$repo_url"'            "repo clone path helper used"
assert_present 'Refusing repo URL with unsafe clone path' "unsafe clone path rejection"
assert_present 'clone_dir="/workspace/\$clone_path"'     "clone under /workspace only"

# --- Internal Docker daemon mode exists for opt-in DinD sessions ---
assert_present 'SAFE_AGENTIC_INTERNAL_DOCKERD'           "internal dockerd mode"
assert_present 'exec dockerd --group agent --host'       "internal dockerd exec"

# --- Claude uses --dangerously-skip-permissions (container IS sandbox) ---
assert_present 'exec claude --dangerously-skip-permissions'  "claude skip-permissions"

# --- Codex uses yolo mode ---
assert_present 'exec codex --yolo' "codex yolo"

# --- Codex auth uses device-auth flow (headless-compatible) ---
assert_present 'codex login --device-auth'                    "codex device-auth flow"

# --- set -euo pipefail ---
assert_present '^set -euo pipefail$'                          "strict mode"

echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
