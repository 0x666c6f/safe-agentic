#!/usr/bin/env bash
# Tests for the `agent checkpoint` command.
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

cat >"$FAKE_BIN/orb" <<'ORBEOF'
#!/usr/bin/env bash
set -euo pipefail
log_file="${TEST_ORB_LOG:?}"
verify_state="${TEST_VERIFY_STATE:?}"
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
      echo "agent-claude-test-1234"; exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "ps" ] && [ "${3:-}" = "-a" ] && [ "${4:-}" = "--latest" ]; then
      echo "agent-claude-test-1234"; exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "exec" ]; then
      # Determine what command is being run
      shift 2  # remove "docker exec"
      name="$1"; shift  # container name
      # Join remaining args to inspect
      full_cmd="$*"
      if echo "$full_cmd" | grep -q "git stash create"; then
        echo "abc123def456"
        exit 0
      fi
      if echo "$full_cmd" | grep -q "update-ref"; then
        exit 0
      fi
      if echo "$full_cmd" | grep -q "for-each-ref"; then
        echo "abc123d 1712345678 before-refactor"
        echo "def456a 1712432078 checkpoint-1712432078"
        exit 0
      fi
      if echo "$full_cmd" | grep -q "git stash apply"; then
        exit 0
      fi
      if echo "$full_cmd" | grep -q "git checkout"; then
        exit 0
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
run_ok   "checkpoint help flag"  bash "$REPO_DIR/bin/agent" checkpoint --help
assert_output_contains "Usage: agent checkpoint" "checkpoint help shows usage"
assert_output_contains "create" "checkpoint help shows create subcommand"
assert_output_contains "list" "checkpoint help shows list subcommand"
assert_output_contains "revert" "checkpoint help shows revert subcommand"

run_ok   "help checkpoint topic"  bash "$REPO_DIR/bin/agent" help checkpoint
assert_output_contains "Usage: agent checkpoint" "help checkpoint topic shows usage"

run_fails "checkpoint no args"  bash "$REPO_DIR/bin/agent" checkpoint
assert_output_contains "agent help checkpoint" "checkpoint no-args shows usage pointer"

run_fails "checkpoint unknown subcmd"  bash "$REPO_DIR/bin/agent" checkpoint frobnicate
assert_output_contains "agent help checkpoint" "checkpoint unknown subcmd shows usage pointer"

# =============================================================================
# checkpoint create
# =============================================================================
: >"$ORB_LOG"
run_ok "checkpoint create by name" bash "$REPO_DIR/bin/agent" checkpoint create agent-claude-test-1234 before-refactor
assert_output_contains "before-refactor" "checkpoint create shows label in confirmation"
assert_log_contains "git stash create" "checkpoint create runs git stash create"
assert_log_contains "update-ref" "checkpoint create runs git update-ref"

: >"$ORB_LOG"
run_ok "checkpoint create --latest" bash "$REPO_DIR/bin/agent" checkpoint create --latest my-snap
assert_log_contains "git stash create" "checkpoint create --latest runs git stash create"
assert_log_contains "update-ref" "checkpoint create --latest runs git update-ref"

# =============================================================================
# checkpoint list
# =============================================================================
: >"$ORB_LOG"
run_ok "checkpoint list by name" bash "$REPO_DIR/bin/agent" checkpoint list agent-claude-test-1234
assert_output_contains "before-refactor" "checkpoint list shows checkpoint labels"
assert_log_contains "for-each-ref" "checkpoint list runs git for-each-ref"

: >"$ORB_LOG"
run_ok "checkpoint list --latest" bash "$REPO_DIR/bin/agent" checkpoint list --latest
assert_log_contains "for-each-ref" "checkpoint list --latest runs git for-each-ref"

# =============================================================================
# checkpoint revert
# =============================================================================
: >"$ORB_LOG"
run_ok "checkpoint revert by name" bash "$REPO_DIR/bin/agent" checkpoint revert agent-claude-test-1234 before-refactor
assert_log_contains "git stash apply" "checkpoint revert runs git stash apply"
assert_log_contains "git checkout" "checkpoint revert runs git checkout"

run_fails "checkpoint revert no ref" bash "$REPO_DIR/bin/agent" checkpoint revert agent-claude-test-1234
assert_output_contains "agent help checkpoint" "checkpoint revert no ref shows usage pointer"

# =============================================================================
# checkpoint revert --latest
# =============================================================================
: >"$ORB_LOG"
run_ok "checkpoint revert --latest" bash "$REPO_DIR/bin/agent" checkpoint revert --latest before-refactor
assert_log_contains "git stash apply" "checkpoint revert --latest runs git stash apply"

# =============================================================================
# -h flag on each subcommand
# =============================================================================
run_ok "checkpoint create -h" bash "$REPO_DIR/bin/agent" checkpoint create -h
assert_output_contains "Usage: agent checkpoint" "checkpoint create -h shows usage"

run_ok "checkpoint list -h" bash "$REPO_DIR/bin/agent" checkpoint list -h
assert_output_contains "Usage: agent checkpoint" "checkpoint list -h shows usage"

run_ok "checkpoint revert -h" bash "$REPO_DIR/bin/agent" checkpoint revert -h
assert_output_contains "Usage: agent checkpoint" "checkpoint revert -h shows usage"

# =============================================================================
# create with custom label — label appears in confirmation output
# =============================================================================
: >"$ORB_LOG"
run_ok "checkpoint create with explicit label" bash "$REPO_DIR/bin/agent" checkpoint create agent-claude-test-1234 my-custom-label
assert_output_contains "my-custom-label" "checkpoint create with explicit label shows label in output"
assert_log_contains "git stash create" "checkpoint create with explicit label runs git stash create"

# =============================================================================
# create --latest + label
# =============================================================================
: >"$ORB_LOG"
run_ok "checkpoint create --latest with label" bash "$REPO_DIR/bin/agent" checkpoint create --latest special-snap
assert_log_contains "git stash create" "checkpoint create --latest with label runs git stash create"
assert_log_contains "update-ref" "checkpoint create --latest with label runs git update-ref"

# =============================================================================
# unknown subcommand rejected
# =============================================================================
run_fails "checkpoint bogus subcommand" bash "$REPO_DIR/bin/agent" checkpoint bogus
assert_output_contains "agent help checkpoint" "checkpoint bogus shows usage pointer"

# =============================================================================
# checkpoint listed in general help
# =============================================================================
run_ok "general help shows checkpoint" bash "$REPO_DIR/bin/agent" help
assert_output_contains "checkpoint" "general help lists checkpoint command"

# =============================================================================
# create when no changes → "No changes" message
# (replace fake orb with one that returns empty from git stash create)
# =============================================================================
cat >"$FAKE_BIN/orb" <<'ORBEOF2'
#!/usr/bin/env bash
set -euo pipefail
log_file="${TEST_ORB_LOG:?}"
verify_state="${TEST_VERIFY_STATE:?}"
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
      echo "agent-claude-test-1234"; exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "ps" ] && [ "${3:-}" = "-a" ] && [ "${4:-}" = "--latest" ]; then
      echo "agent-claude-test-1234"; exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "exec" ]; then
      shift 2  # remove "docker exec"
      name="$1"; shift  # container name
      full_cmd="$*"
      if echo "$full_cmd" | grep -q "git stash create"; then
        # Return empty — no changes
        echo ""
        exit 0
      fi
      exit 0
    fi
    exit 0 ;;
  push|start|stop|create|ssh) ;;
  *) echo "unexpected orb command: $cmd" >&2; exit 1 ;;
esac
ORBEOF2
chmod +x "$FAKE_BIN/orb"

: >"$ORB_LOG"
run_ok "checkpoint create no changes" bash "$REPO_DIR/bin/agent" checkpoint create agent-claude-test-1234 empty-snap
assert_output_contains "No changes" "checkpoint create with no changes shows no-changes message"

# =============================================================================
# container not running → error
# =============================================================================
cat >"$FAKE_BIN/orb" <<'ORBEOF3'
#!/usr/bin/env bash
set -euo pipefail
log_file="${TEST_ORB_LOG:?}"
verify_state="${TEST_VERIFY_STATE:?}"
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
ORBEOF3
chmod +x "$FAKE_BIN/orb"

: >"$ORB_LOG"
run_fails "checkpoint create container not running" bash "$REPO_DIR/bin/agent" checkpoint create agent-claude-test-1234 snap
assert_output_contains "not running" "checkpoint create container not running shows error"

echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
