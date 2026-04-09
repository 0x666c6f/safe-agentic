#!/usr/bin/env bash
# Tests for the `agent diff` command.
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
      echo "${TEST_CONTAINER_STATE:-running}"; exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ] && [[ "${4:-}" == *latest* ]]; then
      echo "agent-claude-my-task"; exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "ps" ] && [ "${3:-}" = "-a" ] && [ "${4:-}" = "--latest" ]; then
      echo "agent-claude-my-task"; exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "ps" ] && [[ "$*" == *"--latest"* ]]; then
      echo "agent-claude-my-task"; exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "exec" ]; then
      # Print simulated git diff output (empty when TEST_EMPTY_DIFF is set)
      if [ "${TEST_EMPTY_DIFF:-}" = "1" ]; then
        exit 0
      fi
      if [ "${TEST_STAT_DIFF:-}" = "1" ]; then
        echo " foo.py | 2 +-"
        echo " 1 file changed, 1 insertion(+), 1 deletion(-)"
        exit 0
      fi
      echo "diff --git a/foo.py b/foo.py"
      echo "index 1234567..abcdefg 100644"
      echo "--- a/foo.py"
      echo "+++ b/foo.py"
      echo "@@ -1 +1 @@"
      echo "-old line"
      echo "+new line"
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
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" \
     TEST_CONTAINER_STATE="${TEST_CONTAINER_STATE:-running}" \
     TEST_EMPTY_DIFF="${TEST_EMPTY_DIFF:-}" \
     TEST_STAT_DIFF="${TEST_STAT_DIFF:-}" \
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
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" \
     TEST_CONTAINER_STATE="${TEST_CONTAINER_STATE:-running}" \
     TEST_EMPTY_DIFF="${TEST_EMPTY_DIFF:-}" \
     TEST_STAT_DIFF="${TEST_STAT_DIFF:-}" \
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
run_ok   "diff help flag"  bash "$REPO_DIR/bin/agent" diff --help
assert_output_contains "Usage: agent diff" "diff help shows usage"
assert_output_contains "--stat" "diff help shows --stat flag"
assert_output_contains "--latest" "diff help shows --latest flag"

run_ok   "help diff topic"  bash "$REPO_DIR/bin/agent" help diff
assert_output_contains "Usage: agent diff" "help diff topic shows usage"

run_fails "diff no args"  bash "$REPO_DIR/bin/agent" diff
assert_output_contains "agent help diff" "diff no-args shows usage pointer"

# =============================================================================
# diff runs git diff in container
# =============================================================================
: >"$ORB_LOG"
run_ok "diff by name" bash "$REPO_DIR/bin/agent" diff agent-claude-my-task
assert_output_contains "diff --git" "diff by name shows git diff output"
assert_log_contains "docker exec" "diff by name runs docker exec"
assert_log_contains "git diff" "diff by name runs git diff"

# =============================================================================
# --stat flag runs git diff --stat
# =============================================================================
: >"$ORB_LOG"
run_ok "diff --stat" bash "$REPO_DIR/bin/agent" diff agent-claude-my-task --stat
assert_log_contains "git diff --stat" "diff --stat uses --stat flag"

# =============================================================================
# --latest resolves to newest container
# =============================================================================
: >"$ORB_LOG"
run_ok "diff --latest" bash "$REPO_DIR/bin/agent" diff --latest
assert_log_contains "docker exec" "diff --latest runs docker exec"

# =============================================================================
# -h is equivalent to --help
# =============================================================================
run_ok   "diff -h flag"  bash "$REPO_DIR/bin/agent" diff -h
assert_output_contains "Usage: agent diff" "diff -h shows usage"

# =============================================================================
# Unknown flag rejected (treated as invalid container name)
# =============================================================================
run_fails "diff unknown flag" bash "$REPO_DIR/bin/agent" diff --bogus
assert_output_contains "invalid characters" "diff unknown flag shows invalid chars error"

# =============================================================================
# Multiple positional args rejected
# =============================================================================
run_fails "diff multiple args" bash "$REPO_DIR/bin/agent" diff name1 name2
assert_output_contains "agent help diff" "diff multiple args shows usage pointer"

# =============================================================================
# --latest and --stat combined
# =============================================================================
: >"$ORB_LOG"
TEST_STAT_DIFF=1
run_ok "diff --latest --stat" bash "$REPO_DIR/bin/agent" diff --latest --stat
assert_log_contains "git diff --stat" "diff --latest --stat uses --stat flag"
TEST_STAT_DIFF=

# =============================================================================
# --stat output contains stat-style content (not full diff)
# =============================================================================
: >"$ORB_LOG"
TEST_STAT_DIFF=1
run_ok "diff --stat output" bash "$REPO_DIR/bin/agent" diff agent-claude-my-task --stat
assert_output_contains "file changed" "diff --stat output shows stat summary"
TEST_STAT_DIFF=

# =============================================================================
# Container not running → error
# =============================================================================
: >"$ORB_LOG"
TEST_CONTAINER_STATE=exited
run_fails "diff container not running" bash "$REPO_DIR/bin/agent" diff agent-claude-my-task
assert_output_contains "not running" "diff container not running shows error"
TEST_CONTAINER_STATE=running

# =============================================================================
# Empty diff → command succeeds (zero exit), no output required
# =============================================================================
: >"$ORB_LOG"
TEST_EMPTY_DIFF=1
run_ok "diff empty output succeeds" bash "$REPO_DIR/bin/agent" diff agent-claude-my-task
TEST_EMPTY_DIFF=

# =============================================================================
# diff listed in general help
# =============================================================================
run_ok "diff in general help" bash "$REPO_DIR/bin/agent" help
assert_output_contains "agent diff" "diff listed in general help"

echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
