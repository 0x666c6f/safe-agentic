#!/usr/bin/env bash
# Integration test for v2 features: run, replay, dashboard, new spawn flags,
# checkpoint fork, cost --history, fleet shared_tasks, pipeline when/outputs.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

AGENT_BIN="$REPO_DIR/bin/agent"
OUT_LOG="$TMP_DIR/out.log"
ERR_LOG="$TMP_DIR/err.log"
FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"
VERIFY_STATE="$TMP_DIR/verify-state"

mkdir -p "$FAKE_BIN"

# Fake orb for dry-run testing
cat >"$FAKE_BIN/orb" <<'FAKEORB'
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
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-c" ] && [[ "${3:-}" == *SSH_AUTH_SOCK* ]]; then
      echo "/tmp/fake-ssh.sock"; exit 0
    fi
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-c" ] && [[ "${3:-}" == *'test -S /var/run/docker.sock'*'printf "%s\n" /var/run/docker.sock'* ]]; then
      echo "/var/run/docker.sock"; exit 0
    fi
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-c" ] && [[ "${3:-}" == "stat -c %g /var/run/docker.sock" ]]; then
      echo "998"; exit 0
    fi
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-lc" ] && [[ "${3:-}" == *safe-agentic-hardening-verify* ]]; then
      : >"$verify_state"
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "image" ] && [ "${3:-}" = "inspect" ]; then
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ]; then
      exit 1
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "network" ]; then
      case "${3:-}" in
        inspect) exit 1 ;;
        create|rm) exit 0 ;;
      esac
    fi
    exit 0 ;;
  push|start|stop|create|ssh) ;;
  *) exit 0 ;;
esac
FAKEORB
chmod +x "$FAKE_BIN/orb"

# Fake git for identity detection
cat >"$FAKE_BIN/git" <<'FAKEGIT'
#!/usr/bin/env bash
if [[ "$*" == *"user.name"* ]]; then echo "Test User"; exit 0; fi
if [[ "$*" == *"user.email"* ]]; then echo "test@example.com"; exit 0; fi
exec /usr/bin/git "$@"
FAKEGIT
chmod +x "$FAKE_BIN/git"

pass=0
fail=0

check() {
  local label="$1" result="$2"
  if [ "$result" = "ok" ]; then
    ((++pass))
  else
    echo "FAIL: $label" >&2
    ((++fail))
  fi
}

run_ok() {
  local label="$1"; shift
  : >"$ORB_LOG"
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
    check "$label" "ok"
  else
    check "$label" "fail"
    echo "  exit=$?" >&2
    cat "$ERR_LOG" >&2
  fi
}

run_fails() {
  local label="$1"; shift
  : >"$ORB_LOG"
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
    check "$label" "fail"
  else
    check "$label" "ok"
  fi
}

output_contains() {
  local needle="$1" label="$2"
  if grep -qF -- "$needle" "$OUT_LOG" 2>/dev/null || grep -qF -- "$needle" "$ERR_LOG" 2>/dev/null; then
    check "$label" "ok"
  else
    echo "  not found: '$needle'" >&2
    check "$label" "fail"
  fi
}

output_not_contains() {
  local needle="$1" label="$2"
  if grep -qF -- "$needle" "$OUT_LOG" 2>/dev/null || grep -qF -- "$needle" "$ERR_LOG" 2>/dev/null; then
    check "$label" "fail"
  else
    check "$label" "ok"
  fi
}

last_docker_run() {
  grep 'docker run ' "$ORB_LOG" | tail -n 1
}

# =============================================================================
# 1. agent run command — basic with --dry-run
# =============================================================================
run_ok "run-basic" bash "$AGENT_BIN" run https://github.com/org/r.git "Test prompt" --dry-run

# 2. agent run with git@ auto-enables SSH
run_ok "run-ssh-detect" bash "$AGENT_BIN" run git@github.com:org/r.git "Test prompt" --dry-run
# The dry-run info output should contain SSH_AUTH_SOCK since git@ triggers --ssh
if grep -qF "SSH_AUTH_SOCK" "$OUT_LOG" 2>/dev/null || grep -qF "SSH_AUTH_SOCK" "$ERR_LOG" 2>/dev/null; then
  check "run-ssh-has-ssh-env" "ok"
else
  check "run-ssh-has-ssh-env" "fail"
fi

# 3. agent run rejects missing repo
run_fails "run-no-repo" bash "$AGENT_BIN" run
output_contains "agent help" "run-no-repo-shows-help"

# 4. agent run rejects missing prompt
run_fails "run-no-prompt" bash "$AGENT_BIN" run https://github.com/org/r.git
output_contains "Prompt required" "run-no-prompt-error"

# 5. agent run --help works
run_ok "run-help" bash "$AGENT_BIN" run --help
output_contains "agent run" "run-help-shows-usage"

# =============================================================================
# 6. agent help includes v2 commands
# =============================================================================
bash "$AGENT_BIN" help >"$OUT_LOG" 2>"$ERR_LOG" || true
output_contains "run" "help-includes-run"
output_contains "replay" "help-includes-replay"
output_contains "dashboard" "help-includes-dashboard"

# =============================================================================
# 7. --ephemeral-auth flag recognized by spawn
# =============================================================================
run_ok "ephemeral-auth" bash "$AGENT_BIN" spawn claude --repo https://github.com/org/r.git --ephemeral-auth --dry-run

# =============================================================================
# 8. --on-complete flag recognized by spawn
# =============================================================================
run_ok "on-complete" bash "$AGENT_BIN" spawn claude --repo https://github.com/org/r.git --on-complete "echo done" --dry-run

# =============================================================================
# 9. --on-fail flag recognized by spawn
# =============================================================================
run_ok "on-fail" bash "$AGENT_BIN" spawn claude --repo https://github.com/org/r.git --on-fail "echo fail" --dry-run

# =============================================================================
# 10. --notify flag recognized by spawn
# =============================================================================
run_ok "notify" bash "$AGENT_BIN" spawn claude --repo https://github.com/org/r.git --notify terminal --dry-run

# =============================================================================
# 11. checkpoint fork in help text
# =============================================================================
bash "$AGENT_BIN" help checkpoint >"$OUT_LOG" 2>"$ERR_LOG" || true
output_contains "fork" "help-checkpoint-fork"
output_contains "create|list|revert|fork" "checkpoint-help-lists-fork"

# =============================================================================
# 12. checkpoint fork is a recognized subcommand (not an error for unknown)
# =============================================================================
# Without a container name it should complain about missing name, not "unknown subcommand"
run_fails "checkpoint-fork-subcmd" bash "$AGENT_BIN" checkpoint fork
output_not_contains "Unknown subcommand" "checkpoint-fork-is-valid-subcmd"
output_contains "Container name" "checkpoint-fork-needs-name"

# =============================================================================
# 13. replay --help works
# =============================================================================
run_ok "replay-help" bash "$AGENT_BIN" replay --help
output_contains "replay" "replay-help-output"

# =============================================================================
# 14. replay is recognized in dispatch (no "Unknown command")
# =============================================================================
run_fails "replay-no-args" bash "$AGENT_BIN" replay
output_not_contains "Unknown command" "replay-is-dispatched"

# =============================================================================
# 15. dashboard --help works
# =============================================================================
run_ok "dashboard-help" bash "$AGENT_BIN" dashboard --help
output_contains "dashboard" "dashboard-help-output"

# =============================================================================
# 16. cost --history in help
# =============================================================================
bash "$AGENT_BIN" help cost >"$OUT_LOG" 2>"$ERR_LOG" || true
output_contains "history" "help-cost-history"

# =============================================================================
# 17. cost --history flag is recognized (doesn't error)
# =============================================================================
run_ok "cost-history" bash "$AGENT_BIN" cost --history

# =============================================================================
# 18. Fleet with shared_tasks in dry-run
# =============================================================================
cat >"$TMP_DIR/fleet.yaml" <<'YAML'
name: test-fleet
shared_tasks: true
agents:
  - name: a
    type: claude
    repo: https://github.com/org/r.git
    prompt: "test"
YAML
run_ok "fleet-shared-tasks" bash "$AGENT_BIN" fleet "$TMP_DIR/fleet.yaml" --dry-run

# =============================================================================
# 19. Pipeline with when/outputs in dry-run
# =============================================================================
cat >"$TMP_DIR/pipeline.yaml" <<'YAML'
name: test-pipeline
steps:
  - name: check
    type: claude
    repo: https://github.com/org/r.git
    prompt: "test"
    outputs: "echo pass"
  - name: act
    type: claude
    repo: https://github.com/org/r.git
    prompt: "test"
    depends_on: check
    when: "pass"
YAML
run_ok "pipeline-conditions" bash "$AGENT_BIN" pipeline "$TMP_DIR/pipeline.yaml" --dry-run
# Verify the dry-run output shows the when/outputs fields
output_contains "when:" "pipeline-shows-when"
output_contains "outputs:" "pipeline-shows-outputs"

# =============================================================================
# 20. help run topic works
# =============================================================================
run_ok "help-run-topic" bash "$AGENT_BIN" help run
output_contains "agent run" "help-run-topic-output"

# =============================================================================
# 21. Spawn help shows new flags
# =============================================================================
bash "$AGENT_BIN" help spawn >"$OUT_LOG" 2>"$ERR_LOG" || true
output_contains "--on-complete" "spawn-help-on-complete"
output_contains "--on-fail" "spawn-help-on-fail"
output_contains "--notify" "spawn-help-notify"
output_contains "--ephemeral-auth" "spawn-help-ephemeral-auth"

# =============================================================================
# Summary
# =============================================================================
echo ""
echo "test-v2-integration: $pass passed, $fail failed"
[ "$fail" -eq 0 ] || exit 1
