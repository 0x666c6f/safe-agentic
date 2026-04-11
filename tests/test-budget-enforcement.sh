#!/usr/bin/env bash
# Unit tests for budget enforcement functions in agent-lib.sh.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

# Stubs expected by agent-lib.sh (normally defined in bin/agent).
IMAGE_NAME="safe-agentic"
die()     { exit 1; }
info()    { :; }
warn()    { :; }
vm_exec() { return 1; }
export -f die info warn vm_exec
export IMAGE_NAME

source "$REPO_DIR/bin/agent-lib.sh"

pass=0
fail=0

assert_eq() {
  local label="$1" expected="$2" actual="$3"
  if [ "$expected" = "$actual" ]; then
    ((++pass))
  else
    echo "FAIL: $label — expected '$expected', got '$actual'" >&2
    ((++fail))
  fi
}

# --- Test 1: compute_running_cost returns correct cost for opus ---
# 1M input tokens @ $15/M = $15.00, 100K output tokens @ $75/M = $7.50, total = $22.50
JSONL_OPUS="$TMP_DIR/opus.jsonl"
cat >"$JSONL_OPUS" <<'EOF'
{"message":{"model":"claude-opus-4-6","usage":{"input_tokens":1000000,"output_tokens":100000}}}
EOF
cost=$(compute_running_cost "$JSONL_OPUS")
assert_eq "opus cost = 22.50" "22.50" "$cost"

# --- Test 2: Zero tokens returns 0.00 ---
JSONL_ZERO="$TMP_DIR/zero.jsonl"
cat >"$JSONL_ZERO" <<'EOF'
{"message":{"model":"claude-opus-4-6","usage":{"input_tokens":0,"output_tokens":0}}}
EOF
cost=$(compute_running_cost "$JSONL_ZERO")
assert_eq "zero tokens cost = 0.00" "0.00" "$cost"

# --- Test 3: check_budget returns 1 (failure) when cost exceeds budget ---
JSONL_OVER="$TMP_DIR/over.jsonl"
cat >"$JSONL_OVER" <<'EOF'
{"message":{"model":"claude-opus-4-6","usage":{"input_tokens":1000000,"output_tokens":100000}}}
EOF
# Budget is $10, cost is $22.50 — should fail
if check_budget "$JSONL_OVER" "10"; then
  echo "FAIL: check_budget should fail when cost ($22.50) > budget ($10)" >&2
  ((++fail))
else
  ((++pass))
fi

# --- Test 4: check_budget returns 0 (success) when under budget ---
# Budget is $50, cost is $22.50 — should pass
if check_budget "$JSONL_OVER" "50"; then
  ((++pass))
else
  echo "FAIL: check_budget should pass when cost ($22.50) <= budget ($50)" >&2
  ((++fail))
fi

# --- Test 5: Unknown model uses DEFAULT_PRICE ($3/$15) ---
JSONL_UNKNOWN="$TMP_DIR/unknown.jsonl"
cat >"$JSONL_UNKNOWN" <<'EOF'
{"message":{"model":"some-unknown-model-v99","usage":{"input_tokens":1000000,"output_tokens":100000}}}
EOF
# 1M input @ $3/M = $3.00, 100K output @ $15/M = $1.50, total = $4.50
cost=$(compute_running_cost "$JSONL_UNKNOWN")
assert_eq "unknown model cost = 4.50" "4.50" "$cost"

# --- Test 6: Missing file returns 0.00 ---
cost=$(compute_running_cost "$TMP_DIR/nonexistent.jsonl")
assert_eq "missing file cost = 0.00" "0.00" "$cost"

# --- Test 7: Multiple entries are summed ---
JSONL_MULTI="$TMP_DIR/multi.jsonl"
cat >"$JSONL_MULTI" <<'EOF'
{"message":{"model":"claude-opus-4-6","usage":{"input_tokens":500000,"output_tokens":50000}}}
{"message":{"model":"claude-opus-4-6","usage":{"input_tokens":500000,"output_tokens":50000}}}
EOF
# Each: 500K input @ $15/M = $7.50, 50K output @ $75/M = $3.75 = $11.25
# Total: $22.50
cost=$(compute_running_cost "$JSONL_MULTI")
assert_eq "multi-entry cost = 22.50" "22.50" "$cost"

# --- Test 8: cache_creation_input_tokens counted ---
JSONL_CACHE="$TMP_DIR/cache.jsonl"
cat >"$JSONL_CACHE" <<'EOF'
{"message":{"model":"claude-sonnet-4-6","usage":{"input_tokens":500000,"cache_creation_input_tokens":500000,"output_tokens":100000}}}
EOF
# (500K + 500K) input @ $3/M = $3.00, 100K output @ $15/M = $1.50, total = $4.50
cost=$(compute_running_cost "$JSONL_CACHE")
assert_eq "cache tokens cost = 4.50" "4.50" "$cost"

# --- Test 9: check_budget exact boundary (cost == budget) passes ---
JSONL_EXACT="$TMP_DIR/exact.jsonl"
cat >"$JSONL_EXACT" <<'EOF'
{"message":{"model":"claude-opus-4-6","usage":{"input_tokens":1000000,"output_tokens":100000}}}
EOF
# Cost is $22.50, budget is $22.50 — should pass (<=)
if check_budget "$JSONL_EXACT" "22.50"; then
  ((++pass))
else
  echo "FAIL: check_budget should pass when cost equals budget" >&2
  ((++fail))
fi

# --- Test 10: Empty JSONL file returns 0.00 ---
JSONL_EMPTY="$TMP_DIR/empty.jsonl"
touch "$JSONL_EMPTY"
cost=$(compute_running_cost "$JSONL_EMPTY")
assert_eq "empty file cost = 0.00" "0.00" "$cost"

# --- Test 11: Malformed JSON lines are skipped ---
JSONL_BAD="$TMP_DIR/bad.jsonl"
cat >"$JSONL_BAD" <<'EOF'
not valid json at all
{"message":{"model":"claude-opus-4-6","usage":{"input_tokens":1000000,"output_tokens":100000}}}
{broken
EOF
cost=$(compute_running_cost "$JSONL_BAD")
assert_eq "malformed lines skipped, cost = 22.50" "22.50" "$cost"

# --- Test 12: emit_event writes valid JSONL ---
EVENTS_FILE="$TMP_DIR/events.jsonl"
emit_event "$EVENTS_FILE" "agent.budget_exceeded" '{"name":"test","estimated_cost":22.50,"budget":10}'
line=$(cat "$EVENTS_FILE")
if echo "$line" | python3 -c "import json,sys; d=json.load(sys.stdin); assert d['event']=='agent.budget_exceeded'" 2>/dev/null; then
  ((++pass))
else
  echo "FAIL: emit_event did not produce valid JSONL with expected event" >&2
  ((++fail))
fi

echo "$pass passed, $fail failed (budget enforcement)"
[ "$fail" -eq 0 ]
