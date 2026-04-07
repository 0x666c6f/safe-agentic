#!/usr/bin/env bash

validate_name_component() {
  local value="$1"
  local label="$2"

  [[ "$value" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]] || die "$label contains invalid characters: $value"
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

  [[ "$value" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]] || die "Network name contains invalid characters: $value"
}

network_name_for_container() {
  echo "${1}-net"
}

ensure_custom_network() {
  local network_name="$1"

  [ "$network_name" = "none" ] && return 0
  vm_exec docker network inspect "$network_name" >/dev/null 2>&1 \
    || die "Docker network '$network_name' not found in VM. Create it first or omit --network."
}

create_managed_network() {
  local network_name="$1"

  if vm_exec docker network inspect "$network_name" >/dev/null 2>&1; then
    vm_exec docker network rm "$network_name" >/dev/null 2>&1 \
      || die "Managed network '$network_name' already exists and is busy."
  fi

  vm_exec docker network create \
    --driver bridge \
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

  docker_cmd+=(--label "app=$IMAGE_NAME")
  docker_cmd+=(--label "safe-agentic.type=container")
  docker_cmd+=(--cap-drop=ALL)
  docker_cmd+=(--security-opt=no-new-privileges:true)
  docker_cmd+=(--read-only)
  docker_cmd+=(--network "$network_name")
  docker_cmd+=(--cpus "$cpus")
  docker_cmd+=(--memory "$memory")
  docker_cmd+=(--pids-limit "$pids_limit")
  docker_cmd+=(--tmpfs /tmp:rw,noexec,nosuid,size=512m)
  docker_cmd+=(--tmpfs /var/tmp:rw,noexec,nosuid,size=256m)
  docker_cmd+=(--tmpfs /run:rw,noexec,nosuid,size=16m)
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
    vm_ssh_sock=$(vm_exec bash -c 'echo $SSH_AUTH_SOCK' 2>/dev/null || echo "")
    if [ -n "$vm_ssh_sock" ]; then
      docker_cmd+=(-v "$vm_ssh_sock:/run/ssh-agent.sock:ro")
      docker_cmd+=(-e "SSH_AUTH_SOCK=/run/ssh-agent.sock")
      info "SSH agent forwarding: enabled"
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
  if $managed_network; then
    remove_managed_network "$network_name"
  fi
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

  docker_cmd=(docker run -it --rm)
  docker_cmd+=(--name "$container_name")
  docker_cmd+=(--hostname "$container_name")

  [ -n "$agent_type" ] && docker_cmd+=(-e "AGENT_TYPE=$agent_type")
  [ -n "$repos_joined" ] && docker_cmd+=(-e "REPOS=$repos_joined")
  docker_cmd+=(-e "GIT_CONFIG_GLOBAL=/home/agent/.config/git/config")

  # Pass host user's git identity into the container
  local git_name git_email
  git_name=$(git config user.name 2>/dev/null || echo "")
  git_email=$(git config user.email 2>/dev/null || echo "")
  [ -n "$git_name" ]  && docker_cmd+=(-e "GIT_AUTHOR_NAME=$git_name" -e "GIT_COMMITTER_NAME=$git_name")
  [ -n "$git_email" ] && docker_cmd+=(-e "GIT_AUTHOR_EMAIL=$git_email" -e "GIT_COMMITTER_EMAIL=$git_email")

  append_runtime_hardening "$network_name" "$memory" "$cpus" "$pids_limit"
  append_ssh_mount "$enable_ssh" "$repos_joined"
  append_cache_mounts
}

prepare_network() {
  local managed_network="$1"
  local container_name="$2"
  local custom_network_name="$3"

  if $managed_network; then
    network_name=$(network_name_for_container "$container_name")
    create_managed_network "$network_name"
  else
    network_name="$custom_network_name"
    validate_network_name "$network_name"
    ensure_custom_network "$network_name"
  fi
}
