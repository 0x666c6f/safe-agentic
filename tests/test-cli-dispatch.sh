#!/usr/bin/env bash
# Tests for top-level CLI dispatch, help output, error handling, and alias scripts.
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
  if grep -q "$needle" "$OUT_LOG" "$ERR_LOG" 2>/dev/null; then
    ((++pass))
  else
    echo "FAIL: $label: '$needle' not in output" >&2
    ((++fail))
  fi
}

last_docker_run() {
  grep 'docker run ' "$ORB_LOG" | tail -n 1
}

assert_contains() {
  local haystack="$1" needle="$2" label="${3:-}"
  if [[ "$haystack" == *"$needle"* ]]; then ((++pass)); else
    echo "FAIL${label:+: $label}: missing '$needle'" >&2; ((++fail))
  fi
}

assert_not_contains() {
  local haystack="$1" needle="$2" label="${3:-}"
  if [[ "$haystack" != *"$needle"* ]]; then ((++pass)); else
    echo "FAIL${label:+: $label}: unexpected '$needle'" >&2; ((++fail))
  fi
}

# =============================================================================
# Help and dispatch
# =============================================================================
run_ok   "help command"     bash "$REPO_DIR/bin/agent" help
assert_output_contains "safe-agentic" "help shows project name"

run_ok   "-h flag"          bash "$REPO_DIR/bin/agent" -h
run_ok   "--help flag"      bash "$REPO_DIR/bin/agent" --help
run_ok   "no args = help"   bash "$REPO_DIR/bin/agent"

run_fails "unknown command"  bash "$REPO_DIR/bin/agent" bogus
assert_output_contains "Unknown command" "unknown command error message"

run_fails "attach no name"   bash "$REPO_DIR/bin/agent" attach
assert_output_contains "Usage: agent attach" "attach usage"

run_fails "stop no arg"      bash "$REPO_DIR/bin/agent" stop
assert_output_contains "Usage: agent stop" "stop usage"

run_fails "vm no subcommand" bash "$REPO_DIR/bin/agent" vm
assert_output_contains "Usage: agent vm" "vm usage"

run_fails "update bad flag"  bash "$REPO_DIR/bin/agent" update --bogus
assert_output_contains "Usage: agent update" "update usage"

# =============================================================================
# agent-codex alias: correct agent type and SSH detection
# =============================================================================
: >"$ORB_LOG"
run_ok "codex alias https" bash "$REPO_DIR/bin/agent-codex" https://github.com/acme/repo.git
codex_https="$(last_docker_run)"
assert_contains "$codex_https" "AGENT_TYPE=codex"       "codex alias sets codex type"
assert_not_contains "$codex_https" "SSH_AUTH_SOCK"       "codex alias https no ssh"

: >"$ORB_LOG"
run_ok "codex alias ssh" bash "$REPO_DIR/bin/agent-codex" git@github.com:acme/repo.git
codex_ssh="$(last_docker_run)"
assert_contains "$codex_ssh" "AGENT_TYPE=codex"          "codex alias ssh sets codex type"
assert_contains "$codex_ssh" "SSH_AUTH_SOCK"             "codex alias ssh enables ssh"

# =============================================================================
# agent-claude alias: SSH detection for ssh:// prefix
# =============================================================================
: >"$ORB_LOG"
run_ok "claude alias ssh://" bash "$REPO_DIR/bin/agent-claude" ssh://git@github.com/acme/repo.git
claude_sshproto="$(last_docker_run)"
assert_contains "$claude_sshproto" "SSH_AUTH_SOCK"       "claude alias detects ssh:// prefix"

# =============================================================================
# Aliases with multiple repos
# =============================================================================
: >"$ORB_LOG"
run_ok "claude multi-repo" bash "$REPO_DIR/bin/agent-claude" https://github.com/a/one.git https://github.com/a/two.git
multi_run="$(last_docker_run)"
assert_contains "$multi_run" "REPOS=https://github.com/a/one.git,https://github.com/a/two.git" "multi repos joined"
assert_not_contains "$multi_run" "SSH_AUTH_SOCK" "multi https repos no ssh"

# Mixed SSH + HTTPS enables SSH
: >"$ORB_LOG"
run_ok "claude mixed repos" bash "$REPO_DIR/bin/agent-claude" https://github.com/a/one.git git@github.com:a/two.git
mixed_run="$(last_docker_run)"
assert_contains "$mixed_run" "SSH_AUTH_SOCK" "mixed repos enable ssh"

# =============================================================================
# Alias with no args
# =============================================================================
run_fails "claude alias no args" bash "$REPO_DIR/bin/agent-claude"
run_fails "codex alias no args"  bash "$REPO_DIR/bin/agent-codex"

echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
