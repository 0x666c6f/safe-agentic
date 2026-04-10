#!/usr/bin/env bash
# Tests that auth volumes default to reuse (shared) and can be opted out
# with --ephemeral-auth / --no-reuse-auth.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"
ERR_LOG="$TMP_DIR/error.log"
VERIFY_STATE="$TMP_DIR/verify-state"
DEFAULT_HOME="$TMP_DIR/home"
mkdir -p "$FAKE_BIN"
mkdir -p "$DEFAULT_HOME/.config/safe-agentic"

# Fake orb that logs docker commands
cat >"$FAKE_BIN/orb" <<'ORBEOF'
#!/usr/bin/env bash
set -euo pipefail
log_file="${TEST_ORB_LOG:?}"
verify_state="${TEST_VERIFY_STATE:?}"
cmd="${1:-}"
shift || true
case "$cmd" in
  list) echo "safe-agentic" ;;
  run)
    [ "${1:-}" = "-m" ] && shift 2
    printf '%s\n' "$*" >>"$log_file"
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-c" ] && [[ "${3:-}" == *SSH_AUTH_SOCK* ]]; then
      echo "/tmp/fake-ssh.sock"; exit 0
    fi
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-c" ] && [[ "${3:-}" == *socat* ]]; then
      exit 0
    fi
    if [ "${1:-}" = "test" ] && [ "${2:-}" = "-S" ]; then
      exit 0
    fi
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-lc" ] && [[ "${3:-}" == *safe-agentic-hardening-verify* ]]; then
      : >"$verify_state"
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "image" ] && [ "${3:-}" = "inspect" ]; then
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "volume" ]; then
      case "${3:-}" in
        inspect|create|rm) exit 0 ;;
      esac
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "network" ]; then
      case "${3:-}" in
        inspect) exit 1 ;;  # managed networks don't pre-exist
        create|rm) exit 0 ;;
      esac
    fi
    exit 0
    ;;
  push|start|stop|create|ssh) ;;
  *) echo "unexpected orb command: $cmd" >&2; exit 1 ;;
esac
ORBEOF
chmod +x "$FAKE_BIN/orb"

# Fake git config that returns a known identity
cat >"$FAKE_BIN/git" <<'GITEOF'
#!/usr/bin/env bash
if [ "${1:-}" = "config" ] && [ "${2:-}" = "user.name" ]; then
  echo "Test User"; exit 0
fi
if [ "${1:-}" = "config" ] && [ "${2:-}" = "user.email" ]; then
  echo "test@example.com"; exit 0
fi
exec /usr/bin/git "$@"
GITEOF
chmod +x "$FAKE_BIN/git"

pass=0
fail=0

run_agent() {
  : >"$ORB_LOG"
  : >"$VERIFY_STATE"
  PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" bash "$@"
}

last_docker_run() {
  grep 'docker run ' "$ORB_LOG" | tail -n 1
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

# =============================================================================
# Test 1: Default spawn uses shared auth volume (reuse-auth is now default)
# =============================================================================
run_agent "$REPO_DIR/bin/agent" spawn claude --name authdef --repo https://github.com/a/b.git >/dev/null 2>&1
default_run="$(last_docker_run)"
assert_contains "$default_run" "src=agent-claude-auth,dst=/home/agent/.claude" "default spawn uses shared claude auth"
assert_contains "$default_run" "src=agent-gh-auth,dst=/home/agent/.config/gh"  "default spawn uses shared gh auth"

run_agent "$REPO_DIR/bin/agent" spawn codex --name authdef-cdx --repo https://github.com/a/b.git >/dev/null 2>&1
default_codex_run="$(last_docker_run)"
assert_contains "$default_codex_run" "src=agent-codex-auth,dst=/home/agent/.codex" "default codex spawn uses shared auth"

# =============================================================================
# Test 2: --ephemeral-auth disables shared auth (uses anonymous volume)
# =============================================================================
run_agent "$REPO_DIR/bin/agent" spawn claude --ephemeral-auth --name eph --repo https://github.com/a/b.git >/dev/null 2>&1
eph_run="$(last_docker_run)"
assert_not_contains "$eph_run" "src=agent-claude-auth" "ephemeral-auth no shared claude auth"
assert_not_contains "$eph_run" "src=agent-gh-auth"     "ephemeral-auth no shared gh auth"
assert_contains "$eph_run" "--mount type=volume,dst=/home/agent/.claude" "ephemeral-auth uses anonymous claude volume"

run_agent "$REPO_DIR/bin/agent" spawn codex --ephemeral-auth --name eph-cdx --repo https://github.com/a/b.git >/dev/null 2>&1
eph_codex_run="$(last_docker_run)"
assert_not_contains "$eph_codex_run" "src=agent-codex-auth" "ephemeral-auth codex no shared auth"
assert_contains "$eph_codex_run" "--mount type=volume,dst=/home/agent/.codex" "ephemeral-auth codex uses anonymous volume"

# =============================================================================
# Test 3: --no-reuse-auth is alias for --ephemeral-auth
# =============================================================================
run_agent "$REPO_DIR/bin/agent" spawn claude --no-reuse-auth --name nra --repo https://github.com/a/b.git >/dev/null 2>&1
nra_run="$(last_docker_run)"
assert_not_contains "$nra_run" "src=agent-claude-auth" "no-reuse-auth no shared claude auth"
assert_not_contains "$nra_run" "src=agent-gh-auth"     "no-reuse-auth no shared gh auth"
assert_contains "$nra_run" "--mount type=volume,dst=/home/agent/.claude" "no-reuse-auth uses anonymous claude volume"

# =============================================================================
# Test 4: Env var override to disable default reuse still works
# =============================================================================
SAFE_AGENTIC_DEFAULT_REUSE_AUTH=false SAFE_AGENTIC_DEFAULT_REUSE_GH_AUTH=false \
  run_agent "$REPO_DIR/bin/agent" spawn claude --name envoff --repo https://github.com/a/b.git >/dev/null 2>&1
envoff_run="$(last_docker_run)"
assert_not_contains "$envoff_run" "src=agent-claude-auth" "env var disables default reuse auth"
assert_not_contains "$envoff_run" "src=agent-gh-auth"     "env var disables default reuse gh auth"

# =============================================================================
echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
