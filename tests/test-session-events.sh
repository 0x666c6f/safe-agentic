#!/usr/bin/env bash
# Unit tests for write_session_event() in agent-session.sh.
set -euo pipefail

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

# Replicate write_session_event from agent-session.sh so we can test it
# in isolation without launching an agent or needing container dependencies.
EVENTS_TMP_DIR=$(mktemp -d)
SESSION_EVENTS_DIR="$EVENTS_TMP_DIR"
SESSION_EVENTS_FILE="$SESSION_EVENTS_DIR/session-events.jsonl"
trap 'rm -rf "$EVENTS_TMP_DIR"' EXIT

write_session_event() {
  local event_type="$1" data="${2:-"{}"}"
  local ts
  ts=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
  mkdir -p "$SESSION_EVENTS_DIR"
  printf '{"ts":"%s","type":"%s","data":%s}\n' "$ts" "$event_type" "$data" >> "$SESSION_EVENTS_FILE"
}

# =============================================================================
# Test 1: session.start event has correct type field
# =============================================================================
write_session_event "session.start" '{"agent":"claude","repos":"git@github.com:org/repo.git"}'

line1=$(head -1 "$SESSION_EVENTS_FILE")
assert_contains "session.start has correct type" '"type":"session.start"' "$line1"

# =============================================================================
# Test 2: session.start event has agent data
# =============================================================================
assert_contains "session.start has agent field"  '"agent":"claude"'  "$line1"
assert_contains "session.start has repos field"   '"repos":"git@github.com:org/repo.git"' "$line1"

# Verify timestamp format (ISO 8601 UTC)
ts=$(echo "$line1" | grep -o '"ts":"[^"]*"' | cut -d'"' -f4)
if echo "$ts" | grep -qE '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$'; then
  ((++pass))
else
  echo "FAIL: session.start timestamp format invalid: $ts" >&2
  ((++fail))
fi

# =============================================================================
# Test 3: Multiple events append (line count check)
# =============================================================================
write_session_event "session.end" '{"exit_code":0}'
write_session_event "session.start" '{"agent":"codex","repos":""}'

line_count=$(wc -l < "$SESSION_EVENTS_FILE" | tr -d ' ')
assert_eq "3 events appended" "3" "$line_count"

# =============================================================================
# Test 4: session.end event has exit_code
# =============================================================================
line2=$(sed -n '2p' "$SESSION_EVENTS_FILE")
assert_contains "session.end has correct type"  '"type":"session.end"' "$line2"
assert_contains "session.end has exit_code"     '"exit_code":0'        "$line2"

# Test non-zero exit code
write_session_event "session.end" '{"exit_code":1}'
line4=$(sed -n '4p' "$SESSION_EVENTS_FILE")
assert_contains "session.end exit_code=1" '"exit_code":1' "$line4"

# =============================================================================
# Test 5: JSONL format is valid (parseable by python3 json.loads)
# =============================================================================
all_valid=true
while IFS= read -r json_line; do
  if ! python3 -c "import json, sys; json.loads(sys.stdin.read())" <<< "$json_line" 2>/dev/null; then
    all_valid=false
    echo "FAIL: invalid JSON line: $json_line" >&2
    break
  fi
done < "$SESSION_EVENTS_FILE"

if $all_valid; then
  ((++pass))
else
  ((++fail))
fi

# =============================================================================
# Test 6: Default data is empty JSON object when omitted
# =============================================================================
write_session_event "session.heartbeat"
line5=$(sed -n '5p' "$SESSION_EVENTS_FILE")
assert_contains "default data is {}" '"data":{}' "$line5"

# =============================================================================
# Test 7: Events file is created in nested directory
# =============================================================================
NESTED_EVENTS_DIR=$(mktemp -d)/sub/nested
SESSION_EVENTS_DIR="$NESTED_EVENTS_DIR"
SESSION_EVENTS_FILE="$SESSION_EVENTS_DIR/session-events.jsonl"

write_session_event "session.start" '{"agent":"shell","repos":""}'

assert_eq "nested events file created" "true" "$([ -f "$SESSION_EVENTS_FILE" ] && echo true || echo false)"
rm -rf "$(dirname "$(dirname "$NESTED_EVENTS_DIR")")"

# Summary
echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
