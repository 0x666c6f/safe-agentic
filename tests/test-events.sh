#!/usr/bin/env bash
# Unit tests for emit_event() and dispatch_event() in agent-lib.sh.
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

# Point audit log at a temp file (avoid polluting real log)
AUDIT_TMP=$(mktemp)
export SAFE_AGENTIC_AUDIT_LOG="$AUDIT_TMP"

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
# Test 1: emit_event writes valid JSONL with ts, event, data fields
# =============================================================================
EVENT_FILE=$(mktemp)
rm -f "$EVENT_FILE"

emit_event "$EVENT_FILE" "agent.spawned" '{"name":"test-agent","type":"claude"}'

assert_eq "event file created" "true" "$([ -f "$EVENT_FILE" ] && echo true || echo false)"

line1=$(head -1 "$EVENT_FILE")
assert_contains "line1 has ts field"    '"ts":'    "$line1"
assert_contains "line1 has event field" '"event":"agent.spawned"' "$line1"
assert_contains "line1 has data field"  '"data":{"name":"test-agent","type":"claude"}' "$line1"

# Verify timestamp format (ISO 8601 UTC)
ts=$(echo "$line1" | grep -o '"ts":"[^"]*"' | cut -d'"' -f4)
if echo "$ts" | grep -qE '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$'; then
  ((++pass))
else
  echo "FAIL: timestamp format invalid: $ts" >&2
  ((++fail))
fi

# =============================================================================
# Test 2: Multiple events append (line count check)
# =============================================================================
emit_event "$EVENT_FILE" "agent.completed" '{"name":"test-agent","exit_code":0}'
emit_event "$EVENT_FILE" "agent.failed"    '{"name":"test-agent","exit_code":1}'

line_count=$(wc -l < "$EVENT_FILE" | tr -d ' ')
assert_eq "3 lines written" "3" "$line_count"

line2=$(sed -n '2p' "$EVENT_FILE")
line3=$(sed -n '3p' "$EVENT_FILE")
assert_contains "line2 has agent.completed" '"event":"agent.completed"' "$line2"
assert_contains "line3 has agent.failed"    '"event":"agent.failed"'    "$line3"

# =============================================================================
# Test 3: dispatch_event with "command" type executes the command
# =============================================================================
CMD_OUTPUT_FILE=$(mktemp)
rm -f "$CMD_OUTPUT_FILE"

dispatch_event "command" "echo dispatched > $CMD_OUTPUT_FILE" "agent.spawned" '{"name":"test"}'

assert_eq "command dispatch created file" "true" "$([ -f "$CMD_OUTPUT_FILE" ] && echo true || echo false)"
cmd_content=$(cat "$CMD_OUTPUT_FILE" 2>/dev/null || echo "")
assert_contains "command dispatch wrote output" "dispatched" "$cmd_content"
rm -f "$CMD_OUTPUT_FILE"

# =============================================================================
# Test 4: dispatch_event with "file" type writes to a file
# =============================================================================
FILE_SINK=$(mktemp)
rm -f "$FILE_SINK"

dispatch_event "file" "$FILE_SINK" "agent.completed" '{"name":"file-test","exit_code":0}'

assert_eq "file sink created" "true" "$([ -f "$FILE_SINK" ] && echo true || echo false)"

sink_line=$(head -1 "$FILE_SINK")
assert_contains "file sink has event" '"event":"agent.completed"' "$sink_line"
assert_contains "file sink has data"  '"data":{"name":"file-test","exit_code":0}' "$sink_line"
rm -f "$FILE_SINK"

# =============================================================================
# Test 5: emit_event creates parent directories
# =============================================================================
NESTED_DIR=$(mktemp -d)/sub/dir
NESTED_FILE="$NESTED_DIR/events.jsonl"

emit_event "$NESTED_FILE" "agent.spawned" '{"name":"nested-test"}'

assert_eq "nested event file created" "true" "$([ -f "$NESTED_FILE" ] && echo true || echo false)"
rm -rf "$(dirname "$(dirname "$NESTED_DIR")")"

# =============================================================================
# Test 6: dispatch_event with unknown sink type does not error
# =============================================================================
dispatch_event "unknown_sink" "/dev/null" "agent.spawned" '{"name":"noop"}' 2>/dev/null
assert_eq "unknown sink no error" "0" "$?"

# Cleanup
rm -f "$EVENT_FILE" "$AUDIT_TMP"

# Summary
echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
