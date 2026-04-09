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

# Summary
echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
