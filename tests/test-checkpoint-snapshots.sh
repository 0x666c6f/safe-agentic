#!/usr/bin/env bash
# Tests for container state snapshots (docker commit) and fork in checkpoint.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"
ERR_LOG="$TMP_DIR/error.log"
OUT_LOG="$TMP_DIR/out.log"
VERIFY_STATE="$TMP_DIR/verify-state"
mkdir -p "$FAKE_BIN"

# ---------------------------------------------------------------------------
# Fake orb: handles docker commit, exec (tmux, kill, tail), run, inspect
# ---------------------------------------------------------------------------
cat >"$FAKE_BIN/orb" <<'ORBEOF'
#!/usr/bin/env bash
set -euo pipefail
log_file="${TEST_ORB_LOG:?}"
verify_state="${TEST_VERIFY_STATE:?}"
cmd="${1:-}"; shift || true
case "$cmd" in
  list) echo "safe-agentic" ;;
  run)
    [ "${1:-}" = "-m" ] && shift 2
    printf '%s\n' "$*" >>"$log_file"

    # docker inspect --format '{{.State.Status}}' <name>
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ] && [[ "${4:-}" == *State.Status* ]]; then
      echo "running"; exit 0
    fi
    # docker inspect --format '{{index .Config.Labels ...}}' <name> → agent type
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ] && [[ "${4:-}" == *agent-type* ]]; then
      echo "claude"; exit 0
    fi
    # docker inspect --format ... latest
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ] && [[ "${4:-}" == *latest* ]]; then
      echo "agent-claude-test-1234"; exit 0
    fi
    # docker ps -a --latest
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "ps" ] && [ "${3:-}" = "-a" ] && [ "${4:-}" = "--latest" ]; then
      echo "agent-claude-test-1234"; exit 0
    fi
    # docker commit
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "commit" ]; then
      echo "sha256:abc123snapshot"; exit 0
    fi
    # docker network inspect / create / rm
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "network" ]; then
      exit 0
    fi
    # docker run -d (for fork)
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "run" ]; then
      echo "forked-container-id"; exit 0
    fi
    # docker exec
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "exec" ]; then
      shift 2  # remove "docker exec"
      # Skip -e KEY=VAL and -i flags
      while [ "${1:-}" = "-e" ] || [ "${1:-}" = "-i" ]; do
        [ "${1:-}" = "-e" ] && shift 2 || shift
      done
      name="$1"; shift  # container name
      full_cmd="$*"
      # tmux list-panes → return a PID
      if echo "$full_cmd" | grep -q "tmux list-panes"; then
        echo "12345"; exit 0
      fi
      # kill -STOP / -CONT → no-op
      if echo "$full_cmd" | grep -q "kill -STOP\|kill -CONT"; then
        exit 0
      fi
      # git stash create
      if echo "$full_cmd" | grep -q "git stash create"; then
        echo "abc123def456"; exit 0
      fi
      # git update-ref
      if echo "$full_cmd" | grep -q "update-ref"; then
        exit 0
      fi
      # checkpoints.jsonl metadata write
      if echo "$full_cmd" | grep -q "checkpoints.jsonl" && echo "$full_cmd" | grep -q "mkdir"; then
        exit 0
      fi
      # tail -1 checkpoints.jsonl → return snapshot metadata (for fork)
      if echo "$full_cmd" | grep -q "tail -1"; then
        # Check verify-state to decide whether to return data or empty
        if [ -f "$verify_state" ] && grep -q "no-checkpoint" "$verify_state" 2>/dev/null; then
          exit 1
        fi
        echo '{"timestamp":"20260410-120000","label":"snap1","image":"safe-agentic-checkpoint:agent-claude-test-1234-20260410-120000","git_ref":"abc123"}'; exit 0
      fi
      exit 0
    fi
    exit 0 ;;
  push|start|stop|create|ssh) ;;
  *) echo "unexpected orb command: $cmd" >&2; exit 1 ;;
esac
ORBEOF
chmod +x "$FAKE_BIN/orb"

cat >"$FAKE_BIN/git" <<'GITEOF'
#!/usr/bin/env bash
if [ "${1:-}" = "config" ] && [ "${2:-}" = "user.name" ]; then echo "X"; exit 0; fi
if [ "${1:-}" = "config" ] && [ "${2:-}" = "user.email" ]; then echo "x@x"; exit 0; fi
exec /usr/bin/git "$@"
GITEOF
chmod +x "$FAKE_BIN/git"

pass=0
fail=0

run_ok() {
  local label="$1"; shift
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
    ((++pass))
  else
    echo "FAIL: $label: expected zero exit" >&2
    cat "$ERR_LOG" >&2
    ((++fail))
  fi
}

run_fails() {
  local label="$1"; shift
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
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
    echo "FAIL: $label: '$needle' not in orb log" >&2
    cat "$ORB_LOG" >&2
    ((++fail))
  fi
}

assert_log_not_contains() {
  local needle="$1" label="$2"
  if grep -q -- "$needle" "$ORB_LOG" 2>/dev/null; then
    echo "FAIL: $label: '$needle' unexpectedly found in orb log" >&2
    cat "$ORB_LOG" >&2
    ((++fail))
  else
    ((++pass))
  fi
}

# =============================================================================
# checkpoint create calls docker commit
# =============================================================================
: >"$ORB_LOG"
rm -f "$VERIFY_STATE"
run_ok "checkpoint create with snapshot" bash "$REPO_DIR/bin/agent" checkpoint create agent-claude-test-1234 snap-test
assert_log_contains "docker commit" "checkpoint create calls docker commit"
assert_log_contains "git stash create" "checkpoint create still runs git stash"
assert_log_contains "update-ref" "checkpoint create still runs git update-ref"
assert_output_contains "snapshot:" "checkpoint create shows snapshot tag"

# =============================================================================
# checkpoint create pauses and resumes agent via kill -STOP/-CONT
# =============================================================================
: >"$ORB_LOG"
run_ok "checkpoint create pause/resume" bash "$REPO_DIR/bin/agent" checkpoint create agent-claude-test-1234 snap-pause
assert_log_contains "tmux list-panes" "checkpoint create queries tmux for agent PID"
assert_log_contains "kill -STOP" "checkpoint create sends SIGSTOP to agent"
assert_log_contains "kill -CONT" "checkpoint create sends SIGCONT to agent"

# =============================================================================
# checkpoint create stores metadata in checkpoints.jsonl
# =============================================================================
: >"$ORB_LOG"
run_ok "checkpoint create metadata" bash "$REPO_DIR/bin/agent" checkpoint create agent-claude-test-1234 snap-meta
assert_log_contains "checkpoints.jsonl" "checkpoint create writes metadata to checkpoints.jsonl"

# =============================================================================
# checkpoint fork calls docker run with the snapshot image
# =============================================================================
: >"$ORB_LOG"
rm -f "$VERIFY_STATE"
run_ok "checkpoint fork" bash "$REPO_DIR/bin/agent" checkpoint fork agent-claude-test-1234 my-fork-agent
assert_log_contains "docker run" "checkpoint fork calls docker run"
assert_log_contains "safe-agentic-checkpoint:agent-claude-test-1234-20260410-120000" "checkpoint fork uses correct snapshot tag"
assert_log_contains "my-fork-agent" "checkpoint fork uses the new name"
assert_log_contains "docker network" "checkpoint fork creates a network"
assert_output_contains "Fork created" "checkpoint fork shows success message"
assert_output_contains "my-fork-agent" "checkpoint fork mentions new container name"

# =============================================================================
# checkpoint fork with custom label
# =============================================================================
: >"$ORB_LOG"
rm -f "$VERIFY_STATE"
run_ok "checkpoint fork with label" bash "$REPO_DIR/bin/agent" checkpoint fork agent-claude-test-1234 my-fork-2 custom-label
assert_log_contains "docker run" "checkpoint fork with label calls docker run"
assert_log_contains "custom-label" "checkpoint fork passes custom label"

# =============================================================================
# checkpoint fork without prior checkpoint fails
# =============================================================================
: >"$ORB_LOG"
echo "no-checkpoint" > "$VERIFY_STATE"
run_fails "checkpoint fork no checkpoint" bash "$REPO_DIR/bin/agent" checkpoint fork agent-claude-test-1234 fork-fail
assert_output_contains "No checkpoint found" "checkpoint fork without checkpoint shows error"

# =============================================================================
# checkpoint fork with invalid name fails
# =============================================================================
: >"$ORB_LOG"
rm -f "$VERIFY_STATE"
run_fails "checkpoint fork invalid name" bash "$REPO_DIR/bin/agent" checkpoint fork agent-claude-test-1234 "bad/name"
assert_output_contains "invalid" "checkpoint fork with invalid name shows validation error"

# =============================================================================
# checkpoint fork without new-name fails
# =============================================================================
: >"$ORB_LOG"
rm -f "$VERIFY_STATE"
run_fails "checkpoint fork no new-name" bash "$REPO_DIR/bin/agent" checkpoint fork agent-claude-test-1234
assert_output_contains "Fork name required" "checkpoint fork without new-name shows error"

# =============================================================================
# help text mentions fork
# =============================================================================
run_ok "checkpoint help mentions fork" bash "$REPO_DIR/bin/agent" checkpoint --help
assert_output_contains "fork" "checkpoint help lists fork subcommand"

# =============================================================================
# checkpoint fork --latest
# =============================================================================
: >"$ORB_LOG"
rm -f "$VERIFY_STATE"
run_ok "checkpoint fork --latest" bash "$REPO_DIR/bin/agent" checkpoint fork --latest my-fork-latest
assert_log_contains "docker run" "checkpoint fork --latest calls docker run"
assert_log_contains "my-fork-latest" "checkpoint fork --latest uses the new name"

echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
