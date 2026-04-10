#!/usr/bin/env bash
# Tests for the `agent output` command.
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

# Minimal JSONL session with one assistant message
SAMPLE_JSONL='{"type":"user","message":{"role":"user","content":"hello"}}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Done. Fixed 3 issues in auth.py."}]}}'

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
    # State inspection
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ] && [[ "${4:-}" == *State.Status* ]]; then
      echo "${TEST_CONTAINER_STATE:-running}"; exit 0
    fi
    # agent-type label
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ] && [[ "${4:-}" == *agent-type* ]]; then
      echo "${TEST_AGENT_TYPE:-claude}"; exit 0
    fi
    # repo-display label
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ] && [[ "${4:-}" == *repo-display* ]]; then
      echo "${TEST_REPO_DISPLAY:--}"; exit 0
    fi
    # terminal mode label
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ] && [[ "${4:-}" == *safe-agentic.terminal* ]]; then
      echo "${TEST_TERMINAL_MODE:-tmux}"; exit 0
    fi
    # container creation time
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ] && [[ "${4:-}" == *\.Created* ]]; then
      echo "2024-01-01T00:00:00.000000000Z"; exit 0
    fi
    # resolve --latest
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "ps" ] && [[ "$*" == *"--latest"* ]]; then
      echo "agent-claude-my-task"; exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ] && [[ "${4:-}" == *latest* ]]; then
      echo "agent-claude-my-task"; exit 0
    fi
    # docker exec for session JSONL (python3 -c with match_script)
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "exec" ] && [[ "$*" == *python3* ]]; then
      printf '%s\n' "${TEST_SAMPLE_JSONL:-}"
      exit 0
    fi
    # docker exec for git commands
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "exec" ] && [[ "$*" == *"git diff"* ]]; then
      printf '%s\n' "${TEST_GIT_DIFF:-}"
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "exec" ] && [[ "$*" == *"git log"* ]]; then
      printf '%s\n' "${TEST_GIT_LOG:-}"
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "exec" ] && [[ "$*" == *"ls-files"* ]]; then
      printf '%s\n' "${TEST_GIT_UNTRACKED:-}"
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "logs" ]; then
      printf '%s\n' "${TEST_DOCKER_LOGS:-}"
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
  if PATH="$FAKE_BIN:$PATH" \
     TEST_ORB_LOG="$ORB_LOG" \
     TEST_VERIFY_STATE="$VERIFY_STATE" \
     TEST_CONTAINER_STATE="${TEST_CONTAINER_STATE:-running}" \
     TEST_AGENT_TYPE="${TEST_AGENT_TYPE:-claude}" \
     TEST_REPO_DISPLAY="${TEST_REPO_DISPLAY:--}" \
     TEST_TERMINAL_MODE="${TEST_TERMINAL_MODE:-tmux}" \
     TEST_SAMPLE_JSONL="${TEST_SAMPLE_JSONL-$SAMPLE_JSONL}" \
     TEST_GIT_DIFF="${TEST_GIT_DIFF:-}" \
     TEST_GIT_LOG="${TEST_GIT_LOG:-}" \
     TEST_GIT_UNTRACKED="${TEST_GIT_UNTRACKED:-}" \
     TEST_DOCKER_LOGS="${TEST_DOCKER_LOGS:-}" \
     "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
    ((++pass))
  else
    echo "FAIL: $label: expected zero exit" >&2
    cat "$ERR_LOG" >&2
    ((++fail))
  fi
}

run_fails() {
  local label="$1"; shift
  if PATH="$FAKE_BIN:$PATH" \
     TEST_ORB_LOG="$ORB_LOG" \
     TEST_VERIFY_STATE="$VERIFY_STATE" \
     TEST_CONTAINER_STATE="${TEST_CONTAINER_STATE:-running}" \
     TEST_AGENT_TYPE="${TEST_AGENT_TYPE:-claude}" \
     TEST_REPO_DISPLAY="${TEST_REPO_DISPLAY:--}" \
     TEST_TERMINAL_MODE="${TEST_TERMINAL_MODE:-tmux}" \
     TEST_SAMPLE_JSONL="${TEST_SAMPLE_JSONL-$SAMPLE_JSONL}" \
     TEST_GIT_DIFF="${TEST_GIT_DIFF:-}" \
     TEST_GIT_LOG="${TEST_GIT_LOG:-}" \
     TEST_GIT_UNTRACKED="${TEST_GIT_UNTRACKED:-}" \
     TEST_DOCKER_LOGS="${TEST_DOCKER_LOGS:-}" \
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
run_ok   "output help flag"  bash "$REPO_DIR/bin/agent" output --help
assert_output_contains "Usage: agent output" "output help shows usage"
assert_output_contains "\-\-diff" "output help shows --diff flag"
assert_output_contains "\-\-files" "output help shows --files flag"
assert_output_contains "\-\-commits" "output help shows --commits flag"
assert_output_contains "\-\-json" "output help shows --json flag"
assert_output_contains "\-\-latest" "output help shows --latest flag"

run_ok   "output -h flag"  bash "$REPO_DIR/bin/agent" output -h
assert_output_contains "Usage: agent output" "output -h shows usage"

run_ok   "help output topic"  bash "$REPO_DIR/bin/agent" help output
assert_output_contains "Usage: agent output" "help output topic shows usage"

run_fails "output no args"  bash "$REPO_DIR/bin/agent" output
assert_output_contains "agent help output" "output no-args shows usage pointer"

# =============================================================================
# Default mode: last assistant message
# =============================================================================
: >"$ORB_LOG"
run_ok "output default by name" bash "$REPO_DIR/bin/agent" output agent-claude-my-task
assert_output_contains "Done. Fixed 3 issues in auth.py." "output default shows last assistant message"

: >"$ORB_LOG"
run_ok "output default --latest" bash "$REPO_DIR/bin/agent" output --latest
assert_output_contains "Done. Fixed 3 issues in auth.py." "output --latest shows last assistant message"

# =============================================================================
# --diff mode
# =============================================================================
: >"$ORB_LOG"
TEST_GIT_DIFF="diff --git a/auth.py b/auth.py"
run_ok "output --diff" bash "$REPO_DIR/bin/agent" output agent-claude-my-task --diff
assert_output_contains "diff --git" "output --diff shows git diff"
assert_log_contains "git diff" "output --diff runs git diff"
TEST_GIT_DIFF=""

# =============================================================================
# --files mode
# =============================================================================
: >"$ORB_LOG"
TEST_GIT_DIFF="auth.py"
TEST_GIT_UNTRACKED="tests/test_new.py"
run_ok "output --files" bash "$REPO_DIR/bin/agent" output agent-claude-my-task --files
assert_output_contains "auth.py" "output --files shows changed file"
TEST_GIT_DIFF=""
TEST_GIT_UNTRACKED=""

# =============================================================================
# --commits mode
# =============================================================================
: >"$ORB_LOG"
TEST_GIT_LOG="abc1234 fix: resolve auth bug"
run_ok "output --commits" bash "$REPO_DIR/bin/agent" output agent-claude-my-task --commits
assert_output_contains "fix: resolve auth bug" "output --commits shows commit log"
assert_log_contains "git log" "output --commits runs git log"
TEST_GIT_LOG=""

# =============================================================================
# --json mode
# =============================================================================
: >"$ORB_LOG"
TEST_GIT_LOG="abc1234 fix: resolve auth bug"
run_ok "output --json" bash "$REPO_DIR/bin/agent" output agent-claude-my-task --json
assert_output_contains '"name"' "output --json has name field"
assert_output_contains '"status"' "output --json has status field"
assert_output_contains '"last_message"' "output --json has last_message field"
assert_output_contains '"commits"' "output --json has commits field"
assert_output_contains '"files_changed"' "output --json has files_changed field"
assert_output_contains '"cost_estimate"' "output --json has cost_estimate field"
assert_output_contains 'agent-claude-my-task' "output --json contains container name"
TEST_GIT_LOG=""

# =============================================================================
# No session data → warning
# =============================================================================
: >"$ORB_LOG"
TEST_SAMPLE_JSONL=""
run_ok "output no session data" bash "$REPO_DIR/bin/agent" output agent-claude-my-task
assert_output_contains "No session data" "output no-data shows warning"
TEST_SAMPLE_JSONL="$SAMPLE_JSONL"

# =============================================================================
# Background sessions without JSONL fall back to docker logs
# =============================================================================
: >"$ORB_LOG"
TEST_SAMPLE_JSONL=""
TEST_TERMINAL_MODE=none
TEST_DOCKER_LOGS='[entrypoint] First run — authenticating via device code flow...'
run_ok "output background logs fallback" bash "$REPO_DIR/bin/agent" output agent-claude-my-task
assert_output_contains "device code flow" "output background shows logs when no session data"
assert_log_contains "docker logs --tail 80 agent-claude-my-task" "output background reads docker logs"

: >"$ORB_LOG"
run_ok "output --json background logs fallback" bash "$REPO_DIR/bin/agent" output agent-claude-my-task --json
assert_output_contains 'device code flow' "output --json background stores log excerpt"
TEST_SAMPLE_JSONL="$SAMPLE_JSONL"
TEST_TERMINAL_MODE=tmux
TEST_DOCKER_LOGS=""

# =============================================================================
# Container not running → error for git-dependent modes
# =============================================================================
: >"$ORB_LOG"
TEST_CONTAINER_STATE=exited
run_fails "output --diff exited container" bash "$REPO_DIR/bin/agent" output agent-claude-my-task --diff
assert_output_contains "not running" "output --diff exited shows error"

: >"$ORB_LOG"
run_fails "output --files exited container" bash "$REPO_DIR/bin/agent" output agent-claude-my-task --files
assert_output_contains "not running" "output --files exited shows error"

: >"$ORB_LOG"
run_fails "output --commits exited container" bash "$REPO_DIR/bin/agent" output agent-claude-my-task --commits
assert_output_contains "not running" "output --commits exited shows error"
TEST_CONTAINER_STATE=running

# =============================================================================
# --latest and name are mutually exclusive
# =============================================================================
run_fails "output --latest and name" bash "$REPO_DIR/bin/agent" output agent-claude-my-task --latest
assert_output_contains "agent help output" "output --latest+name shows usage pointer"

# =============================================================================
# output listed in general help
# =============================================================================
run_ok "output in general help" bash "$REPO_DIR/bin/agent" help
assert_output_contains "agent output" "output listed in general help"

echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
