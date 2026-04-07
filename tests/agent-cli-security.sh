#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"
ERR_LOG="$TMP_DIR/error.log"
VERIFY_STATE="$TMP_DIR/verify-state"
mkdir -p "$FAKE_BIN"

cat >"$FAKE_BIN/orb" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail

log_file="${TEST_ORB_LOG:?}"
verify_state="${TEST_VERIFY_STATE:?}"
cmd="${1:-}"
shift || true

case "$cmd" in
  list)
    echo "safe-agentic"
    ;;
  run)
    [ "${1:-}" = "-m" ] && shift 2
    printf '%s\n' "$*" >>"$log_file"

    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-c" ] && [ "${3:-}" = 'echo $SSH_AUTH_SOCK' ]; then
      echo "/tmp/fake-ssh.sock"
      exit 0
    fi

    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-lc" ] && [[ "${3:-}" == *safe-agentic-hardening-verify* ]]; then
      : >"$verify_state"
      exit 0
    fi

    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "network" ] && [ "${3:-}" = "inspect" ]; then
      case "${4:-}" in
        custom-net|none)
          exit 0
          ;;
        *)
          exit 1
          ;;
      esac
    fi

    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "image" ] && [ "${3:-}" = "inspect" ]; then
      exit 0
    fi

    exit 0
    ;;
  push|start|stop|create|ssh)
    ;;
  *)
    echo "unexpected orb command: $cmd" >&2
    exit 1
    ;;
esac
EOF

chmod +x "$FAKE_BIN/orb"

assert_contains() {
  local haystack="$1"
  local needle="$2"
  [[ "$haystack" == *"$needle"* ]] || {
    echo "missing expected fragment: $needle" >&2
    echo "$haystack" >&2
    exit 1
  }
}

assert_not_contains() {
  local haystack="$1"
  local needle="$2"
  [[ "$haystack" != *"$needle"* ]] || {
    echo "unexpected fragment present: $needle" >&2
    echo "$haystack" >&2
    exit 1
  }
}

run_agent() {
  : >"$ORB_LOG"
  : >"$VERIFY_STATE"
  PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" bash "$@"
}

last_docker_run() {
  grep 'docker run ' "$ORB_LOG" | tail -n 1
}

run_agent "$REPO_DIR/bin/agent" spawn claude --name review --repo https://github.com/acme/repo.git >/dev/null
spawn_run="$(last_docker_run)"
assert_contains "$spawn_run" "--cap-drop=ALL"
assert_contains "$spawn_run" "--security-opt=no-new-privileges:true"
assert_contains "$spawn_run" "--network agent-claude-review-net"
assert_contains "$spawn_run" "--mount type=volume,dst=/home/agent/.claude"
assert_contains "$spawn_run" "--memory 8g"
assert_contains "$spawn_run" "--cpus 4"
assert_not_contains "$spawn_run" "--cap-add"
assert_not_contains "$spawn_run" "src=agent-claude-auth"
assert_not_contains "$spawn_run" "DOCKER_HOST="
assert_not_contains "$spawn_run" "/home/agent/.docker"
assert_not_contains "$spawn_run" "src=agent-gh-auth"
assert_not_contains "$spawn_run" "volume-nocopy"

if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" bash "$REPO_DIR/bin/agent" spawn claude -- --privileged >"$ERR_LOG" 2>&1; then
  echo "unsafe passthrough unexpectedly accepted" >&2
  exit 1
fi
assert_contains "$(cat "$ERR_LOG")" "Unknown argument '--'."

if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" bash "$REPO_DIR/bin/agent" spawn claude --network host >"$ERR_LOG" 2>&1; then
  echo "unsafe network mode unexpectedly accepted" >&2
  exit 1
fi
assert_contains "$(cat "$ERR_LOG")" "Unsafe network mode 'host'"

run_agent "$REPO_DIR/bin/agent-claude" https://github.com/acme/repo.git >/dev/null
https_alias_run="$(last_docker_run)"
assert_not_contains "$https_alias_run" "SSH_AUTH_SOCK=/run/ssh-agent.sock"

run_agent "$REPO_DIR/bin/agent-claude" git@github.com:acme/repo.git >/dev/null
ssh_alias_run="$(last_docker_run)"
assert_contains "$ssh_alias_run" "SSH_AUTH_SOCK=/run/ssh-agent.sock"

echo "agent CLI security regression checks passed"
