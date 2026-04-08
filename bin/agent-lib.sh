#!/usr/bin/env bash

AGENT_LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEFAULTS_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/safe-agentic"
DEFAULTS_FILE="${SAFE_AGENTIC_DEFAULTS_FILE:-$DEFAULTS_DIR/defaults.sh}"

# shellcheck disable=SC1091
source "$AGENT_LIB_DIR/repo-url.sh"

trim_whitespace() {
  local value="$1"

  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s\n' "$value"
}

render_command() {
  local rendered=""
  local arg

  for arg in "$@"; do
    printf -v rendered '%s%q ' "$rendered" "$arg"
  done

  printf '%s\n' "${rendered% }"
}

bool_is_true() {
  case "${1:-}" in
    1|true|TRUE|yes|YES|on|ON) return 0 ;;
    *) return 1 ;;
  esac
}

load_user_defaults() {
  [ -f "$DEFAULTS_FILE" ] || return 0
  local line="" line_no=0

  while IFS= read -r line || [ -n "$line" ]; do
    line_no=$((line_no + 1))
    line="${line%$'\r'}"
    parse_defaults_line "$line" "$line_no"
  done < "$DEFAULTS_FILE"
}

defaults_error() {
  local message="$1"

  if declare -F die >/dev/null 2>&1; then
    die "$message"
  fi

  echo "$message" >&2
  return 1
}

default_key_allowed() {
  case "$1" in
    SAFE_AGENTIC_DEFAULT_CPUS|SAFE_AGENTIC_DEFAULT_DOCKER|SAFE_AGENTIC_DEFAULT_DOCKER_SOCKET|SAFE_AGENTIC_DEFAULT_IDENTITY|SAFE_AGENTIC_DEFAULT_MEMORY|SAFE_AGENTIC_DEFAULT_NETWORK|SAFE_AGENTIC_DEFAULT_PIDS_LIMIT|SAFE_AGENTIC_DEFAULT_REUSE_AUTH|SAFE_AGENTIC_DEFAULT_REUSE_GH_AUTH|SAFE_AGENTIC_DEFAULT_SSH|GIT_AUTHOR_EMAIL|GIT_AUTHOR_NAME|GIT_COMMITTER_EMAIL|GIT_COMMITTER_NAME)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

parse_defaults_value() {
  local raw

  raw=$(trim_whitespace "$1")

  case "$raw" in
    \"*\")
      raw="${raw#\"}"
      raw="${raw%\"}"
      raw="${raw//\\\"/\"}"
      raw="${raw//\\\\/\\}"
      printf '%s\n' "$raw"
      ;;
    \'*\')
      raw="${raw#\'}"
      raw="${raw%\'}"
      printf '%s\n' "$raw"
      ;;
    *[[:space:]]*)
      return 1
      ;;
    *)
      printf '%s\n' "$raw"
      ;;
  esac
}

parse_defaults_line() {
  local line="$1"
  local line_no="$2"
  local key value

  line=$(trim_whitespace "$line")
  [ -n "$line" ] || return 0
  [[ "$line" == \#* ]] && return 0

  if [[ "$line" == export[[:space:]]* ]]; then
    line=$(trim_whitespace "${line#export}")
  fi

  [[ "$line" == *=* ]] \
    || defaults_error "Unsupported line in $DEFAULTS_FILE:$line_no. Use simple KEY=value assignments only."

  key=$(trim_whitespace "${line%%=*}")
  value="${line#*=}"

  default_key_allowed "$key" \
    || defaults_error "Unsupported defaults key '$key' in $DEFAULTS_FILE:$line_no."

  value=$(parse_defaults_value "$value") \
    || defaults_error "Unsupported value for $key in $DEFAULTS_FILE:$line_no. Use KEY=value or quote the full value."

  export "$key=$value"
}

validate_name_component() {
  local value="$1"
  local label="$2"

  [[ "$value" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]] \
    || die "$label contains invalid characters: $value. Allowed: letters, numbers, ., _, -"
}

validate_pids_limit() {
  local value="$1"
  [[ "$value" =~ ^[0-9]+$ ]] || die "PIDs limit must be a positive integer: $value"
  [ "$value" -ge 64 ] || die "PIDs limit must be >= 64 (got $value). Use default $DEFAULT_PIDS_LIMIT for safety."
}

validate_network_name() {
  local value="$1"

  case "$value" in
    bridge|host|container:*)
      die "Unsafe network mode '$value' is not allowed. Create a dedicated Docker network and pass its name."
      ;;
    none)
      return 0
      ;;
  esac

  [[ "$value" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]] \
    || die "Network name contains invalid characters: $value. Allowed: letters, numbers, ., _, -"
}

parse_identity() {
  local identity="$1"
  local name email

  [[ "$identity" == *"<"*">" ]] || die "Identity must look like: Name <email@example.com>"

  name=$(trim_whitespace "${identity%<*}")
  email="${identity##*<}"
  email="${email%>}"

  [ -n "$name" ] || die "Identity name is required. Format: Name <email@example.com>"
  case "$email" in
    ""|*[[:space:]]*|*\<*|*\>*|*@|@*|*@@*)
      die "Identity email is invalid: $email"
      ;;
    *@*)
      ;;
    *)
      die "Identity email is invalid: $email"
      ;;
  esac

  printf '%s\t%s\n' "$name" "$email"
}

apply_identity() {
  local identity="$1"
  local parsed name email

  parsed=$(parse_identity "$identity")
  name="${parsed%%$'\t'*}"
  email="${parsed#*$'\t'}"

  GIT_AUTHOR_NAME="$name"
  GIT_AUTHOR_EMAIL="$email"
  GIT_COMMITTER_NAME="$name"
  GIT_COMMITTER_EMAIL="$email"
}

apply_default_identity() {
  [ -n "${SAFE_AGENTIC_DEFAULT_IDENTITY:-}" ] || return 0
  [ -n "${GIT_AUTHOR_NAME:-}" ] && [ -n "${GIT_AUTHOR_EMAIL:-}" ] && return 0
  apply_identity "$SAFE_AGENTIC_DEFAULT_IDENTITY"
}

repo_uses_ssh() {
  case "$1" in
    git@*|ssh://*) return 0 ;;
    *) return 1 ;;
  esac
}

repo_display_label() {
  local -a repos=("$@")
  local path

  case "${#repos[@]}" in
    0) printf '%s\n' "-" ;;
    1)
      path=$(repo_path_from_url "${repos[0]}" 2>/dev/null || true)
      printf '%s\n' "${path:-${repos[0]}}"
      ;;
    *)
      printf '%s\n' "${#repos[@]} repos"
      ;;
  esac
}

repo_slug_label() {
  local path

  path=$(repo_path_from_url "$1" 2>/dev/null || true)
  [ -n "$path" ] || return 1
  printf '%s\n' "${path//\//-}"
}

container_exists() {
  local container_name="$1"
  local existing

  existing=$(vm_exec docker ps -aq --filter "name=^${container_name}$" 2>/dev/null || echo "")
  [ -n "$existing" ]
}

default_container_suffix() {
  local fallback="$1"
  shift
  local -a repos=("$@")
  local slug

  case "${#repos[@]}" in
    0) printf '%s\n' "$fallback" ;;
    1)
      slug=$(repo_slug_label "${repos[0]}" 2>/dev/null || true)
      printf '%s\n' "${slug:-$fallback}"
      ;;
    *)
      slug=$(repo_slug_label "${repos[0]}" 2>/dev/null || true)
      slug="${slug:-workspace}"
      printf '%s\n' "${slug}-plus${#repos[@]}"
      ;;
  esac
}

resolve_container_name() {
  local prefix="$1"
  local explicit_name="$2"
  local fallback="$3"
  shift 3
  local -a repos=("$@")
  local suffix container_name

  if [ -n "$explicit_name" ]; then
    suffix="$explicit_name"
  else
    if [ ${#repos[@]} -gt 0 ]; then
      suffix=$(default_container_suffix "$fallback" "${repos[@]}")
    else
      suffix=$(default_container_suffix "$fallback")
    fi
  fi

  container_name="${prefix}-${suffix}"
  if container_exists "$container_name"; then
    # Remove stopped container with same name to allow reuse
    local state
    state=$(vm_exec docker inspect --format '{{.State.Status}}' "$container_name" 2>/dev/null || echo "")
    if [ "$state" = "exited" ] || [ "$state" = "created" ]; then
      vm_exec docker rm "$container_name" >/dev/null 2>&1 || true
    else
      # Running container — append timestamp to avoid conflict
      container_name="${container_name}-${fallback}"
    fi
  fi

  printf '%s\n' "$container_name"
}

resolve_latest_container() {
  vm_exec docker ps -a --latest --filter "name=^${CONTAINER_PREFIX}-" --format '{{.Names}}' 2>/dev/null || true
}

resolve_container_reference() {
  local name="$1"
  local candidate
  local -a matches=()
  local names

  if [[ "$name" == ${CONTAINER_PREFIX}-* ]]; then
    printf '%s\n' "$name"
    return 0
  fi

  case "$name" in
    claude-*|codex-*|shell-*)
      printf '%s\n' "${CONTAINER_PREFIX}-${name}"
      return 0
      ;;
  esac

  names=$(vm_exec docker ps -a --format '{{.Names}}' --filter "name=^${CONTAINER_PREFIX}-" 2>/dev/null || true)
  while IFS= read -r candidate; do
    [ -n "$candidate" ] || continue
    case "$candidate" in
      "${CONTAINER_PREFIX}-${name}"|${CONTAINER_PREFIX}-*-"$name")
        matches+=("$candidate")
        ;;
    esac
  done <<< "$names"

  case "${#matches[@]}" in
    0) printf '%s\n' "${CONTAINER_PREFIX}-${name}" ;;
    1) printf '%s\n' "${matches[0]}" ;;
    *) die "Multiple running containers match '$name'. Use full name or --latest." ;;
  esac
}

ensure_ssh_for_repos() {
  local enable_ssh="$1"
  shift
  local repo_url

  $enable_ssh && return 0

  for repo_url in "$@"; do
    if repo_uses_ssh "$repo_url"; then
      die "SSH repo detected: $repo_url. Re-run with --ssh or use agent-claude/agent-codex."
    fi
  done
}

auth_volume_exists() {
  local volume_name="$1"

  vm_exec docker volume inspect "$volume_name" >/dev/null 2>&1
}

managed_network_summary() {
  printf '%s\n' "managed: egress filtered, TCP 22/80/443 only"
}

custom_network_summary() {
  local network_name="$1"

  case "$network_name" in
    none) printf '%s\n' "custom: no network access" ;;
    *) printf '%s\n' "custom: bypasses managed egress guardrails" ;;
  esac
}

bridge_name_for_network() {
  local value="$1"
  local checksum

  checksum=$(printf '%s' "$value" | cksum | awk '{print $1}')
  printf 'sa%s\n' "$checksum"
}

network_name_for_container() {
  echo "${1}-net"
}

verify_vm_runtime_hardening() {
  # shellcheck disable=SC2016
  vm_exec bash -lc '
    set -euo pipefail
    echo safe-agentic-hardening-verify >/dev/null

    for mnt in /Users /mnt/mac /Volumes /private /opt/orbstack; do
      if [ -d "$mnt" ] && find "$mnt" -mindepth 1 -maxdepth 1 -print -quit 2>/dev/null | grep -q .; then
        echo "unsafe mount visible: $mnt" >&2
        exit 1
      fi
    done

    for cmd in open osascript code mac; do
      cmdpath=$(command -v "$cmd" 2>/dev/null || true)
      [ -z "$cmdpath" ] && continue
      if printf "%s\n" "$cmdpath" | grep -q "^/opt/orbstack-guest/"; then
        echo "unsafe integration command visible: $cmdpath" >&2
        exit 1
      fi
      if [ -L "$cmdpath" ]; then
        echo "unsafe integration symlink visible: $cmdpath" >&2
        exit 1
      fi
      if file "$cmdpath" 2>/dev/null | grep -qi "orbstack\|mac"; then
        echo "unsafe integration binary visible: $cmdpath" >&2
        exit 1
      fi
    done

    test -r /etc/safe-agentic/seccomp.json
  '
}

sync_vm_seccomp_profile() {
  vm_copy_from_host "$REPO_DIR/config/seccomp.json" /tmp/seccomp.json
  vm_exec bash -c 'install -m 0644 -D /tmp/seccomp.json /etc/safe-agentic/seccomp.json'
}

reapply_vm_runtime_hardening() {
  vm_copy_from_host "$REPO_DIR/vm/setup.sh" /tmp/setup.sh
  vm_exec bash /tmp/setup.sh
  sync_vm_seccomp_profile
}

ensure_vm_runtime_hardening() {
  info "Verifying VM hardening..."
  if verify_vm_runtime_hardening; then
    return 0
  fi

  warn "VM hardening drift detected. Re-applying safe-agentic protections..."
  reapply_vm_runtime_hardening
  verify_vm_runtime_hardening || die "VM hardening verification failed after re-apply. Fix the VM before launching agents."
}

ensure_local_image_present() {
  vm_exec docker image inspect "$IMAGE_NAME:$IMAGE_TAG" >/dev/null 2>&1 \
    || die "Image '$IMAGE_NAME:$IMAGE_TAG' not found in VM. Run 'agent update' or 'agent setup' first."
}

ensure_custom_network() {
  local network_name="$1"

  [ "$network_name" = "none" ] && return 0
  vm_exec docker network inspect "$network_name" >/dev/null 2>&1 \
    || die "Docker network '$network_name' not found in VM. Create it first or omit --network."
}

create_managed_network() {
  local network_name="$1"
  local bridge_name

  bridge_name=$(bridge_name_for_network "$network_name")

  if vm_exec docker network inspect "$network_name" >/dev/null 2>&1; then
    vm_exec docker network rm "$network_name" >/dev/null 2>&1 \
      || die "Managed network '$network_name' already exists and is busy."
  fi

  vm_exec docker network create \
    --driver bridge \
    --opt "com.docker.network.bridge.name=$bridge_name" \
    --opt com.docker.network.bridge.enable_icc=false \
    --label "app=$IMAGE_NAME" \
    --label "safe-agentic.type=container-network" \
    "$network_name" >/dev/null
}

remove_managed_network() {
  local network_name="$1"
  local network_type

  network_type=$(vm_exec docker network inspect --format '{{index .Labels "safe-agentic.type"}}' "$network_name" 2>/dev/null || echo "")
  [ "$network_type" = "container-network" ] || return 0
  vm_exec docker network rm "$network_name" >/dev/null 2>&1 || true
}

append_ephemeral_volume() {
  local destination="$1"
  docker_cmd+=(--mount "type=volume,dst=$destination")
}

append_named_volume() {
  local source="$1"
  local destination="$2"
  docker_cmd+=(--mount "type=volume,src=$source,dst=$destination")
}

append_runtime_hardening() {
  local network_name="$1"
  local memory="$2"
  local cpus="$3"
  local pids_limit="$4"
  local repo_display="$5"
  local agent_label="$6"
  local ssh_label="$7"
  local network_mode_label="$8"

  docker_cmd+=(--label "app=$IMAGE_NAME")
  docker_cmd+=(--label "safe-agentic.type=container")
  docker_cmd+=(--label "safe-agentic.agent-type=$agent_label")
  docker_cmd+=(--label "safe-agentic.repo-display=$repo_display")
  docker_cmd+=(--label "safe-agentic.ssh=$ssh_label")
  docker_cmd+=(--label "safe-agentic.network-mode=$network_mode_label")
  docker_cmd+=(--cap-drop=ALL)
  docker_cmd+=(--security-opt=no-new-privileges:true)
  docker_cmd+=(--security-opt "seccomp=/etc/safe-agentic/seccomp.json")
  docker_cmd+=(--read-only)
  docker_cmd+=(--network "$network_name")
  docker_cmd+=(--cpus "$cpus")
  docker_cmd+=(--memory "$memory")
  docker_cmd+=(--pids-limit "$pids_limit")
  docker_cmd+=(--ulimit nofile=65536:65536)
  docker_cmd+=(--tmpfs "/tmp:rw,noexec,nosuid,size=512m")
  docker_cmd+=(--tmpfs "/var/tmp:rw,noexec,nosuid,size=256m")
  docker_cmd+=(--tmpfs "/run:rw,noexec,nosuid,size=16m")
  docker_cmd+=(--tmpfs "/dev/shm:rw,noexec,nosuid,size=64m")
  append_ephemeral_volume /workspace
  docker_cmd+=(--tmpfs "/home/agent/.config:rw,noexec,nosuid,uid=1000,gid=1000,size=32m")
  docker_cmd+=(--tmpfs "/home/agent/.ssh:rw,noexec,nosuid,uid=1000,gid=1000,size=1m")
  docker_cmd+=(--tmpfs "/home/agent/.local:rw,noexec,nosuid,uid=1000,gid=1000,size=64m")
}

append_ssh_mount() {
  local enable_ssh="$1"
  local repos_joined="$2"

  if $enable_ssh; then
    local vm_ssh_sock
    # shellcheck disable=SC2016
    vm_ssh_sock=$(vm_exec bash -c 'echo $SSH_AUTH_SOCK' 2>/dev/null || echo "")
    if [ -n "$vm_ssh_sock" ]; then
      # With userns-remap the container's uid maps to an unprivileged VM uid
      # that cannot read the OrbStack SSH socket (owned florian:orbstack 660).
      # Relay via socat to a world-accessible socket so the remapped uid works.
      local relay_sock="/tmp/safe-agentic-ssh-agent.sock"
      local relay_script="/tmp/safe-agentic-ssh-relay.sh"
      # Use start-stop-daemon to fully daemonize socat. Plain nohup/&
      # keeps orb run waiting because it tracks child processes.
      # Split into separate vm_exec calls: start-stop-daemon inside
      # bash -c inside orb run causes process tracking issues.
      vm_exec bash -c "
        pkill -f 'socat.*safe-agentic-ssh-agent' 2>/dev/null || true
        rm -f '$relay_sock'
        printf '#!/bin/bash\nexec socat UNIX-LISTEN:$relay_sock,fork,mode=666 UNIX-CONNECT:$vm_ssh_sock\n' > '$relay_script'
        chmod +x '$relay_script'
      " 2>/dev/null || true
      vm_exec start-stop-daemon --start --background --exec "$relay_script" 2>/dev/null || true
      # Brief wait for the socket to appear (failure is non-fatal, fallback below)
      vm_exec bash -c "for i in 1 2 3 4 5; do [ -S '$relay_sock' ] && exit 0; sleep 0.2; done; exit 1" 2>/dev/null || true
      if vm_exec test -S "$relay_sock" 2>/dev/null; then
        docker_cmd+=(-v "$relay_sock:/run/ssh-agent.sock")
        docker_cmd+=(-e "SSH_AUTH_SOCK=/run/ssh-agent.sock")
        info "SSH agent forwarding: enabled"
      else
        warn "Failed to create SSH relay socket. Falling back to direct mount."
        docker_cmd+=(-v "$vm_ssh_sock:/run/ssh-agent.sock:ro")
        docker_cmd+=(-e "SSH_AUTH_SOCK=/run/ssh-agent.sock")
        info "SSH agent forwarding: enabled (direct, may fail with userns-remap)"
      fi
    else
      warn "No SSH_AUTH_SOCK in VM. Git SSH operations may not work."
    fi
    return
  fi

  if [ -n "$repos_joined" ] && echo "$repos_joined" | grep -qE '(^|,)(git@|ssh://)'; then
    warn "SSH repos detected but --ssh not passed. Clone will fail without SSH agent."
    warn "Re-run with --ssh to enable SSH agent forwarding."
  fi
}

inject_host_config() {
  local agent_type="$1"
  local host_config config_b64

  case "$agent_type" in
    codex)
      host_config="${CODEX_HOME:-$HOME/.codex}/config.toml"
      if [ -f "$host_config" ]; then
        # Adapt config for container: remap project trust paths to /workspace,
        # ensure sandbox settings are correct for container environment.
        config_b64=$(sed \
          -e 's|^\[projects\."/Users/[^"]*"\]|# &|' \
          -e 's|^trust_level = "trusted"|# &|' \
          -e 's|^config_file = "/Users/[^"]*"|# &|' \
          "$host_config" | {
            cat
            printf '\n[projects."/workspace"]\ntrust_level = "trusted"\n'
          } | base64)
        docker_cmd+=(-e "SAFE_AGENTIC_CODEX_CONFIG_B64=$config_b64")
        info "Host config: injected from $host_config"
        # Publish port range for MCP OAuth callbacks if OAuth MCP servers are configured.
        # OrbStack forwards published container ports to macOS localhost, so the
        # browser OAuth redirect (http://127.0.0.1:PORT/callback) reaches the container.
        if grep -q 'mcp_servers\.' "$host_config" 2>/dev/null; then
          docker_cmd+=(--label "safe-agentic.mcp-oauth=true")
        fi
      fi
      ;;
    claude)
      host_config="${CLAUDE_CONFIG_DIR:-$HOME/.claude}/settings.json"
      if [ -f "$host_config" ]; then
        config_b64=$(base64 < "$host_config")
        docker_cmd+=(-e "SAFE_AGENTIC_CLAUDE_CONFIG_B64=$config_b64")
        info "Host config: injected from $host_config"
      fi
      ;;
  esac
}

append_cache_mounts() {
  append_ephemeral_volume /home/agent/.npm
  append_ephemeral_volume /home/agent/.cache/pip
  append_ephemeral_volume /home/agent/go
  append_ephemeral_volume /home/agent/.terraform.d/plugin-cache
}

run_container() {
  local managed_network="$1"
  local network_name="$2"
  local status=0

  orb run -m "$VM_NAME" "${docker_cmd[@]}" || status=$?
  # Container persists after exit (no --rm). Network and sidecar cleanup
  # is deferred to 'agent stop' or 'agent cleanup'.
  return "$status"
}

build_container_runtime() {
  local container_name="$1"
  local agent_type="$2"
  local repos_joined="$3"
  local enable_ssh="$4"
  local network_name="$5"
  local memory="$6"
  local cpus="$7"
  local pids_limit="$8"
  local repo_display="$9"
  local network_mode_label="${10}"
  local agent_label ssh_label

  docker_cmd=(docker run -it)
  ACTIVE_CONTAINER_NAME="$container_name"
  docker_cmd+=(--pull=never)
  docker_cmd+=(--name "$container_name")
  docker_cmd+=(--hostname "$container_name")

  agent_label="${agent_type:-shell}"
  if $enable_ssh; then
    ssh_label="on"
  else
    ssh_label="off"
  fi

  [ -n "$agent_type" ] && docker_cmd+=(-e "AGENT_TYPE=$agent_type")
  [ -n "$repos_joined" ] && docker_cmd+=(-e "REPOS=$repos_joined")
  docker_cmd+=(-e "GIT_CONFIG_GLOBAL=/home/agent/.config/git/config")

  # Only forward identity when the caller explicitly exports it.
  [ -n "${GIT_AUTHOR_NAME:-}" ]    && docker_cmd+=(-e "GIT_AUTHOR_NAME=$GIT_AUTHOR_NAME")
  [ -n "${GIT_COMMITTER_NAME:-}" ] && docker_cmd+=(-e "GIT_COMMITTER_NAME=$GIT_COMMITTER_NAME")
  [ -n "${GIT_AUTHOR_EMAIL:-}" ]   && docker_cmd+=(-e "GIT_AUTHOR_EMAIL=$GIT_AUTHOR_EMAIL")
  [ -n "${GIT_COMMITTER_EMAIL:-}" ] && docker_cmd+=(-e "GIT_COMMITTER_EMAIL=$GIT_COMMITTER_EMAIL")

  append_runtime_hardening "$network_name" "$memory" "$cpus" "$pids_limit" "$repo_display" "$agent_label" "$ssh_label" "$network_mode_label"
  append_ssh_mount "$enable_ssh" "$repos_joined"
  append_cache_mounts
}

prepare_network() {
  local managed_network="$1"
  local container_name="$2"
  local custom_network_name="$3"
  local dry_run="${4:-false}"

  if $managed_network; then
    network_name=$(network_name_for_container "$container_name")
    if ! $dry_run; then
      create_managed_network "$network_name"
    fi
  else
    network_name="$custom_network_name"
    validate_network_name "$network_name"
    if ! $dry_run; then
      ensure_custom_network "$network_name"
    fi
    if [ "$network_name" != "none" ] && ! $dry_run; then
      warn "Custom network '$network_name' bypasses managed egress guardrails."
    fi
  fi
}
