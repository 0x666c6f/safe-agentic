#!/usr/bin/env bash
# Tests for the `agent sessions` command.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"
OUT_LOG="$TMP_DIR/out.log"
ERR_LOG="$TMP_DIR/err.log"
EXPORT_DIR="$TMP_DIR/export"
DEST_DIR="$TMP_DIR/dest"
mkdir -p "$FAKE_BIN" "$EXPORT_DIR" "$DEST_DIR"

mkdir -p "$EXPORT_DIR/sessions/2026/04/10" "$EXPORT_DIR/log"
printf '{"type":"assistant","message":{"content":"done"}}\n' >"$EXPORT_DIR/sessions/2026/04/10/rollout.jsonl"
printf '{"session_id":"abc"}\n' >"$EXPORT_DIR/history.jsonl"
printf 'codex log\n' >"$EXPORT_DIR/log/codex-tui.log"
printf 'sqlite payload\n' >"$EXPORT_DIR/logs_1.sqlite"
printf 'sqlite state\n' >"$EXPORT_DIR/state_5.sqlite"

cat >"$FAKE_BIN/orb" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
log_file="${TEST_ORB_LOG:?}"
export_dir="${TEST_EXPORT_DIR:?}"
cmd="${1:-}"; shift || true
case "$cmd" in
  list)
    echo "safe-agentic"
    ;;
  run)
    [ "${1:-}" = "-m" ] && shift 2
    printf '%s\n' "$*" >>"$log_file"
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ] && [[ "${4:-}" == *State.Status* ]]; then
      echo "${TEST_CONTAINER_STATE:-running}"
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ] && [[ "${4:-}" == *agent-type* ]]; then
      echo "${TEST_AGENT_TYPE:-codex}"
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "ps" ] && [[ "$*" == *"--latest"* ]]; then
      echo "agent-codex-test-task"
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "start" ]; then
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "exec" ]; then
      if [ "${TEST_EMPTY_EXPORT:-0}" = "1" ]; then
        exit 0
      fi
      tar -cf - -C "$export_dir" .
      exit 0
    fi
    exit 0
    ;;
  *)
    echo "unexpected orb command: $cmd" >&2
    exit 1
    ;;
esac
EOF
chmod +x "$FAKE_BIN/orb"

cat >"$FAKE_BIN/git" <<'EOF'
#!/usr/bin/env bash
if [ "${1:-}" = "config" ] && [ "${2:-}" = "user.name" ]; then echo "X"; exit 0; fi
if [ "${1:-}" = "config" ] && [ "${2:-}" = "user.email" ]; then echo "x@x"; exit 0; fi
exec /usr/bin/git "$@"
EOF
chmod +x "$FAKE_BIN/git"

pass=0
fail=0

run_ok() {
  local label="$1"; shift
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_EXPORT_DIR="$EXPORT_DIR" \
     TEST_CONTAINER_STATE="${TEST_CONTAINER_STATE:-running}" TEST_AGENT_TYPE="${TEST_AGENT_TYPE:-codex}" \
     TEST_EMPTY_EXPORT="${TEST_EMPTY_EXPORT:-0}" \
     "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
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
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_EXPORT_DIR="$EXPORT_DIR" \
     TEST_CONTAINER_STATE="${TEST_CONTAINER_STATE:-running}" TEST_AGENT_TYPE="${TEST_AGENT_TYPE:-codex}" \
     TEST_EMPTY_EXPORT="${TEST_EMPTY_EXPORT:-0}" \
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
    echo "FAIL: $label: missing '$needle'" >&2
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
    echo "FAIL: $label: missing '$needle' in orb log" >&2
    cat "$ORB_LOG" >&2
    ((++fail))
  fi
}

assert_file_exists() {
  local path="$1" label="$2"
  if [ -f "$path" ]; then
    ((++pass))
  else
    echo "FAIL: $label: missing file $path" >&2
    find "$DEST_DIR" -maxdepth 6 -type f | sort >&2 || true
    ((++fail))
  fi
}

run_ok "sessions help" bash "$REPO_DIR/bin/agent" sessions --help
assert_output_contains "agent sessions" "sessions help shows command"
assert_output_contains "destination" "sessions help shows destination"

run_fails "sessions no args" bash "$REPO_DIR/bin/agent" sessions
assert_output_contains "agent help sessions" "sessions no-args usage pointer"

: >"$ORB_LOG"
rm -rf "$DEST_DIR" && mkdir -p "$DEST_DIR"
run_ok "sessions exports codex bundle" bash "$REPO_DIR/bin/agent" sessions agent-codex-test-task "$DEST_DIR"
assert_output_contains "Exported" "sessions export confirms count"
assert_file_exists "$DEST_DIR/history.jsonl" "sessions exports history"
assert_file_exists "$DEST_DIR/sessions/2026/04/10/rollout.jsonl" "sessions exports rollout"
assert_file_exists "$DEST_DIR/log/codex-tui.log" "sessions exports codex logs"
assert_file_exists "$DEST_DIR/logs_1.sqlite" "sessions exports codex log db"
assert_file_exists "$DEST_DIR/state_5.sqlite" "sessions exports codex state db"
assert_log_contains "bash -c" "sessions export uses non-login shell for clean tar stream"
assert_log_contains "projects" "sessions export searches project sessions too"
assert_log_contains "log" "sessions export includes log dir"
assert_log_contains "state_\\*.sqlite" "sessions export includes sqlite state patterns"

: >"$ORB_LOG"
rm -rf "$DEST_DIR" && mkdir -p "$DEST_DIR"
TEST_CONTAINER_STATE=exited run_ok "sessions starts stopped container" bash "$REPO_DIR/bin/agent" sessions agent-codex-test-task "$DEST_DIR"
assert_output_contains "Starting container" "sessions starts stopped container"
TEST_CONTAINER_STATE=running

: >"$ORB_LOG"
rm -rf "$DEST_DIR" && mkdir -p "$DEST_DIR"
TEST_EMPTY_EXPORT=1 run_ok "sessions empty export warns" bash "$REPO_DIR/bin/agent" sessions agent-codex-test-task "$DEST_DIR"
assert_output_contains "No session data" "sessions empty export warns gracefully"
TEST_EMPTY_EXPORT=0

echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
