#!/usr/bin/env bash
# Tests for agent cost command.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"
OUT_LOG="$TMP_DIR/out.log"
ERR_LOG="$TMP_DIR/err.log"

mkdir -p "$FAKE_BIN"

cat >"$FAKE_BIN/orb" <<'EOF'
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
      echo "running"; exit 0
    fi
    # docker inspect for agent-type label
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ] && [[ "${4:-}" == *agent-type* ]]; then
      echo "claude"; exit 0
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
    exit 0 ;;
  *) echo "unexpected orb command: $cmd" >&2; exit 1 ;;
esac
EOF
chmod +x "$FAKE_BIN/orb"

pass=0
fail=0

run_ok() {
  local label="$1"; shift
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_JSONL_DATA="${TEST_JSONL_DATA:-}" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
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
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_JSONL_DATA="${TEST_JSONL_DATA:-}" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
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

# ---------------------------------------------------------------------------
# Help
# ---------------------------------------------------------------------------
run_ok "cost --help" bash "$REPO_DIR/bin/agent" cost --help
assert_output_contains "agent cost" "cost help shows command name"
assert_output_contains "latest" "cost help mentions --latest"

run_ok "help cost topic" bash "$REPO_DIR/bin/agent" help cost
assert_output_contains "agent cost" "help cost topic shows command"

# ---------------------------------------------------------------------------
# cost with no args should fail (requires container name or --latest)
# ---------------------------------------------------------------------------
run_fails "cost no args" bash "$REPO_DIR/bin/agent" cost
assert_output_contains "agent help cost" "cost no-args usage pointer"

# ---------------------------------------------------------------------------
# cost parses session JSONL and computes totals
# ---------------------------------------------------------------------------
: >"$ORB_LOG"
TEST_JSONL_DATA='{"model":"claude-sonnet-4-6-20250219","usage":{"input_tokens":1000,"output_tokens":500}}'
run_ok "cost with sample data" bash "$REPO_DIR/bin/agent" cost agent-claude-test-task
assert_output_contains "1,000" "cost shows input tokens"
assert_output_contains "500" "cost shows output tokens"
assert_output_contains "TOTAL" "cost shows total row"
assert_output_contains "claude-sonnet-4-6" "cost shows model name"

# ---------------------------------------------------------------------------
# cost with nested usage (in payload)
# ---------------------------------------------------------------------------
: >"$ORB_LOG"
TEST_JSONL_DATA='{"payload":{"model":"claude-opus-4-6","usage":{"input_tokens":2000,"output_tokens":800}}}'
run_ok "cost with nested payload usage" bash "$REPO_DIR/bin/agent" cost agent-claude-test-task
assert_output_contains "claude-opus-4-6" "nested payload model shown"
assert_output_contains "2,000" "nested input tokens shown"

# ---------------------------------------------------------------------------
# cost with no session data warns gracefully
# ---------------------------------------------------------------------------
: >"$ORB_LOG"
TEST_JSONL_DATA=""
run_ok "cost no session data" bash "$REPO_DIR/bin/agent" cost agent-claude-test-task
assert_output_contains "No session data" "cost warns when no data"

echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
