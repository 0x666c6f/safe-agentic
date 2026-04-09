#!/usr/bin/env bash
# Tests for the `agent todo` command.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"
ERR_LOG="$TMP_DIR/error.log"
OUT_LOG="$TMP_DIR/out.log"
VERIFY_STATE="$TMP_DIR/verify-state"
TODOS_FILE="$TMP_DIR/todos.json"
mkdir -p "$FAKE_BIN"

# Initialise todos file as empty array
echo '[]' > "$TODOS_FILE"

cat >"$FAKE_BIN/orb" <<'ORBEOF'
#!/usr/bin/env bash
set -euo pipefail
log_file="${TEST_ORB_LOG:?}"
verify_state="${TEST_VERIFY_STATE:?}"
todos_file="${TEST_TODOS_FILE:?}"
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
      # Handle -i flag (stdin pipe for python scripts)
      interactive=false
      if [ "${1:-}" = "-i" ]; then
        interactive=true
        shift
      fi
      name="$1"; shift  # container name

      if [ "${1:-}" = "mkdir" ]; then
        exit 0
      fi

      # python3 - args ... (script comes via stdin)
      if [ "${1:-}" = "python3" ] && [ "${2:-}" = "-" ]; then
        shift 2  # remove "python3 -"
        # Remaining args are passed as sys.argv; replace container todos path with local one
        local_args=()
        for arg in "$@"; do
          local_args+=("$(echo "$arg" | sed "s|/workspace/.safe-agentic/todos.json|${todos_file}|g")")
        done
        # Read stdin (the python script) and run it locally
        python3 - "${local_args[@]}"
        exit $?
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
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" TEST_TODOS_FILE="$TODOS_FILE" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
    ((++pass))
  else
    echo "FAIL: $label: expected zero exit" >&2
    cat "$ERR_LOG" >&2
    ((++fail))
  fi
}

run_fails() {
  local label="$1"; shift
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" TEST_TODOS_FILE="$TODOS_FILE" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
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
run_ok   "todo help flag"  bash "$REPO_DIR/bin/agent" todo --help
assert_output_contains "Usage: agent todo" "todo help shows usage"
assert_output_contains "add" "todo help shows add subcommand"
assert_output_contains "list" "todo help shows list subcommand"
assert_output_contains "check" "todo help shows check subcommand"
assert_output_contains "uncheck" "todo help shows uncheck subcommand"

run_ok   "help todo topic"  bash "$REPO_DIR/bin/agent" help todo
assert_output_contains "Usage: agent todo" "help todo topic shows usage"

run_fails "todo no args"  bash "$REPO_DIR/bin/agent" todo
assert_output_contains "agent help todo" "todo no-args shows usage pointer"

run_fails "todo unknown subcmd"  bash "$REPO_DIR/bin/agent" todo frobnicate
assert_output_contains "agent help todo" "todo unknown subcmd shows usage pointer"

# =============================================================================
# todo add
# =============================================================================
echo '[]' > "$TODOS_FILE"
: >"$ORB_LOG"
run_ok "todo add by name" bash "$REPO_DIR/bin/agent" todo add agent-claude-test-1234 "Fix auth tests"
assert_output_contains "Fix auth tests" "todo add shows added text in confirmation"
assert_log_contains "docker exec" "todo add runs docker exec"

# =============================================================================
# todo list
# =============================================================================
: >"$ORB_LOG"
run_ok "todo list by name" bash "$REPO_DIR/bin/agent" todo list agent-claude-test-1234
assert_output_contains "Fix auth tests" "todo list shows the added todo"
assert_output_contains "[ ]" "todo list shows unchecked marker"
assert_output_contains "complete" "todo list shows completion count"

# =============================================================================
# todo check
# =============================================================================
: >"$ORB_LOG"
run_ok "todo check by name" bash "$REPO_DIR/bin/agent" todo check agent-claude-test-1234 1
assert_output_contains "done" "todo check shows done confirmation"

# Verify it's now marked done in list
run_ok "todo list after check" bash "$REPO_DIR/bin/agent" todo list agent-claude-test-1234
assert_output_contains "[x]" "todo list shows checked marker after check"

# =============================================================================
# todo uncheck
# =============================================================================
: >"$ORB_LOG"
run_ok "todo uncheck by name" bash "$REPO_DIR/bin/agent" todo uncheck agent-claude-test-1234 1
assert_output_contains "not done" "todo uncheck shows not-done confirmation"

# Verify unchecked in list
run_ok "todo list after uncheck" bash "$REPO_DIR/bin/agent" todo list agent-claude-test-1234
assert_output_contains "[ ]" "todo list shows unchecked marker after uncheck"

# =============================================================================
# --latest flag
# =============================================================================
echo '[]' > "$TODOS_FILE"
: >"$ORB_LOG"
run_ok "todo add --latest" bash "$REPO_DIR/bin/agent" todo add --latest "Deploy to staging"
assert_log_contains "docker exec" "todo add --latest runs docker exec"

: >"$ORB_LOG"
run_ok "todo list --latest" bash "$REPO_DIR/bin/agent" todo list --latest
assert_output_contains "Deploy to staging" "todo list --latest shows the added todo"

# =============================================================================
# Error cases
# =============================================================================
run_fails "todo add no text"  bash "$REPO_DIR/bin/agent" todo add agent-claude-test-1234
assert_output_contains "agent help todo" "todo add no text shows usage pointer"

run_fails "todo check no index"  bash "$REPO_DIR/bin/agent" todo check agent-claude-test-1234
assert_output_contains "agent help todo" "todo check no index shows usage pointer"

run_fails "todo uncheck no index"  bash "$REPO_DIR/bin/agent" todo uncheck agent-claude-test-1234
assert_output_contains "agent help todo" "todo uncheck no index shows usage pointer"

echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
