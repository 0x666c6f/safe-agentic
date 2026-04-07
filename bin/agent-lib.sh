#!/usr/bin/env bash

validate_name_component() {
  local value="$1"
  local label="$2"

  [[ "$value" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]] || die "$label contains invalid characters: $value"
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

  [[ "$value" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]] || die "Network name contains invalid characters: $value"
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
  vm_exec bash -lc '
    set -euo pipefail
    echo safe-agentic-hardening-verify >/dev/null

    for mnt in /Users /mnt/mac /Volumes /private /opt/orbstack; do
      if [ -d "$mnt" ] && ls -A "$mnt" 2>/dev/null | grep -q .; then
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

  docker_cmd+=(--label "app=$IMAGE_NAME")
  docker_cmd+=(--label "safe-agentic.type=container")
  docker_cmd+=(--cap-drop=ALL)
  docker_cmd+=(--security-opt=no-new-privileges:true)
  docker_cmd+=(--security-opt "seccomp=/etc/safe-agentic/seccomp.json")
  docker_cmd+=(--read-only)
  docker_cmd+=(--network "$network_name")
  docker_cmd+=(--cpus "$cpus")
  docker_cmd+=(--memory "$memory")
  docker_cmd+=(--pids-limit "$pids_limit")
  docker_cmd+=(--ulimit nofile=65536:65536)
  docker_cmd+=(--tmpfs /tmp:rw,noexec,nosuid,size=512m)
  docker_cmd+=(--tmpfs /var/tmp:rw,noexec,nosuid,size=256m)
  docker_cmd+=(--tmpfs /run:rw,noexec,nosuid,size=16m)
  docker_cmd+=(--tmpfs /dev/shm:rw,noexec,nosuid,size=64m)
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
  docker_cmd+=(--pull=never)
  docker_cmd+=(--name "$container_name")
  docker_cmd+=(--hostname "$container_name")

  [ -n "$agent_type" ] && docker_cmd+=(-e "AGENT_TYPE=$agent_type")
  [ -n "$repos_joined" ] && docker_cmd+=(-e "REPOS=$repos_joined")
  docker_cmd+=(-e "GIT_CONFIG_GLOBAL=/home/agent/.config/git/config")

  # Only forward identity when the caller explicitly exports it.
  [ -n "${GIT_AUTHOR_NAME:-}" ]    && docker_cmd+=(-e "GIT_AUTHOR_NAME=$GIT_AUTHOR_NAME")
  [ -n "${GIT_COMMITTER_NAME:-}" ] && docker_cmd+=(-e "GIT_COMMITTER_NAME=$GIT_COMMITTER_NAME")
  [ -n "${GIT_AUTHOR_EMAIL:-}" ]   && docker_cmd+=(-e "GIT_AUTHOR_EMAIL=$GIT_AUTHOR_EMAIL")
  [ -n "${GIT_COMMITTER_EMAIL:-}" ] && docker_cmd+=(-e "GIT_COMMITTER_EMAIL=$GIT_COMMITTER_EMAIL")

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
    if [ "$network_name" != "none" ]; then
      warn "Custom network '$network_name' bypasses managed egress guardrails."
    fi
  fi
}
