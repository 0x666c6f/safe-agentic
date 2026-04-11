#!/usr/bin/env bash
# Tests for agent cost --history (cost dashboard) and cost audit logging on stop.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"
OUT_LOG="$TMP_DIR/out.log"
ERR_LOG="$TMP_DIR/err.log"
AUDIT_DIR="$TMP_DIR/config/safe-agentic"
AUDIT_FILE="$AUDIT_DIR/audit.jsonl"

mkdir -p "$FAKE_BIN" "$AUDIT_DIR"

# Fake orb binary for tests that need the VM (stop tests)
cat >"$FAKE_BIN/orb" <<'ORBEOF'
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
    # docker exec to read session JSONL
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "exec" ]; then
      printf '%s\n' "${TEST_JSONL_DATA:-}"
      exit 0
    fi
    # docker ps --latest to resolve container
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "ps" ] && [[ "$*" == *"--latest"* ]]; then
      echo "agent-claude-test-task"
      exit 0
    fi
    # docker ps -a for stop --all
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "ps" ]; then
      echo ""
      exit 0
    fi
    # docker stop / docker rm / docker network — no-ops
    exit 0 ;;
  *) echo "unexpected orb command: $cmd" >&2; exit 1 ;;
esac
ORBEOF
chmod +x "$FAKE_BIN/orb"

pass=0
fail=0

run_ok() {
  local label="$1"; shift
  if "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
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
  if "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
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

# ===========================================================================
# Test 1: --history with known audit data shows per-container costs
# ===========================================================================
now_ts=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
cat >"$AUDIT_FILE" <<EOF
{"timestamp":"$now_ts","action":"cost","container":"agent-claude-alpha","cost":12.5}
{"timestamp":"$now_ts","action":"cost","container":"agent-claude-beta","cost":3.25}
{"timestamp":"$now_ts","action":"cost","container":"agent-claude-alpha","cost":7.5}
{"timestamp":"$now_ts","action":"stop","container":"agent-claude-alpha","details":""}
EOF

run_ok "history shows costs" \
  env SAFE_AGENTIC_AUDIT_LOG="$AUDIT_FILE" bash "$REPO_DIR/bin/agent" cost --history
assert_output_contains "Cost history" "history header present"
assert_output_contains "agent-claude-alpha" "alpha container shown"
assert_output_contains "agent-claude-beta" "beta container shown"
assert_output_contains "20.00" "alpha total is 12.5+7.5=20.00"
assert_output_contains "3.25" "beta total is 3.25"
assert_output_contains "Total" "total row present"
assert_output_contains "23.25" "grand total is 23.25"

# ===========================================================================
# Test 2: --history 1d filters by date (old entries excluded)
# ===========================================================================
cat >"$AUDIT_FILE" <<EOF
{"timestamp":"2020-01-01T00:00:00Z","action":"cost","container":"agent-claude-old","cost":100.0}
{"timestamp":"$now_ts","action":"cost","container":"agent-claude-recent","cost":5.0}
EOF

run_ok "history 1d filters old entries" \
  env SAFE_AGENTIC_AUDIT_LOG="$AUDIT_FILE" bash "$REPO_DIR/bin/agent" cost --history 1d
assert_output_contains "last 1 days" "period shown in header"
assert_output_contains "agent-claude-recent" "recent entry shown"
assert_output_not_contains "agent-claude-old" "old entry filtered out"
assert_output_contains "5.00" "total is 5.00 (only recent)"

# ===========================================================================
# Test 3: --history with empty audit log shows zero total
# ===========================================================================
: >"$AUDIT_FILE"
run_ok "history empty log shows zero total" \
  env SAFE_AGENTIC_AUDIT_LOG="$AUDIT_FILE" bash "$REPO_DIR/bin/agent" cost --history
assert_output_contains "Total" "total row present for empty log"
assert_output_contains '$0.00' "zero total for empty log"

# ===========================================================================
# Test 4: --history skips non-cost actions
# ===========================================================================
cat >"$AUDIT_FILE" <<EOF
{"timestamp":"$now_ts","action":"spawn","container":"agent-claude-foo","details":"type=claude"}
{"timestamp":"$now_ts","action":"stop","container":"agent-claude-foo","details":""}
{"timestamp":"$now_ts","action":"cost","container":"agent-claude-foo","cost":2.50}
EOF

run_ok "history skips non-cost actions" \
  env SAFE_AGENTIC_AUDIT_LOG="$AUDIT_FILE" bash "$REPO_DIR/bin/agent" cost --history
assert_output_contains "2.50" "only cost action counted"
assert_output_contains "agent-claude-foo" "container shown"

# ===========================================================================
# Test 5: --history with no audit file fails with message
# ===========================================================================
rm -f "$TMP_DIR/nonexistent.jsonl"
run_fails "history no audit file" \
  env SAFE_AGENTIC_AUDIT_LOG="$TMP_DIR/nonexistent.jsonl" bash "$REPO_DIR/bin/agent" cost --history
assert_output_contains "No audit log found" "error message shown for missing file"

# ===========================================================================
# Test 6: --history appears in help text
# ===========================================================================
run_ok "cost help mentions history" bash "$REPO_DIR/bin/agent" cost --help
assert_output_contains "history" "help mentions --history"

# ===========================================================================
# Test 7: --history with malformed JSON lines is resilient
# ===========================================================================
cat >"$AUDIT_FILE" <<EOF
not json at all
{"timestamp":"$now_ts","action":"cost","container":"agent-valid","cost":1.00}
{"broken json
{"timestamp":"$now_ts","action":"cost","container":"agent-valid","cost":2.00}
EOF

run_ok "history handles malformed lines" \
  env SAFE_AGENTIC_AUDIT_LOG="$AUDIT_FILE" bash "$REPO_DIR/bin/agent" cost --history
assert_output_contains "3.00" "valid entries summed despite bad lines"

# ===========================================================================
# Test 8: _record_cost_to_audit writes cost entry on stop
# ===========================================================================
: >"$AUDIT_FILE"
: >"$ORB_LOG"
run_ok "stop records cost to audit" \
  env PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" \
  TEST_JSONL_DATA='{"model":"claude-sonnet-4-6-20250219","usage":{"input_tokens":1000000,"output_tokens":100000}}' \
  TEST_CONTAINER_STATE="running" TEST_AGENT_TYPE="claude" \
  SAFE_AGENTIC_AUDIT_LOG="$AUDIT_FILE" \
  bash "$REPO_DIR/bin/agent" stop agent-claude-test-task

# Check that the audit file has a cost entry
if [ -s "$AUDIT_FILE" ]; then
  cost_lines=$(grep -c '"action":"cost"' "$AUDIT_FILE" || echo "0")
  if [ "$cost_lines" -ge 1 ]; then
    ((++pass))
  else
    echo "FAIL: stop did not write cost entry to audit log" >&2
    cat "$AUDIT_FILE" >&2
    ((++fail))
  fi
  # Verify the cost entry has a numeric cost field
  if grep '"action":"cost"' "$AUDIT_FILE" | head -1 | grep -q '"cost":'; then
    ((++pass))
  else
    echo "FAIL: cost entry missing numeric cost field" >&2
    ((++fail))
  fi
else
  echo "FAIL: audit file empty after stop" >&2
  ((++fail)); ((++fail))
fi

# ===========================================================================
# Test 9: stop with no session data does not write cost entry
# ===========================================================================
: >"$AUDIT_FILE"
: >"$ORB_LOG"
run_ok "stop no session data skips cost" \
  env PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" \
  TEST_JSONL_DATA="" \
  TEST_CONTAINER_STATE="running" TEST_AGENT_TYPE="claude" \
  SAFE_AGENTIC_AUDIT_LOG="$AUDIT_FILE" \
  bash "$REPO_DIR/bin/agent" stop agent-claude-test-task

# The only entry should be the "stop" action from audit_log, not a "cost" entry
cost_lines_empty=$(grep -c '"action":"cost"' "$AUDIT_FILE" 2>/dev/null || true)
cost_lines_empty="${cost_lines_empty:-0}"
cost_lines_empty=$(echo "$cost_lines_empty" | tr -d '[:space:]')
if [ "$cost_lines_empty" -eq 0 ]; then
  ((++pass))
else
  echo "FAIL: stop with no data should not write cost entry" >&2
  ((++fail))
fi

# ===========================================================================
# Test 10: --history default period is 7d
# ===========================================================================
cat >"$AUDIT_FILE" <<EOF
{"timestamp":"$now_ts","action":"cost","container":"agent-test","cost":10.0}
EOF

run_ok "history default period is 7d" \
  env SAFE_AGENTIC_AUDIT_LOG="$AUDIT_FILE" bash "$REPO_DIR/bin/agent" cost --history
assert_output_contains "last 7 days" "default period is 7 days"

# ===========================================================================
# Summary
# ===========================================================================
echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
