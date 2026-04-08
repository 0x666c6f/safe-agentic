#!/usr/bin/env bash
# Command-level tests for attach/stop/cleanup/update-adjacent agent behavior.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"
ERR_LOG="$TMP_DIR/error.log"
VERIFY_STATE="$TMP_DIR/verify-state"
mkdir -p "$FAKE_BIN"

cat >"$FAKE_BIN/orb" <<'ORBEOF'
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

    case "$*" in
      "docker network inspect custom-net")
        exit 0
        ;;
      "docker image inspect safe-agentic:latest")
        exit 0
        ;;
      "docker ps -q --filter name=^agent-"*)
        printf 'cid-one\ncid-two\n'
        exit 0
        ;;
      "docker ps -a -q --filter name=^agent-"*)
        printf 'cid-one\ncid-two\n'
        exit 0
        ;;
      "docker ps -a --format {{.Names}} --filter name=^agent-"*)
        printf 'agent-one\nagent-two\n'
        exit 0
        ;;
      "docker ps --format {{.Names}} --filter name=^agent-"*)
        printf 'agent-one\nagent-two\n'
        exit 0
        ;;
      "docker ps -aq --filter name=^agent-")
        printf 'cid-one\ncid-two\n'
        exit 0
        ;;
      "docker volume ls -q")
        printf 'agent-claude-auth\nagent-codex-auth\nagent-gh-auth\n'
        exit 0
        ;;
      "docker network ls -q --filter label=app=safe-agentic --filter label=safe-agentic.type=container-network")
        printf 'net-a\nnet-b\n'
        exit 0
        ;;
    esac

    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-lc" ] && [[ "${3:-}" == *safe-agentic-hardening-verify* ]]; then
      : >"$verify_state"
      exit 0
    fi

    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ] && [[ "${4:-}" == *State.Status* ]]; then
      echo "running"
      exit 0
    fi

    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "network" ] && [ "${3:-}" = "inspect" ] && [ "${4:-}" = "--format" ]; then
      case "${6:-}" in
        agent-stopme-net|agent-one-net|agent-two-net)
          echo "container-network"
          exit 0
          ;;
        *)
          exit 1
          ;;
      esac
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
ORBEOF
chmod +x "$FAKE_BIN/orb"

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

last_docker_run() {
  grep 'docker run ' "$ORB_LOG" | tail -n 1
}

# --- attach uses login shell ---
run_agent "$REPO_DIR/bin/agent" attach claude-review >/dev/null 2>&1
attach_log="$(cat "$ORB_LOG")"
assert_contains "$attach_log" "docker exec -it agent-claude-review bash -l" "attach login shell"

# --- stop removes matching managed network ---
run_agent "$REPO_DIR/bin/agent" stop stopme >/dev/null 2>&1
stop_log="$(cat "$ORB_LOG")"
assert_contains "$stop_log" "docker stop agent-stopme" "stop single container"
assert_contains "$stop_log" "docker rm agent-stopme" "stop removes single container"
assert_contains "$stop_log" "docker network rm agent-stopme-net" "stop removes single network"

# --- stop --all removes networks for each running agent ---
run_agent "$REPO_DIR/bin/agent" stop --all >/dev/null 2>&1
stop_all_log="$(cat "$ORB_LOG")"
assert_contains "$stop_all_log" "docker stop cid-one cid-two" "stop all containers"
assert_contains "$stop_all_log" "docker rm cid-one cid-two" "stop all removes containers"
assert_contains "$stop_all_log" "docker network rm agent-one-net" "stop all first network"
assert_contains "$stop_all_log" "docker network rm agent-two-net" "stop all second network"

# --- cleanup removes stopped containers, keeps shared auth by default, prunes image layers ---
run_agent "$REPO_DIR/bin/agent" cleanup >/dev/null 2>&1
cleanup_log="$(cat "$ORB_LOG")"
assert_contains "$cleanup_log" "docker stop cid-one cid-two" "cleanup stop running"
assert_contains "$cleanup_log" "docker rm cid-one cid-two" "cleanup remove stopped"
assert_not_contains "$cleanup_log" "docker volume rm agent-claude-auth agent-codex-auth" "cleanup keeps auth volumes"
assert_contains "$cleanup_log" "docker network rm net-a net-b" "cleanup managed networks"
assert_contains "$cleanup_log" "docker image prune -f --filter label=app=safe-agentic" "cleanup image prune"

# --- cleanup --auth also removes shared auth volumes ---
run_agent "$REPO_DIR/bin/agent" cleanup --auth >/dev/null 2>&1
cleanup_auth_log="$(cat "$ORB_LOG")"
assert_contains "$cleanup_auth_log" "docker volume rm agent-claude-auth agent-codex-auth agent-gh-auth" "cleanup auth volumes"

# --- custom network uses existing network and skips managed create ---
run_agent "$REPO_DIR/bin/agent" spawn claude --name custom --network custom-net --repo https://github.com/acme/repo.git >/dev/null 2>&1
custom_log="$(cat "$ORB_LOG")"
custom_run="$(last_docker_run)"
assert_contains "$custom_log" "docker network inspect custom-net" "custom network inspected"
assert_contains "$custom_run" "--network custom-net" "custom network used"
assert_not_contains "$custom_log" "docker network create" "no managed network create for custom network"

# --- network=none is accepted and also skips managed create ---
run_agent "$REPO_DIR/bin/agent" spawn claude --name none --network none --repo https://github.com/acme/repo.git >/dev/null 2>&1
none_log="$(cat "$ORB_LOG")"
none_run="$(last_docker_run)"
assert_contains "$none_run" "--network none" "network none used"
assert_not_contains "$none_log" "docker network create" "no managed network create for none"

# --- shell gets no agent auth volume by default (tmpfs is OK) ---
run_agent "$REPO_DIR/bin/agent" shell --network custom-net >/dev/null 2>&1
shell_run="$(last_docker_run)"
assert_not_contains "$shell_run" "--mount type=volume,dst=/home/agent/.claude" "shell no claude auth volume"
assert_not_contains "$shell_run" "--mount type=volume,dst=/home/agent/.codex" "shell no codex auth volume"

# --- docker sidecar is started and cleanup deferred to stop/cleanup ---
run_agent "$REPO_DIR/bin/agent" spawn claude --docker --name docky --repo https://github.com/acme/repo.git >/dev/null 2>&1
docker_log="$(cat "$ORB_LOG")"
assert_contains "$docker_log" "docker run -d --name safe-agentic-docker-agent-claude-docky" "docker sidecar started"
assert_contains "$docker_log" "docker rm -f safe-agentic-docker-agent-claude-docky" "docker sidecar pre-start cleanup"
assert_not_contains "$docker_log" "docker volume rm agent-claude-docky-docker-sock" "docker sidecar volume cleanup deferred"

echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
