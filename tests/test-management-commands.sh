#!/usr/bin/env bash
# Command coverage for setup/list/vm management paths.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"
STATE_FILE="$TMP_DIR/vm-created"

trap 'rm -rf "$TMP_DIR"' EXIT

mkdir -p "$FAKE_BIN"

cat >"$FAKE_BIN/orb" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

log_file="${TEST_ORB_LOG:?}"
state_file="${TEST_VM_STATE:?}"
cmd="${1:-}"
shift || true

case "$cmd" in
  list)
    if [ -f "$state_file" ] || [ "${TEST_VM_EXISTS:-0}" = "1" ]; then
      echo "safe-agentic"
    fi
    ;;
  create)
    printf 'create|%s\n' "$*" >>"$log_file"
    : >"$state_file"
    ;;
  start|stop|ssh)
    printf '%s|%s\n' "$cmd" "$*" >>"$log_file"
    ;;
  run)
    [ "${1:-}" = "-m" ] && shift 2
    printf 'run|%s\n' "$*" >>"$log_file"
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-lc" ] && [[ "${3:-}" == cat\ \>\ * ]]; then
      cat >/dev/null
    fi
    ;;
  *)
    echo "unexpected orb command: $cmd" >&2
    exit 1
    ;;
esac
EOF

chmod +x "$FAKE_BIN/orb"

pass=0
fail=0

run_agent() {
  : >"$ORB_LOG"
  rm -f "$STATE_FILE"
  TEST_VM_EXISTS="${TEST_VM_EXISTS:-1}" \
  TEST_ORB_LOG="$ORB_LOG" \
  TEST_VM_STATE="$STATE_FILE" \
  PATH="$FAKE_BIN:$PATH" \
  bash "$REPO_DIR/bin/agent" "$@"
}

assert_contains() {
  local haystack="$1"
  local needle="$2"
  local label="$3"
  if [[ "$haystack" == *"$needle"* ]]; then
    ((++pass))
  else
    echo "FAIL: $label — missing '$needle'" >&2
    echo "  in: $haystack" >&2
    ((++fail))
  fi
}

assert_not_contains() {
  local haystack="$1"
  local needle="$2"
  local label="$3"
  if [[ "$haystack" != *"$needle"* ]]; then
    ((++pass))
  else
    echo "FAIL: $label — unexpected '$needle'" >&2
    echo "  in: $haystack" >&2
    ((++fail))
  fi
}

# --- list uses filtered docker ps output ---
TEST_VM_EXISTS=1 run_agent list >/dev/null 2>&1
list_log="$(cat "$ORB_LOG")"
assert_contains "$list_log" "run|docker ps --filter name=^agent- --format table {{.Names}}" "list docker ps filter"

# --- vm subcommands map to orb controls ---
TEST_VM_EXISTS=1 run_agent vm ssh >/dev/null 2>&1
assert_contains "$(cat "$ORB_LOG")" "ssh|safe-agentic" "vm ssh"

TEST_VM_EXISTS=1 run_agent vm stop >/dev/null 2>&1
assert_contains "$(cat "$ORB_LOG")" "stop|safe-agentic" "vm stop"

TEST_VM_EXISTS=1 run_agent vm start >/dev/null 2>&1
vm_start_log="$(cat "$ORB_LOG")"
assert_contains "$vm_start_log" "start|safe-agentic" "vm start"
assert_contains "$vm_start_log" "run|bash -lc cat > '/tmp/setup.sh'" "vm start copies setup script"
assert_contains "$vm_start_log" "run|bash /tmp/setup.sh" "vm start reruns hardening"

# --- setup creates VM when absent, bootstraps, builds image ---
TEST_VM_EXISTS=0 run_agent setup >/dev/null 2>&1
setup_log="$(cat "$ORB_LOG")"
assert_contains "$setup_log" "create|ubuntu safe-agentic" "setup creates vm"
assert_contains "$setup_log" "start|safe-agentic" "setup starts vm"
assert_contains "$setup_log" "run|bash -lc cat > '/tmp/setup.sh'" "setup copies setup script"
assert_contains "$setup_log" "run|bash /tmp/setup.sh" "setup runs hardening"
assert_contains "$setup_log" "run|docker build -t safe-agentic:latest /tmp/safe-agentic/" "setup builds image"

# --- setup with existing VM does not recreate it ---
TEST_VM_EXISTS=1 run_agent setup >/dev/null 2>&1
existing_setup_log="$(cat "$ORB_LOG")"
assert_not_contains "$existing_setup_log" "create|ubuntu safe-agentic" "setup skips create when vm exists"
assert_contains "$existing_setup_log" "start|safe-agentic" "setup still starts existing vm"

echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
