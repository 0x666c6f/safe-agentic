#!/usr/bin/env bash
# Unit tests for notification functions in agent-lib.sh and --notify flag in cmd_spawn.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

# Stubs expected by agent-lib.sh
IMAGE_NAME="safe-agentic"
die()     { echo "DIE: $*" >&2; exit 1; }
info()    { :; }
warn()    { :; }
vm_exec() { return 1; }
export -f die info warn vm_exec
export IMAGE_NAME

source "$REPO_DIR/bin/agent-lib.sh"

pass=0
fail=0

assert_eq() {
  local label="$1"
  local expected="$2"
  local actual="$3"
  if [ "$expected" = "$actual" ]; then
    ((++pass))
  else
    echo "FAIL: $label — expected '$expected', got '$actual'" >&2
    ((++fail))
  fi
}

assert_contains() {
  local label="$1"
  local needle="$2"
  local haystack="$3"
  if echo "$haystack" | grep -qF "$needle"; then
    ((++pass))
  else
    echo "FAIL: $label — '$needle' not found in output" >&2
    ((++fail))
  fi
}

# =============================================================================
# Test 1: send_notification with command: type executes and writes file
# =============================================================================
NOTIFY_OUT="$TMP_DIR/notify-test.out"
send_notification "command:echo notified > $NOTIFY_OUT" "" "test-agent" "completed"
if [ -f "$NOTIFY_OUT" ] && grep -q "notified" "$NOTIFY_OUT"; then
  ((++pass))
else
  echo "FAIL: command notification did not write expected file" >&2
  ((++fail))
fi

# =============================================================================
# Test 2: send_notification with terminal type doesn't crash
# =============================================================================
send_notification "terminal" "" "test-agent" "completed" 2>/dev/null
# If we get here without error, it passed
((++pass))

# =============================================================================
# Test 3: parse_notify_targets with 3 comma-separated targets returns 3 lines
# =============================================================================
result=$(parse_notify_targets "terminal,slack,command:my-script")
line_count=$(echo "$result" | wc -l | tr -d ' ')
assert_eq "parse_notify_targets 3 targets" "3" "$line_count"

# Verify each target is on its own line
assert_contains "parse first target" "terminal" "$result"
assert_contains "parse second target" "slack" "$result"
assert_contains "parse third target" "command:my-script" "$result"

# =============================================================================
# Test 4: parse_notify_targets with single target returns 1 line
# =============================================================================
result_single=$(parse_notify_targets "terminal")
line_count_single=$(echo "$result_single" | wc -l | tr -d ' ')
assert_eq "parse_notify_targets 1 target" "1" "$line_count_single"
assert_eq "single target value" "terminal" "$result_single"

# =============================================================================
# Test 5: --notify flag accepted by cmd_spawn (dry-run)
# =============================================================================
FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"
VERIFY_STATE="$TMP_DIR/verify-state"
mkdir -p "$FAKE_BIN"

# Fake orb
cat >"$FAKE_BIN/orb" <<'ORBEOF'
#!/usr/bin/env bash
set -euo pipefail
log_file="${TEST_ORB_LOG:?}"
verify_state="${TEST_VERIFY_STATE:?}"
cmd="${1:-}"
shift || true
case "$cmd" in
  list) echo "safe-agentic" ;;
  run)
    [ "${1:-}" = "-m" ] && shift 2
    printf '%s\n' "$*" >>"$log_file"
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-lc" ] && [[ "${3:-}" == *safe-agentic-hardening-verify* ]]; then
      : >"$verify_state"
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "image" ] && [ "${3:-}" = "inspect" ]; then
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "network" ]; then
      case "${3:-}" in
        inspect) exit 1 ;;
        create) exit 0 ;;
        rm) exit 0 ;;
      esac
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "volume" ]; then
      exit 1
    fi
    exit 0
    ;;
  push|start|stop|create|ssh) ;;
  *) ;;
esac
ORBEOF
chmod +x "$FAKE_BIN/orb"

# Fake git
cat >"$FAKE_BIN/git" <<'GITEOF'
#!/usr/bin/env bash
if [ "${1:-}" = "config" ] && [ "${2:-}" = "user.name" ]; then
  echo "Test User"; exit 0
fi
if [ "${1:-}" = "config" ] && [ "${2:-}" = "user.email" ]; then
  echo "test@example.com"; exit 0
fi
exec /usr/bin/git "$@"
GITEOF
chmod +x "$FAKE_BIN/git"

ERR_LOG="$TMP_DIR/err.log"
: >"$ORB_LOG"
: >"$VERIFY_STATE"
if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" \
  bash "$REPO_DIR/bin/agent" spawn claude --repo https://github.com/org/r.git --notify terminal --dry-run >"$ERR_LOG" 2>&1; then
  ((++pass))
else
  echo "FAIL: --notify dry-run should succeed (rc=$?)" >&2
  ((++fail))
fi

# =============================================================================
# Test 6: Docker command contains the notify label and env var
# =============================================================================
dry_output=$(cat "$ERR_LOG")
assert_contains "dry-run output has notify label" "safe-agentic.notify-b64=" "$dry_output"
assert_contains "dry-run output has notify env" "SAFE_AGENTIC_NOTIFY_B64=" "$dry_output"

# Verify the base64-encoded value decodes back to the original targets
notify_b64=$(printf '%s' "terminal" | base64 | tr -d '\n')
assert_contains "dry-run notify b64 value" "$notify_b64" "$dry_output"

# =============================================================================
# Test 7: send_notification with command target using positional arg
# =============================================================================
NOTIFY_OUT2="$TMP_DIR/notify-test2.out"
send_notification "command" "echo cmd-notified > $NOTIFY_OUT2" "test-agent2" "failed"
if [ -f "$NOTIFY_OUT2" ] && grep -q "cmd-notified" "$NOTIFY_OUT2"; then
  ((++pass))
else
  echo "FAIL: command notification with target arg did not write expected file" >&2
  ((++fail))
fi

# =============================================================================
# Test 8: send_notification with slack type and no webhook doesn't crash
# =============================================================================
send_notification "slack" "" "test-agent" "completed" 2>/dev/null
((++pass))

# =============================================================================
# Test 9: send_notification with unknown type emits warning (non-fatal)
# =============================================================================
warned=""
warn() { warned="$*"; }
send_notification "unknown-type" "" "test-agent" "completed" 2>/dev/null
assert_contains "unknown type warns" "Unknown notification type" "$warned"

# Summary
echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
