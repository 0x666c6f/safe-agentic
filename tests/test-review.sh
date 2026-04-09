#!/usr/bin/env bash
# Tests for `agent review` — AI code review of agent's changes.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"
OUT_LOG="$TMP_DIR/out.log"
ERR_LOG="$TMP_DIR/err.log"
mkdir -p "$FAKE_BIN"

# ---------------------------------------------------------------------------
# Fake orb that simulates: docker inspect (running), docker exec (codex/git)
# TEST_CONTAINER_TYPE: "claude" or "codex" (label on container)
# TEST_HAS_CODEX:      "1" if codex should be found in container, else "0"
# ---------------------------------------------------------------------------
cat >"$FAKE_BIN/orb" <<'ORBEOF'
#!/usr/bin/env bash
set -euo pipefail

log_file="${TEST_ORB_LOG:?}"
container_type="${TEST_CONTAINER_TYPE:-claude}"
has_codex="${TEST_HAS_CODEX:-1}"

cmd="${1:-}"
shift || true

case "$cmd" in
  list)
    echo "safe-agentic"
    ;;
  run)
    [ "${1:-}" = "-m" ] && shift 2
    printf '%s\n' "$*" >>"$log_file"

    # resolve --latest container name
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "ps" ] && [ "${3:-}" = "-a" ] && [ "${4:-}" = "--latest" ]; then
      echo "agent-claude-my-task"
      exit 0
    fi

    # docker inspect --format '{{.State.Status}}' <name>
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ] && [[ "${4:-}" == *State.Status* ]]; then
      echo "running"
      exit 0
    fi

    # docker inspect --format '{{index .Config.Labels "safe-agentic.agent-type"}}' <name>
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ] && [[ "${4:-}" == *agent-type* ]]; then
      echo "$container_type"
      exit 0
    fi

    # docker exec <name> command -v codex  (codex availability check)
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "exec" ] && [ "${4:-}" = "command" ] && [ "${5:-}" = "-v" ] && [ "${6:-}" = "codex" ]; then
      if [ "$has_codex" = "1" ]; then
        echo "/usr/local/bin/codex"
        exit 0
      else
        exit 1
      fi
    fi

    # docker exec <name> bash -lc "... codex review ..."
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "exec" ] && [ "${4:-}" = "bash" ] && [ "${5:-}" = "-lc" ]; then
      printf 'exec_cmd: %s\n' "${6:-}" >>"$log_file"
      exit 0
    fi

    exit 0
    ;;
  *)
    echo "unexpected orb command: $cmd" >&2
    exit 1
    ;;
esac
ORBEOF
chmod +x "$FAKE_BIN/orb"

pass=0
fail=0

run_ok() {
  local label="$1"
  shift
  if TEST_ORB_LOG="$ORB_LOG" PATH="$FAKE_BIN:$PATH" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
    ((++pass))
  else
    echo "FAIL: $label — expected zero exit" >&2
    cat "$ERR_LOG" >&2 || true
    ((++fail))
  fi
}

run_fails() {
  local label="$1"
  shift
  if TEST_ORB_LOG="$ORB_LOG" PATH="$FAKE_BIN:$PATH" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
    echo "FAIL: $label — expected non-zero exit" >&2
    ((++fail))
  else
    ((++pass))
  fi
}

assert_contains() {
  local haystack="$1" needle="$2" label="${3:-}"
  if [[ "$haystack" == *"$needle"* ]]; then
    ((++pass))
  else
    echo "FAIL${label:+: $label}: missing '$needle'" >&2
    echo "  in: $haystack" >&2
    ((++fail))
  fi
}

assert_not_contains() {
  local haystack="$1" needle="$2" label="${3:-}"
  if [[ "$haystack" != *"$needle"* ]]; then
    ((++pass))
  else
    echo "FAIL${label:+: $label}: unexpected '$needle'" >&2
    echo "  in: $haystack" >&2
    ((++fail))
  fi
}

# --- help flag prints usage ---
help_output="$(TEST_ORB_LOG="$ORB_LOG" PATH="$FAKE_BIN:$PATH" \
  bash "$REPO_DIR/bin/agent" review --help 2>&1 || true)"
assert_contains "$help_output" "agent review" "help shows usage"
assert_contains "$help_output" "--base" "help shows --base option"
assert_contains "$help_output" "--latest" "help shows --latest option"

# --- no args fails with helpful message ---
run_fails "no args fails" bash "$REPO_DIR/bin/agent" review
assert_contains "$(cat "$ERR_LOG")" "agent help review" "no-args error mentions help"

# --- with codex, no --base: runs codex review --uncommitted ---
: >"$ORB_LOG"
TEST_HAS_CODEX=1 TEST_CONTAINER_TYPE=claude run_ok "codex uncommitted review" \
  bash "$REPO_DIR/bin/agent" review my-task
log="$(cat "$ORB_LOG")"
assert_contains "$log" "codex review --uncommitted" "codex review --uncommitted invoked"
assert_not_contains "$log" "git diff" "git diff not used when codex present"

# --- with codex + --base: runs codex review --base BRANCH ---
: >"$ORB_LOG"
TEST_HAS_CODEX=1 TEST_CONTAINER_TYPE=claude run_ok "codex base review" \
  bash "$REPO_DIR/bin/agent" review my-task --base main
log="$(cat "$ORB_LOG")"
assert_contains "$log" "codex review --base main" "codex review --base main invoked"
assert_not_contains "$log" "git diff" "git diff not used when codex present with --base"

# --- without codex, no --base: falls back to git diff ---
: >"$ORB_LOG"
TEST_HAS_CODEX=0 TEST_CONTAINER_TYPE=claude run_ok "git diff fallback" \
  bash "$REPO_DIR/bin/agent" review my-task
log="$(cat "$ORB_LOG")"
assert_contains "$log" "git diff" "git diff used when codex absent"
assert_not_contains "$log" "codex review" "codex not invoked when absent"

# --- without codex + --base: falls back to git diff BASE ---
: >"$ORB_LOG"
TEST_HAS_CODEX=0 TEST_CONTAINER_TYPE=codex run_ok "git diff base fallback" \
  bash "$REPO_DIR/bin/agent" review my-task --base origin/main
log="$(cat "$ORB_LOG")"
assert_contains "$log" "git diff origin/main" "git diff origin/main used when codex absent"

# --- --latest resolves latest container ---
: >"$ORB_LOG"
TEST_HAS_CODEX=1 TEST_CONTAINER_TYPE=claude run_ok "latest flag" \
  bash "$REPO_DIR/bin/agent" review --latest
log="$(cat "$ORB_LOG")"
assert_contains "$log" "docker ps -a --latest" "latest container lookup performed"

# --- name + --latest is rejected ---
run_fails "name and --latest rejected" \
  bash "$REPO_DIR/bin/agent" review my-task --latest
assert_contains "$(cat "$ERR_LOG")" "not both" "mutual exclusion error"

# --- unknown flag is rejected ---
run_fails "unknown flag rejected" \
  bash "$REPO_DIR/bin/agent" review my-task --unknown-flag
err="$(cat "$ERR_LOG")"
assert_contains "$err" "Unexpected argument" "unknown flag error"

# --- review listed in general help ---
help_out="$(TEST_ORB_LOG="$ORB_LOG" PATH="$FAKE_BIN:$PATH" \
  bash "$REPO_DIR/bin/agent" help 2>&1 || true)"
assert_contains "$help_out" "review" "general help lists review command"

# --- -h shorthand works ---
h_output="$(TEST_ORB_LOG="$ORB_LOG" PATH="$FAKE_BIN:$PATH" \
  bash "$REPO_DIR/bin/agent" review -h 2>&1 || true)"
assert_contains "$h_output" "agent review" "review -h shows usage"

# --- container not running → error ---
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
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ] && [[ "${4:-}" == *State.Status* ]]; then
      echo "exited"; exit 0
    fi
    exit 0 ;;
  *)
    echo "unexpected orb command: $cmd" >&2; exit 1 ;;
esac
ORBEOF
chmod +x "$FAKE_BIN/orb"

: >"$ORB_LOG"
run_fails "review container not running" \
  bash "$REPO_DIR/bin/agent" review my-task
assert_contains "$(cat "$ERR_LOG")" "not running" "review container not running shows error"

echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
