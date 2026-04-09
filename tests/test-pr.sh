#!/usr/bin/env bash
# Tests for the `agent pr` command.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"
ERR_LOG="$TMP_DIR/error.log"
OUT_LOG="$TMP_DIR/out.log"
VERIFY_STATE="$TMP_DIR/verify-state"
mkdir -p "$FAKE_BIN"

# Fake orb that simulates a running agent with SSH label and a feature branch
cat >"$FAKE_BIN/orb" <<'ORBEOF'
#!/usr/bin/env bash
set -euo pipefail
log_file="${TEST_ORB_LOG:?}"
cmd="${1:-}"; shift || true
case "$cmd" in
  list) echo "safe-agentic" ;;
  run)
    [ "${1:-}" = "-m" ] && shift 2
    printf '%s\n' "$*" >>"$log_file"
    # docker inspect --format ...State.Status...
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ] && [[ "${4:-}" == *State.Status* ]]; then
      echo "running"; exit 0
    fi
    # docker inspect --format ...latest... (resolve container name)
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ] && [[ "${4:-}" == *latest* ]]; then
      echo "agent-claude-my-task"; exit 0
    fi
    # docker ps -a --latest (resolve latest container name)
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "ps" ] && [ "${3:-}" = "-a" ] && [ "${4:-}" = "--latest" ]; then
      echo "agent-claude-my-task"; exit 0
    fi
    # docker inspect --format ...safe-agentic.ssh...
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ] && [[ "${4:-}" == *safe-agentic.ssh* ]]; then
      echo "yes"; exit 0
    fi
    # docker exec — handle various subcommands
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "exec" ]; then
      shift 2  # remove "docker exec"
      # Skip -e KEY=VAL and -i flags
      while [ "${1:-}" = "-e" ] || [ "${1:-}" = "-i" ]; do
        [ "${1:-}" = "-e" ] && shift 2 || shift
      done
      name="${1:-}"; shift || true
      # bash -c '...todos...' — no incomplete todos
      if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-c" ] && [[ "${3:-}" == *todos* ]]; then
        exit 0
      fi
      # bash -c '...rev-parse...' — return branch name
      if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-c" ] && [[ "${3:-}" == *rev-parse* ]]; then
        echo "feature/my-branch"; exit 0
      fi
      # bash -c '...git add -A...' — stage + commit
      if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-c" ] && [[ "${3:-}" == *git\ add\ -A* ]]; then
        exit 0
      fi
      # bash -c '...git push...' — push
      if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-c" ] && [[ "${3:-}" == *git\ push* ]]; then
        exit 0
      fi
      # bash -c '...gh pr create...' — return PR URL
      if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-c" ] && [[ "${3:-}" == *gh\ pr\ create* ]]; then
        echo "https://github.com/org/repo/pull/42"; exit 0
      fi
      exit 0
    fi
    exit 0 ;;
  push|start|stop|create|ssh) ;;
  *) echo "unexpected orb command: $cmd" >&2; exit 1 ;;
esac
ORBEOF
chmod +x "$FAKE_BIN/orb"

cat >"$FAKE_BIN/git" <<'GITEOF'
#!/usr/bin/env bash
if [ "${1:-}" = "config" ] && [ "${2:-}" = "user.name" ]; then echo "X"; exit 0; fi
if [ "${1:-}" = "config" ] && [ "${2:-}" = "user.email" ]; then echo "x@x"; exit 0; fi
exec /usr/bin/git "$@"
GITEOF
chmod +x "$FAKE_BIN/git"

pass=0
fail=0

run_ok() {
  local label="$1"; shift
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
    ((++pass))
  else
    echo "FAIL: $label: expected zero exit" >&2
    cat "$ERR_LOG" >&2
    ((++fail))
  fi
}

run_fails() {
  local label="$1"; shift
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
    echo "FAIL: $label: expected non-zero exit" >&2
    ((++fail))
  else
    ((++pass))
  fi
}

assert_output_contains() {
  local needle="$1" label="$2"
  if grep -q -- "$needle" "$OUT_LOG" "$ERR_LOG" 2>/dev/null; then
    ((++pass))
  else
    echo "FAIL: $label: '$needle' not in output" >&2
    cat "$OUT_LOG" >&2
    cat "$ERR_LOG" >&2
    ((++fail))
  fi
}

assert_log_contains() {
  local needle="$1" label="$2"
  if grep -q -- "$needle" "$ORB_LOG" 2>/dev/null; then
    ((++pass))
  else
    echo "FAIL: $label: '$needle' not in orb log" >&2
    cat "$ORB_LOG" >&2
    ((++fail))
  fi
}

# =============================================================================
# Help and dispatch
# =============================================================================
run_ok   "pr help flag"  bash "$REPO_DIR/bin/agent" pr --help
assert_output_contains "Usage: agent pr" "pr help shows usage"
assert_output_contains "\-\-title" "pr help shows --title flag"
assert_output_contains "\-\-base" "pr help shows --base flag"
assert_output_contains "\-\-latest" "pr help shows --latest flag"

run_ok   "help pr topic"  bash "$REPO_DIR/bin/agent" help pr
assert_output_contains "Usage: agent pr" "help pr topic shows usage"

run_fails "pr no args"  bash "$REPO_DIR/bin/agent" pr
assert_output_contains "agent help pr" "pr no-args shows usage pointer"

# =============================================================================
# pr by name — full happy path
# =============================================================================
: >"$ORB_LOG"
run_ok "pr by name" bash "$REPO_DIR/bin/agent" pr agent-claude-my-task
assert_output_contains "https://github.com" "pr by name prints PR URL"
assert_log_contains "docker exec" "pr by name runs docker exec"
assert_log_contains "gh pr create" "pr by name calls gh pr create"

# =============================================================================
# --latest resolves to newest container
# =============================================================================
: >"$ORB_LOG"
run_ok "pr --latest" bash "$REPO_DIR/bin/agent" pr --latest
assert_output_contains "https://github.com" "pr --latest prints PR URL"
assert_log_contains "docker exec" "pr --latest runs docker exec"

# =============================================================================
# --title sets PR title
# =============================================================================
: >"$ORB_LOG"
run_ok "pr --title" bash "$REPO_DIR/bin/agent" pr agent-claude-my-task --title "My custom title"
assert_log_contains "gh pr create" "pr --title calls gh pr create"

# =============================================================================
# --base sets base branch
# =============================================================================
: >"$ORB_LOG"
run_ok "pr --base" bash "$REPO_DIR/bin/agent" pr agent-claude-my-task --base dev
assert_log_contains "gh pr create" "pr --base calls gh pr create"
assert_log_contains "dev" "pr --base passes base branch"

# =============================================================================
# Missing SSH label → error
# =============================================================================
cat >"$FAKE_BIN/orb" <<'ORBEOF'
#!/usr/bin/env bash
set -euo pipefail
log_file="${TEST_ORB_LOG:?}"
cmd="${1:-}"; shift || true
case "$cmd" in
  list) echo "safe-agentic" ;;
  run)
    [ "${1:-}" = "-m" ] && shift 2
    printf '%s\n' "$*" >>"$log_file"
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ] && [[ "${4:-}" == *State.Status* ]]; then
      echo "running"; exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ] && [[ "${4:-}" == *latest* ]]; then
      echo "agent-claude-my-task"; exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "ps" ] && [ "${3:-}" = "-a" ] && [ "${4:-}" = "--latest" ]; then
      echo "agent-claude-my-task"; exit 0
    fi
    # No SSH label — return empty
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ] && [[ "${4:-}" == *safe-agentic.ssh* ]]; then
      echo ""; exit 0
    fi
    exit 0 ;;
  push|start|stop|create|ssh) ;;
  *) echo "unexpected orb command: $cmd" >&2; exit 1 ;;
esac
ORBEOF
chmod +x "$FAKE_BIN/orb"

: >"$ORB_LOG"
run_fails "pr no-ssh fails" bash "$REPO_DIR/bin/agent" pr agent-claude-my-task
assert_output_contains "ssh" "pr no-ssh mentions ssh requirement"

# =============================================================================
# -h flag prints usage
# =============================================================================
run_ok "pr -h flag" bash "$REPO_DIR/bin/agent" pr -h
assert_output_contains "Usage: agent pr" "pr -h shows usage"

# =============================================================================
# pr listed in general help
# =============================================================================
run_ok "general help shows pr" bash "$REPO_DIR/bin/agent" help
assert_output_contains "pr" "general help lists pr command"

# =============================================================================
# unknown flag rejected
# =============================================================================
run_fails "pr unknown flag" bash "$REPO_DIR/bin/agent" pr agent-claude-my-task --frobnicate
assert_output_contains "agent help pr" "pr unknown flag shows usage pointer"

# =============================================================================
# name + --latest both given → rejected
# =============================================================================
run_fails "pr name and --latest rejected" bash "$REPO_DIR/bin/agent" pr agent-claude-my-task --latest
assert_output_contains "not both" "pr name+--latest shows mutual exclusion error"

# =============================================================================
# container not running → error
# =============================================================================
cat >"$FAKE_BIN/orb" <<'ORBEOF'
#!/usr/bin/env bash
set -euo pipefail
log_file="${TEST_ORB_LOG:?}"
cmd="${1:-}"; shift || true
case "$cmd" in
  list) echo "safe-agentic" ;;
  run)
    [ "${1:-}" = "-m" ] && shift 2
    printf '%s\n' "$*" >>"$log_file"
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ] && [[ "${4:-}" == *State.Status* ]]; then
      echo "exited"; exit 0
    fi
    exit 0 ;;
  push|start|stop|create|ssh) ;;
  *) echo "unexpected orb command: $cmd" >&2; exit 1 ;;
esac
ORBEOF
chmod +x "$FAKE_BIN/orb"

: >"$ORB_LOG"
run_fails "pr container not running" bash "$REPO_DIR/bin/agent" pr agent-claude-my-task
assert_output_contains "not running" "pr container not running shows error"

echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
