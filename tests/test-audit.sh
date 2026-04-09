#!/usr/bin/env bash
# Unit tests for audit_log() in agent-lib.sh.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Stubs expected by agent-lib.sh
IMAGE_NAME="safe-agentic"
die()     { exit 1; }
info()    { :; }
warn()    { :; }
vm_exec() { return 1; }
export -f die info warn vm_exec
export IMAGE_NAME

# Point audit log at a temp file
AUDIT_TMP=$(mktemp)
export SAFE_AGENTIC_AUDIT_LOG="$AUDIT_TMP"
trap 'rm -f "$AUDIT_TMP"' EXIT

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

# Write 3 entries
audit_log "spawn"  "agent-claude-foo" "type=claude ssh=true auth=ephemeral"
audit_log "attach" "agent-claude-foo" ""
audit_log "stop"   "agent-claude-foo" ""

# Count lines
line_count=$(wc -l < "$AUDIT_TMP" | tr -d ' ')
assert_eq "3 lines written" "3" "$line_count"

# Each line must be valid JSON with expected keys
line1=$(sed -n '1p' "$AUDIT_TMP")
line2=$(sed -n '2p' "$AUDIT_TMP")
line3=$(sed -n '3p' "$AUDIT_TMP")

assert_contains "line1 has action=spawn"  '"action":"spawn"'   "$line1"
assert_contains "line1 has container"     '"container":"agent-claude-foo"' "$line1"
assert_contains "line1 has timestamp key" '"timestamp"'        "$line1"
assert_contains "line1 has details"       '"details":"type=claude ssh=true auth=ephemeral"' "$line1"

assert_contains "line2 has action=attach" '"action":"attach"'  "$line2"
assert_contains "line3 has action=stop"   '"action":"stop"'    "$line3"

# Timestamp must look like an ISO-8601 UTC datetime
ts=$(python3 -c "import sys, json; print(json.loads(sys.stdin.read()).get('timestamp',''))" <<< "$line1")
if echo "$ts" | grep -qE '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$'; then
  ((++pass))
else
  echo "FAIL: timestamp format invalid: $ts" >&2
  ((++fail))
fi

# =============================================================================
# Multiple audit_log calls produce correct line count
# Use a fresh file by overriding AUDIT_LOG_FILE directly (it's set at source time
# from SAFE_AGENTIC_AUDIT_LOG, so we override the variable directly in this shell).
# =============================================================================
AUDIT_TMP2=$(mktemp)
AUDIT_LOG_FILE="$AUDIT_TMP2"

audit_log "spawn"   "agent-claude-bar" "type=claude ssh=false"
audit_log "stop"    "agent-claude-bar" ""
audit_log "attach"  "agent-claude-bar" ""
audit_log "cleanup" ""                 ""

line_count2=$(wc -l < "$AUDIT_TMP2" | tr -d ' ')
assert_eq "4 lines written for second batch" "4" "$line_count2"

# =============================================================================
# Each field is present: action, container, details
# =============================================================================
spawn_line=$(sed -n '1p' "$AUDIT_TMP2")
assert_contains "spawn action field"     '"action":"spawn"'              "$spawn_line"
assert_contains "spawn container field"  '"container":"agent-claude-bar"' "$spawn_line"
assert_contains "spawn details field"    '"details":"type=claude ssh=false"' "$spawn_line"
assert_contains "spawn timestamp field"  '"timestamp"'                   "$spawn_line"

# =============================================================================
# Timestamp is ISO 8601 format
# =============================================================================
ts2=$(python3 -c "import sys, json; print(json.loads(sys.stdin.read()).get('timestamp',''))" <<< "$spawn_line")
if echo "$ts2" | grep -qE '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$'; then
  ((++pass))
else
  echo "FAIL: second batch timestamp format invalid: $ts2" >&2
  ((++fail))
fi

# =============================================================================
# Different actions recorded correctly
# =============================================================================
stop_line=$(sed -n '2p' "$AUDIT_TMP2")
attach_line=$(sed -n '3p' "$AUDIT_TMP2")
cleanup_line=$(sed -n '4p' "$AUDIT_TMP2")
assert_contains "stop action"    '"action":"stop"'    "$stop_line"
assert_contains "attach action"  '"action":"attach"'  "$attach_line"
assert_contains "cleanup action" '"action":"cleanup"' "$cleanup_line"

# =============================================================================
# Empty details produces empty string in JSON (not null/missing)
# =============================================================================
assert_contains "stop empty details"    '"details":""' "$stop_line"
assert_contains "attach empty details"  '"details":""' "$attach_line"

# =============================================================================
# Audit log file is created (exists on disk)
# =============================================================================
assert_eq "audit log file exists" "true" "$([ -f "$AUDIT_TMP2" ] && echo true || echo false)"
rm -f "$AUDIT_TMP2"

# =============================================================================
# SAFE_AGENTIC_AUDIT_LOG override writes to custom path (fresh subshell)
# =============================================================================
AUDIT_TMP3=$(mktemp)
SAFE_AGENTIC_AUDIT_LOG="$AUDIT_TMP3" \
  bash -c "source \"$REPO_DIR/bin/agent-lib.sh\"; audit_log spawn agent-overridden-test ''"
override_count=$(wc -l < "$AUDIT_TMP3" | tr -d ' ')
assert_eq "SAFE_AGENTIC_AUDIT_LOG override writes one line" "1" "$override_count"
rm -f "$AUDIT_TMP3"

# =============================================================================
# cmd_audit: --help flag works
# (audit command reads from a file — no VM needed for help)
# =============================================================================
out_help=$(bash "$REPO_DIR/bin/agent" audit --help 2>&1)
rc_help=$?
if [ "$rc_help" -eq 0 ]; then
  ((++pass))
else
  echo "FAIL: audit --help expected zero exit (rc=$rc_help)" >&2
  ((++fail))
fi
if echo "$out_help" | grep -q "Usage: agent audit"; then
  ((++pass))
else
  echo "FAIL: audit --help shows usage — got: $out_help" >&2
  ((++fail))
fi

# =============================================================================
# cmd_audit: no log file shows "No audit log yet"
# =============================================================================
NONEXISTENT="/tmp/nonexistent-audit-log-$$.jsonl"
rm -f "$NONEXISTENT"
out_no_log=$(SAFE_AGENTIC_AUDIT_LOG="$NONEXISTENT" \
  bash "$REPO_DIR/bin/agent" audit 2>&1)
if echo "$out_no_log" | grep -q "No audit log yet"; then
  ((++pass))
else
  echo "FAIL: audit no log file shows 'No audit log yet' — got: $out_no_log" >&2
  ((++fail))
fi

# =============================================================================
# cmd_audit: --lines N limits output
# =============================================================================
AUDIT_TMP4=$(mktemp)
for i in 1 2 3 4 5; do
  printf '{"timestamp":"2026-01-01T00:00:0%dZ","action":"spawn","container":"agent-test-%d","details":""}\n' "$i" "$i" >> "$AUDIT_TMP4"
done

out_lines=$(SAFE_AGENTIC_AUDIT_LOG="$AUDIT_TMP4" \
  bash "$REPO_DIR/bin/agent" audit --lines 2 2>/dev/null)
line_count_limited=$(echo "$out_lines" | grep -c "agent-test" || true)
if [ "$line_count_limited" -eq 2 ]; then
  ((++pass))
else
  echo "FAIL: audit --lines 2 shows $line_count_limited lines (expected 2)" >&2
  ((++fail))
fi
rm -f "$AUDIT_TMP4"

# Summary
echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
