#!/usr/bin/env bash
# Optional live smoke test against the real OrbStack VM and built image.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
VM_NAME="safe-agentic"
LIVE_AGENT_ROOT="$TMP_DIR/live-agent"
LIVE_AGENT_BIN="$LIVE_AGENT_ROOT/bin"
CURRENT_PID=""
CURRENT_CONTAINER=""
CURRENT_NETWORK=""
CURRENT_LOG=""

skip() {
  echo "SKIP: $*"
  exit 77
}

cleanup() {
  if [ -n "${CURRENT_PID:-}" ]; then
    kill "$CURRENT_PID" >/dev/null 2>&1 || true
    wait "$CURRENT_PID" >/dev/null 2>&1 || true
  fi
  if command -v orb >/dev/null 2>&1; then
    [ -n "${CURRENT_CONTAINER:-}" ] && orb run -m "$VM_NAME" docker rm -f "$CURRENT_CONTAINER" >/dev/null 2>&1 || true
    [ -n "${CURRENT_NETWORK:-}" ] && orb run -m "$VM_NAME" docker network rm "$CURRENT_NETWORK" >/dev/null 2>&1 || true
    orb run -m "$VM_NAME" docker volume rm agent-codex-auth >/dev/null 2>&1 || true
  fi
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

pass=0
fail=0

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

assert_ok() {
  local label="$1"
  shift

  if "$@" >/dev/null 2>&1; then
    ((++pass))
  else
    echo "FAIL: $label" >&2
    ((++fail))
  fi
}

prepare_live_agent_cli() {
  local image_tag="$1"

  mkdir -p "$LIVE_AGENT_BIN"
  cp "$REPO_DIR/bin/agent-lib.sh" "$LIVE_AGENT_BIN/agent-lib.sh"
  cp "$REPO_DIR/bin/docker-runtime.sh" "$LIVE_AGENT_BIN/docker-runtime.sh"
  cp "$REPO_DIR/bin/repo-url.sh" "$LIVE_AGENT_BIN/repo-url.sh"
  sed "s/^IMAGE_TAG=.*/IMAGE_TAG=\"$image_tag\"/" "$REPO_DIR/bin/agent" >"$LIVE_AGENT_BIN/agent"
  chmod +x "$LIVE_AGENT_BIN/agent"
}

start_spawn_session() {
  local suffix="$1"
  shift

  CURRENT_CONTAINER="agent-codex-$suffix"
  CURRENT_NETWORK="${CURRENT_CONTAINER}-net"
  CURRENT_LOG="$TMP_DIR/${suffix}.log"
  CURRENT_PID=""

  script -q /dev/null bash "$LIVE_AGENT_BIN/agent" spawn codex --name "$suffix" "$@" >"$CURRENT_LOG" 2>&1 &
  CURRENT_PID=$!

  local ready=false
  local _i
  for _i in $(seq 1 40); do
    if orb run -m "$VM_NAME" docker inspect "$CURRENT_CONTAINER" >/dev/null 2>&1; then
      ready=true
      break
    fi
    if ! kill -0 "$CURRENT_PID" >/dev/null 2>&1; then
      break
    fi
    sleep 0.5
  done

  if ! $ready; then
    echo "FAIL: live spawn did not create $CURRENT_CONTAINER" >&2
    [ -f "$CURRENT_LOG" ] && cat "$CURRENT_LOG" >&2
    ((++fail))
    return 1
  fi
}

stop_spawn_session() {
  if [ -n "${CURRENT_PID:-}" ]; then
    kill "$CURRENT_PID" >/dev/null 2>&1 || true
    wait "$CURRENT_PID" >/dev/null 2>&1 || true
    CURRENT_PID=""
  fi
  [ -n "${CURRENT_CONTAINER:-}" ] && orb run -m "$VM_NAME" docker rm -f "$CURRENT_CONTAINER" >/dev/null 2>&1 || true
  [ -n "${CURRENT_NETWORK:-}" ] && orb run -m "$VM_NAME" docker network rm "$CURRENT_NETWORK" >/dev/null 2>&1 || true
  CURRENT_CONTAINER=""
  CURRENT_NETWORK=""
  CURRENT_LOG=""
}

wait_for_container_gone() {
  local name="$1"
  local _i

  for _i in $(seq 1 20); do
    if ! orb run -m "$VM_NAME" docker inspect "$name" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  return 1
}

wait_for_network_gone() {
  local name="$1"
  local _i

  for _i in $(seq 1 20); do
    if ! orb run -m "$VM_NAME" docker network inspect "$name" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  return 1
}

[ "${SAFE_AGENTIC_SKIP_LIVE:-}" = "1" ] && skip "SAFE_AGENTIC_SKIP_LIVE=1"
command -v orb >/dev/null 2>&1 || skip "orb not installed"
command -v script >/dev/null 2>&1 || skip "script utility not installed"
orb list 2>/dev/null | awk '{print $1}' | grep -qx "$VM_NAME" || skip "VM '$VM_NAME' not found"
orb run -m "$VM_NAME" docker info >/dev/null 2>&1 || skip "docker unavailable in VM"

IMAGE_NAME=""
for candidate in safe-agentic:validation safe-agentic:latest; do
  if orb run -m "$VM_NAME" docker image inspect "$candidate" >/dev/null 2>&1; then
    IMAGE_NAME="$candidate"
    break
  fi
done
[ -n "$IMAGE_NAME" ] || skip "no built safe-agentic image found"
prepare_live_agent_cli "${IMAGE_NAME#*:}"

security_options="$(orb run -m "$VM_NAME" docker info --format '{{json .SecurityOptions}}')"
assert_contains "$security_options" "name=userns" "docker userns remap active"
assert_ok "macOS /Users blocked in VM" orb run -m "$VM_NAME" bash -lc '! ls -A /Users 2>/dev/null | grep -q .'
assert_ok "macOS /mnt/mac blocked in VM" orb run -m "$VM_NAME" bash -lc '! ls -A /mnt/mac 2>/dev/null | grep -q .'
assert_ok "open unavailable in VM" orb run -m "$VM_NAME" bash -lc '! command -v open >/dev/null 2>&1'
assert_ok "osascript unavailable in VM" orb run -m "$VM_NAME" bash -lc '! command -v osascript >/dev/null 2>&1'

image_smoke="$(orb run -m "$VM_NAME" docker run --rm --entrypoint bash "$IMAGE_NAME" -lc \
  'id;
   command -v claude codex aws helm eza zoxide yq delta node npm pnpm bun terraform docker gh >/dev/null;
   test -f /home/agent/.ssh.baked/known_hosts;
   test -f /home/agent/.ssh.baked/config')"
assert_contains "$image_smoke" "uid=1000(agent) gid=1000(agent)" "image runs as agent user"
assert_contains "$image_smoke" "groups=1000(agent)" "agent has no extra groups"

assert_ok "entrypoint smoke completed" \
  orb run -m "$VM_NAME" docker run --rm \
    --read-only \
    --tmpfs /tmp:rw,noexec,nosuid,size=64m \
    --tmpfs /var/tmp:rw,noexec,nosuid,size=32m \
    --tmpfs /run:rw,noexec,nosuid,size=8m \
    --tmpfs /home/agent/.config:rw,noexec,nosuid,uid=1000,gid=1000,size=8m \
    --tmpfs /home/agent/.ssh:rw,noexec,nosuid,uid=1000,gid=1000,size=1m \
    "$IMAGE_NAME" -lc \
    'git config --global user.name | grep -x Agent >/dev/null;
     git config --global user.email | grep -x agent@localhost >/dev/null;
     test -f /home/agent/.config/git/config;
     test -f /home/agent/.ssh/known_hosts;
     test -f /home/agent/.ssh/config;
     pwd | grep -x /workspace >/dev/null'

spawn_suffix="live-inspect-$$"
if start_spawn_session "$spawn_suffix"; then
  cap_drop="$(orb run -m "$VM_NAME" docker inspect --format '{{json .HostConfig.CapDrop}}' "$CURRENT_CONTAINER")"
  security_opt="$(orb run -m "$VM_NAME" docker inspect --format '{{json .HostConfig.SecurityOpt}}' "$CURRENT_CONTAINER")"
  readonly_rootfs="$(orb run -m "$VM_NAME" docker inspect --format '{{.HostConfig.ReadonlyRootfs}}' "$CURRENT_CONTAINER")"
  memory_limit="$(orb run -m "$VM_NAME" docker inspect --format '{{.HostConfig.Memory}}' "$CURRENT_CONTAINER")"
  nano_cpus="$(orb run -m "$VM_NAME" docker inspect --format '{{.HostConfig.NanoCpus}}' "$CURRENT_CONTAINER")"
  pids_limit="$(orb run -m "$VM_NAME" docker inspect --format '{{.HostConfig.PidsLimit}}' "$CURRENT_CONTAINER")"
  network_name="$(orb run -m "$VM_NAME" docker inspect --format '{{range $k, $_ := .NetworkSettings.Networks}}{{$k}}{{end}}' "$CURRENT_CONTAINER")"
  ssh_mount="$(orb run -m "$VM_NAME" docker inspect --format '{{range .Mounts}}{{if eq .Destination "/run/ssh-agent.sock"}}present{{end}}{{end}}' "$CURRENT_CONTAINER")"
  env_list="$(orb run -m "$VM_NAME" docker inspect --format '{{json .Config.Env}}' "$CURRENT_CONTAINER")"
  codex_mount_name="$(orb run -m "$VM_NAME" docker inspect --format '{{range .Mounts}}{{if eq .Destination "/home/agent/.codex"}}{{.Name}}{{end}}{{end}}' "$CURRENT_CONTAINER")"
  workspace_mount_name="$(orb run -m "$VM_NAME" docker inspect --format '{{range .Mounts}}{{if eq .Destination "/workspace"}}{{.Name}}{{end}}{{end}}' "$CURRENT_CONTAINER")"

  assert_contains "$cap_drop" "\"ALL\"" "spawn drops all caps"
  assert_contains "$security_opt" "no-new-privileges:true" "spawn no-new-privileges"
  assert_contains "$readonly_rootfs" "true" "spawn readonly rootfs"
  assert_contains "$memory_limit" "8589934592" "spawn default memory limit"
  assert_contains "$nano_cpus" "4000000000" "spawn default cpu limit"
  assert_contains "$pids_limit" "512" "spawn default pids limit"
  assert_contains "$network_name" "$CURRENT_NETWORK" "spawn dedicated network"
  assert_ok "spawn has no SSH mount by default" test -z "$ssh_mount"
  assert_ok "spawn has no SSH_AUTH_SOCK env by default" bash -lc '[[ "$1" != *SSH_AUTH_SOCK=* ]]' _ "$env_list"
  assert_ok "spawn has no host git identity env by default" bash -lc '[[ "$1" != *GIT_AUTHOR_NAME=* && "$1" != *GIT_AUTHOR_EMAIL=* && "$1" != *GIT_COMMITTER_NAME=* && "$1" != *GIT_COMMITTER_EMAIL=* ]]' _ "$env_list"
  assert_ok "spawn auth mount is anonymous by default" bash -lc '[ -n "$1" ] && [ "$1" != "agent-codex-auth" ]' _ "$codex_mount_name"
  assert_ok "spawn workspace mount is anonymous" test -n "$workspace_mount_name"

  assert_ok "agent stop succeeds" bash "$LIVE_AGENT_BIN/agent" stop "$CURRENT_CONTAINER"
  assert_ok "agent stop removes container" wait_for_container_gone "$CURRENT_CONTAINER"
  assert_ok "agent stop removes managed network" wait_for_network_gone "$CURRENT_NETWORK"

  stop_spawn_session
fi

spawn_suffix="live-reuse-$$"
if start_spawn_session "$spawn_suffix" --reuse-auth; then
  codex_mount_name="$(orb run -m "$VM_NAME" docker inspect --format '{{range .Mounts}}{{if eq .Destination "/home/agent/.codex"}}{{.Name}}{{end}}{{end}}' "$CURRENT_CONTAINER")"
  assert_contains "$codex_mount_name" "agent-codex-auth" "reuse-auth uses named volume"
  stop_spawn_session
fi

echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
