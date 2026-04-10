#!/usr/bin/env bash
# Tests for agent replay command.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"
OUT_LOG="$TMP_DIR/out.log"
ERR_LOG="$TMP_DIR/err.log"

mkdir -p "$FAKE_BIN"

# Sample session events JSONL
SAMPLE_EVENTS='{"ts":"2026-04-10T14:30:00Z","type":"session.start","data":{"agent":"claude","model":"claude-opus-4-6"}}
{"ts":"2026-04-10T14:30:05Z","type":"tool.call","data":{"tool":"Read","file":"src/auth.py","tokens_in":150,"tokens_out":0}}
{"ts":"2026-04-10T14:31:00Z","type":"git.commit","data":{"sha":"abc1234def","message":"fix auth bug"}}
{"ts":"2026-04-10T14:32:00Z","type":"agent.message","data":{"content":"I have fixed the authentication bug in src/auth.py by updating the token validation logic."}}
{"ts":"2026-04-10T14:35:00Z","type":"session.end","data":{"exit_code":0}}'

cat >"$FAKE_BIN/orb" <<'FAKEORB'
#!/usr/bin/env bash
set -euo pipefail
log_file="${TEST_ORB_LOG:?}"
cmd="${1:-}"; shift || true
case "$cmd" in
  list) echo "safe-agentic" ;;
  run)
    [ "${1:-}" = "-m" ] && shift 2
    printf '%s\n' "$*" >>"$log_file"
    # docker inspect for State.Status
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ] && [[ "${4:-}" == *State.Status* ]]; then
      echo "${TEST_CONTAINER_STATE:-running}"; exit 0
    fi
    # docker inspect for agent-type label
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ] && [[ "${4:-}" == *agent-type* ]]; then
      echo "${TEST_AGENT_TYPE:-claude}"; exit 0
    fi
    # docker exec cat session-events.jsonl
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "exec" ]; then
      if [ -n "${TEST_SESSION_EVENTS:-}" ]; then
        printf '%s\n' "$TEST_SESSION_EVENTS"
      fi
      exit 0
    fi
    # docker cp for session events (stopped container)
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "cp" ]; then
      # Return data via tar format on stdout when piped
      if [ -n "${TEST_SESSION_EVENTS:-}" ]; then
        printf '%s\n' "$TEST_SESSION_EVENTS"
      fi
      exit 0
    fi
    # docker ps --latest to resolve container
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "ps" ] && [[ "$*" == *"--latest"* ]]; then
      echo "agent-claude-test-task"
      exit 0
    fi
    exit 0 ;;
  *) echo "unexpected orb command: $cmd" >&2; exit 1 ;;
esac
FAKEORB
chmod +x "$FAKE_BIN/orb"

pass=0
fail=0

run_ok() {
  local label="$1"; shift
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" \
     TEST_SESSION_EVENTS="${TEST_SESSION_EVENTS:-}" \
     TEST_CONTAINER_STATE="${TEST_CONTAINER_STATE:-running}" \
     TEST_AGENT_TYPE="${TEST_AGENT_TYPE:-claude}" \
     "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
    ((++pass))
  else
    echo "FAIL: $label: expected zero exit" >&2
    cat "$OUT_LOG" >&2
    cat "$ERR_LOG" >&2
    ((++fail))
  fi
}

run_fails() {
  local label="$1"; shift
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" \
     TEST_SESSION_EVENTS="${TEST_SESSION_EVENTS:-}" \
     TEST_CONTAINER_STATE="${TEST_CONTAINER_STATE:-running}" \
     TEST_AGENT_TYPE="${TEST_AGENT_TYPE:-claude}" \
     "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
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
    echo "  stdout: $(cat "$OUT_LOG")" >&2
    echo "  stderr: $(cat "$ERR_LOG")" >&2
    ((++fail))
  fi
}

assert_output_not_contains() {
  local needle="$1" label="$2"
  if grep -q -- "$needle" "$OUT_LOG" "$ERR_LOG" 2>/dev/null; then
    echo "FAIL: $label: '$needle' unexpectedly found in output" >&2
    echo "  stdout: $(cat "$OUT_LOG")" >&2
    echo "  stderr: $(cat "$ERR_LOG")" >&2
    ((++fail))
  else
    ((++pass))
  fi
}

# ---------------------------------------------------------------------------
# Help
# ---------------------------------------------------------------------------
run_ok "replay --help" bash "$REPO_DIR/bin/agent" replay --help
assert_output_contains "agent replay" "replay help shows command name"
assert_output_contains "tools-only" "replay help mentions --tools-only"
assert_output_contains "cost-timeline" "replay help mentions --cost-timeline"

run_ok "replay -h" bash "$REPO_DIR/bin/agent" replay -h
assert_output_contains "agent replay" "replay -h shows command name"

run_ok "help replay topic" bash "$REPO_DIR/bin/agent" help replay
assert_output_contains "agent replay" "help replay topic shows command"

# ---------------------------------------------------------------------------
# No args should fail
# ---------------------------------------------------------------------------
run_fails "replay no args" bash "$REPO_DIR/bin/agent" replay
assert_output_contains "agent help replay" "replay no-args usage pointer"

# ---------------------------------------------------------------------------
# Test 1: Replay shows "Session started" for a session.start event
# ---------------------------------------------------------------------------
: >"$ORB_LOG"
TEST_SESSION_EVENTS="$SAMPLE_EVENTS"
run_ok "replay session start" bash "$REPO_DIR/bin/agent" replay agent-claude-test-task
assert_output_contains "Session started" "replay shows Session started"
assert_output_contains "claude" "replay shows agent name"
assert_output_contains "claude-opus-4-6" "replay shows model name"
assert_output_contains "14:30:00" "replay shows timestamp"

# ---------------------------------------------------------------------------
# Test 2: Replay shows tool calls
# ---------------------------------------------------------------------------
assert_output_contains "Read" "replay shows tool name"
assert_output_contains "src/auth.py" "replay shows file path"
assert_output_contains "150 tokens" "replay shows token count"

# ---------------------------------------------------------------------------
# Test 3: Replay shows git commits
# ---------------------------------------------------------------------------
assert_output_contains "Git commit" "replay shows git commit"
assert_output_contains "abc1234" "replay shows commit sha"
assert_output_contains "fix auth bug" "replay shows commit message"

# ---------------------------------------------------------------------------
# Test 4: Replay shows agent messages
# ---------------------------------------------------------------------------
assert_output_contains "Agent:" "replay shows agent message"
assert_output_contains "authentication bug" "replay shows message content"

# ---------------------------------------------------------------------------
# Test 5: Replay shows "Session ended"
# ---------------------------------------------------------------------------
assert_output_contains "Session ended" "replay shows Session ended"
assert_output_contains "exit 0" "replay shows exit code"

# ---------------------------------------------------------------------------
# Test 6: Replay shows totals
# ---------------------------------------------------------------------------
assert_output_contains "Total:" "replay shows total line"
assert_output_contains "150" "replay shows total input tokens"

# ---------------------------------------------------------------------------
# Test 7: --tools-only filters non-tool events
# ---------------------------------------------------------------------------
: >"$ORB_LOG"
TEST_SESSION_EVENTS="$SAMPLE_EVENTS"
run_ok "replay --tools-only" bash "$REPO_DIR/bin/agent" replay agent-claude-test-task --tools-only
assert_output_contains "Read" "tools-only shows tool calls"
assert_output_not_contains "Session started" "tools-only hides session.start"
assert_output_not_contains "Session ended" "tools-only hides session.end"
assert_output_not_contains "Git commit" "tools-only hides git.commit"
assert_output_not_contains "Agent:" "tools-only hides agent.message"

# ---------------------------------------------------------------------------
# Test 8: --cost-timeline shows running cost
# ---------------------------------------------------------------------------
: >"$ORB_LOG"
TEST_SESSION_EVENTS="$SAMPLE_EVENTS"
run_ok "replay --cost-timeline" bash "$REPO_DIR/bin/agent" replay agent-claude-test-task --cost-timeline
assert_output_contains "Running cost:" "cost-timeline shows running cost"

# ---------------------------------------------------------------------------
# Test 9: No events found gives error message
# ---------------------------------------------------------------------------
: >"$ORB_LOG"
TEST_SESSION_EVENTS=""
run_fails "replay no events" bash "$REPO_DIR/bin/agent" replay agent-claude-test-task
assert_output_contains "No session events" "no events shows error message"
TEST_SESSION_EVENTS="$SAMPLE_EVENTS"

# ---------------------------------------------------------------------------
# Test 10: agent help contains "replay"
# ---------------------------------------------------------------------------
run_ok "replay in general help" bash "$REPO_DIR/bin/agent" help
assert_output_contains "replay" "replay listed in general help"

# ---------------------------------------------------------------------------
# Test 11: Unknown flag rejected
# ---------------------------------------------------------------------------
run_fails "replay unknown flag" bash "$REPO_DIR/bin/agent" replay --bogus
assert_output_contains "Unknown flag" "replay unknown flag error shown"

# ---------------------------------------------------------------------------
# Test 12: --latest resolves to newest container
# ---------------------------------------------------------------------------
: >"$ORB_LOG"
TEST_SESSION_EVENTS="$SAMPLE_EVENTS"
run_ok "replay --latest" bash "$REPO_DIR/bin/agent" replay --latest
assert_output_contains "Session started" "replay --latest shows events"

# ---------------------------------------------------------------------------
# Test 13: Stopped container can be replayed via docker cp
# ---------------------------------------------------------------------------
: >"$ORB_LOG"
TEST_CONTAINER_STATE=exited
TEST_SESSION_EVENTS="$SAMPLE_EVENTS"
run_ok "replay stopped container" bash "$REPO_DIR/bin/agent" replay agent-claude-test-task
assert_output_contains "Session started" "stopped container replay shows events"
TEST_CONTAINER_STATE=running

echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
