#!/usr/bin/env bash
# Tests for agent run (one-command quick start).
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"
OUT_LOG="$TMP_DIR/out.log"
ERR_LOG="$TMP_DIR/err.log"
VERIFY_STATE="$TMP_DIR/verify-state"
mkdir -p "$FAKE_BIN"

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
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "network" ]; then
      case "${3:-}" in
        inspect) exit 1 ;;
        create|rm) exit 0 ;;
      esac
    fi
    exit 0 ;;
  push|start|stop|create|ssh) ;;
  *) echo "unexpected orb command: $cmd" >&2; exit 1 ;;
esac
ORBEOF
chmod +x "$FAKE_BIN/orb"

# Fake git to provide deterministic identity
cat >"$FAKE_BIN/git" <<'GITEOF'
#!/usr/bin/env bash
if [ "${1:-}" = "config" ] && [ "${2:-}" = "--global" ] && [ "${3:-}" = "user.name" ]; then echo "Test User"; exit 0; fi
if [ "${1:-}" = "config" ] && [ "${2:-}" = "--global" ] && [ "${3:-}" = "user.email" ]; then echo "test@example.com"; exit 0; fi
if [ "${1:-}" = "config" ] && [ "${2:-}" = "user.name" ]; then echo "Test User"; exit 0; fi
if [ "${1:-}" = "config" ] && [ "${2:-}" = "user.email" ]; then echo "test@example.com"; exit 0; fi
exec /usr/bin/git "$@"
GITEOF
chmod +x "$FAKE_BIN/git"

pass=0
fail=0

run_ok() {
  local label="$1"; shift
  : >"$ORB_LOG"
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
  : >"$ORB_LOG"
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
    echo "  stdout: $(cat "$OUT_LOG")" >&2
    echo "  stderr: $(cat "$ERR_LOG")" >&2
    ((++fail))
  fi
}

assert_output_not_contains() {
  local needle="$1" label="$2"
  if grep -q -- "$needle" "$OUT_LOG" "$ERR_LOG" 2>/dev/null; then
    echo "FAIL: $label: '$needle' unexpectedly in output" >&2
    echo "  stdout: $(cat "$OUT_LOG")" >&2
    echo "  stderr: $(cat "$ERR_LOG")" >&2
    ((++fail))
  else
    ((++pass))
  fi
}

assert_log_contains() {
  local needle="$1" label="$2"
  if grep -q -- "$needle" "$ORB_LOG" 2>/dev/null; then
    ((++pass))
  else
    echo "FAIL: $label: '$needle' not in orb log" >&2
    echo "  log: $(cat "$ORB_LOG")" >&2
    ((++fail))
  fi
}

assert_log_not_contains() {
  local needle="$1" label="$2"
  if grep -q -- "$needle" "$ORB_LOG" 2>/dev/null; then
    echo "FAIL: $label: '$needle' unexpectedly in orb log" >&2
    echo "  log: $(cat "$ORB_LOG")" >&2
    ((++fail))
  else
    ((++pass))
  fi
}

AGENT_BIN="$REPO_DIR/bin/agent"

# =============================================================================
# Help
# =============================================================================
run_ok "run --help" bash "$AGENT_BIN" run --help
assert_output_contains "agent run" "run help shows command"
assert_output_contains "prompt" "run help mentions prompt"
assert_output_contains "dry-run" "run help mentions --dry-run"

run_ok "help run topic" bash "$AGENT_BIN" help run
assert_output_contains "agent run" "help run topic shows command"
assert_output_contains "Smart defaults" "help run mentions smart defaults"

# =============================================================================
# Failures: no repo, no prompt
# =============================================================================
run_fails "run no args" bash "$AGENT_BIN" run
assert_output_contains "agent help run" "no-args shows usage pointer"

run_fails "run only repo no prompt" bash "$AGENT_BIN" run git@github.com:org/repo.git
assert_output_contains "agent help run" "single-arg shows usage pointer"

run_fails "run only prompt no repo" bash "$AGENT_BIN" run "Fix tests"
assert_output_contains "agent help run" "single-arg prompt-only shows usage pointer"

# =============================================================================
# SSH auto-detection with git@ URL
# =============================================================================
run_ok "run SSH repo dry-run" bash "$AGENT_BIN" run --dry-run git@github.com:org/repo.git "Fix tests"
assert_output_contains "--ssh" "SSH auto-detected for git@ URL"
assert_output_contains "--reuse-auth" "reuse-auth is default"
assert_output_contains "Fix tests" "prompt passed through"
assert_output_contains "--identity" "git identity auto-detected"

# =============================================================================
# HTTPS URL: no SSH
# =============================================================================
run_ok "run HTTPS repo dry-run" bash "$AGENT_BIN" run --dry-run https://github.com/org/repo.git "Add feature"
assert_output_not_contains "--ssh" "no SSH for HTTPS URL"
assert_output_contains "--reuse-auth" "reuse-auth still set for HTTPS"
assert_output_contains "Add feature" "prompt passed for HTTPS"

# =============================================================================
# ssh:// URL scheme triggers SSH
# =============================================================================
run_ok "run ssh:// URL dry-run" bash "$AGENT_BIN" run --dry-run ssh://git@github.com/org/repo.git "Refactor"
assert_output_contains "--ssh" "SSH auto-detected for ssh:// URL"

# =============================================================================
# --codex flag
# =============================================================================
run_ok "run --codex dry-run" bash "$AGENT_BIN" run --codex --dry-run git@github.com:org/repo.git "Fix lint"
assert_output_contains "codex" "codex agent type used"

# =============================================================================
# --ephemeral-auth disables reuse-auth
# =============================================================================
run_ok "run --ephemeral-auth dry-run" bash "$AGENT_BIN" run --ephemeral-auth --dry-run https://github.com/org/repo.git "Test"
assert_output_not_contains "--reuse-auth" "ephemeral-auth disables reuse-auth"

# =============================================================================
# Multiple repos
# =============================================================================
run_ok "run multiple repos dry-run" bash "$AGENT_BIN" run --dry-run \
  git@github.com:org/a.git git@github.com:org/b.git "Align APIs"
assert_output_contains "org/a.git" "first repo present"
assert_output_contains "org/b.git" "second repo present"
assert_output_contains "--ssh" "SSH detected across multiple repos"
assert_output_contains "Align APIs" "prompt with multiple repos"

# =============================================================================
# Multiple repos, only one SSH
# =============================================================================
run_ok "run mixed repos dry-run" bash "$AGENT_BIN" run --dry-run \
  https://github.com/org/a.git git@github.com:org/b.git "Fix"
assert_output_contains "--ssh" "SSH enabled when any repo is SSH"

# =============================================================================
# --name flag
# =============================================================================
run_ok "run --name dry-run" bash "$AGENT_BIN" run --name my-task --dry-run git@github.com:org/repo.git "Work"
assert_output_contains "--name" "name flag forwarded"
assert_output_contains "my-task" "name value forwarded"

# =============================================================================
# --background flag
# =============================================================================
run_ok "run --background dry-run" bash "$AGENT_BIN" run --background --dry-run git@github.com:org/repo.git "Work"
assert_output_contains "--background" "background flag forwarded"

# =============================================================================
# --max-cost flag
# =============================================================================
run_ok "run --max-cost dry-run" bash "$AGENT_BIN" run --max-cost 5 --dry-run git@github.com:org/repo.git "Work"
assert_output_contains "--max-cost" "max-cost flag forwarded"

# =============================================================================
# Unknown flag fails
# =============================================================================
run_fails "run unknown flag" bash "$AGENT_BIN" run --bogus git@github.com:org/repo.git "Prompt"
assert_output_contains "agent help run" "unknown flag shows usage pointer"

# =============================================================================
echo ""
echo "test-run-command: $pass passed, $fail failed"
exit "$fail"
