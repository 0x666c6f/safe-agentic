#!/usr/bin/env bash

DOCKER_RUNTIME_KIND="off"
DOCKER_RUNTIME_CONTAINER=""
DOCKER_RUNTIME_SOCKET_VOLUME=""
DOCKER_RUNTIME_DATA_VOLUME=""

DOCKER_HOST_SOCKET_PATH="/run/docker-host.sock"
DOCKER_INTERNAL_SOCKET_DIR="/run/safe-agentic-docker"
DOCKER_INTERNAL_SOCKET_PATH="$DOCKER_INTERNAL_SOCKET_DIR/docker.sock"

docker_runtime_container_name() {
  printf 'safe-agentic-docker-%s\n' "$1"
}

docker_runtime_socket_volume_name() {
  printf '%s-docker-sock\n' "$1"
}

docker_runtime_data_volume_name() {
  printf '%s-docker-data\n' "$1"
}

create_labeled_volume() {
  local volume_name="$1"
  local volume_type="$2"
  local parent_name="$3"

  if vm_exec docker volume inspect "$volume_name" >/dev/null 2>&1; then
    return 0
  fi

  vm_exec docker volume create \
    --label "app=$IMAGE_NAME" \
    --label "safe-agentic.type=$volume_type" \
    --label "safe-agentic.parent=$parent_name" \
    "$volume_name" >/dev/null
}

ensure_vm_docker_socket_path() {
  local socket_path

  socket_path=$(vm_exec bash -c 'test -S /var/run/docker.sock && printf "%s\n" /var/run/docker.sock' 2>/dev/null || true)
  [ -n "$socket_path" ] || die "VM Docker socket not found at /var/run/docker.sock."
  printf '%s\n' "$socket_path"
}

vm_docker_socket_gid() {
  local gid

  gid=$(vm_exec bash -c 'stat -c %g /var/run/docker.sock' 2>/dev/null || true)
  [[ "$gid" =~ ^[0-9]+$ ]] || die "Unable to determine VM Docker socket group."
  printf '%s\n' "$gid"
}

append_docker_cli_state() {
  append_ephemeral_volume /home/agent/.docker
}

append_host_docker_socket_access() {
  local socket_path socket_gid

  socket_path=$(ensure_vm_docker_socket_path)
  socket_gid=$(vm_docker_socket_gid)

  append_docker_cli_state
  docker_cmd+=(-e "DOCKER_HOST=unix://$DOCKER_HOST_SOCKET_PATH")
  docker_cmd+=(-v "$socket_path:$DOCKER_HOST_SOCKET_PATH")
  docker_cmd+=(--group-add "$socket_gid")
}

append_internal_docker_access() {
  local container_name="$1"

  DOCKER_RUNTIME_KIND="dind"
  DOCKER_RUNTIME_CONTAINER=$(docker_runtime_container_name "$container_name")
  DOCKER_RUNTIME_SOCKET_VOLUME=$(docker_runtime_socket_volume_name "$container_name")
  DOCKER_RUNTIME_DATA_VOLUME=$(docker_runtime_data_volume_name "$container_name")

  append_docker_cli_state
  docker_cmd+=(-e "DOCKER_HOST=unix://$DOCKER_INTERNAL_SOCKET_PATH")
  docker_cmd+=(--mount "type=volume,src=$DOCKER_RUNTIME_SOCKET_VOLUME,dst=$DOCKER_INTERNAL_SOCKET_DIR")
}

reset_docker_runtime_state() {
  DOCKER_RUNTIME_KIND="off"
  DOCKER_RUNTIME_CONTAINER=""
  DOCKER_RUNTIME_SOCKET_VOLUME=""
  DOCKER_RUNTIME_DATA_VOLUME=""
}

wait_for_internal_docker_runtime() {
  local runtime_name="$1"
  local output=""
  local _i

  for _i in $(seq 1 40); do
    if output=$(vm_exec docker exec "$runtime_name" docker --host "unix://$DOCKER_INTERNAL_SOCKET_PATH" info 2>&1); then
      return 0
    fi
    sleep 0.5
  done

  output=$(vm_exec docker logs "$runtime_name" 2>&1 || true)
  [ -n "$output" ] && warn "Internal Docker daemon logs:\n$output"
  return 1
}

start_internal_docker_runtime() {
  local container_name="$1"
  local network_name="$2"

  if [ "$DOCKER_RUNTIME_CONTAINER" != "$(docker_runtime_container_name "$container_name")" ]; then
    append_internal_docker_access "$container_name"
  fi

  create_labeled_volume "$DOCKER_RUNTIME_SOCKET_VOLUME" "docker-runtime-volume" "$container_name"
  create_labeled_volume "$DOCKER_RUNTIME_DATA_VOLUME" "docker-runtime-volume" "$container_name"

  vm_exec docker rm -f "$DOCKER_RUNTIME_CONTAINER" >/dev/null 2>&1 || true

  vm_exec docker run -d \
    --name "$DOCKER_RUNTIME_CONTAINER" \
    --hostname "$DOCKER_RUNTIME_CONTAINER" \
    --pull=never \
    --privileged \
    --user root \
    --network "$network_name" \
    --label "app=$IMAGE_NAME" \
    --label "safe-agentic.type=docker-runtime" \
    --label "safe-agentic.parent=$container_name" \
    --mount "type=volume,src=$DOCKER_RUNTIME_SOCKET_VOLUME,dst=$DOCKER_INTERNAL_SOCKET_DIR" \
    --mount "type=volume,src=$DOCKER_RUNTIME_DATA_VOLUME,dst=/var/lib/docker" \
    --tmpfs /tmp:rw,nosuid,size=512m \
    -e SAFE_AGENTIC_INTERNAL_DOCKERD=1 \
    -e "SAFE_AGENTIC_DOCKER_SOCKET=$DOCKER_INTERNAL_SOCKET_PATH" \
    -e SAFE_AGENTIC_DOCKER_DATA_ROOT=/var/lib/docker \
    "$IMAGE_NAME:$IMAGE_TAG" >/dev/null

  wait_for_internal_docker_runtime "$DOCKER_RUNTIME_CONTAINER" \
    || die "Internal Docker daemon failed to start for '$container_name'."
}

remove_docker_runtime_for_container() {
  local container_name="$1"
  local runtime_name socket_volume data_volume

  runtime_name=$(docker_runtime_container_name "$container_name")
  socket_volume=$(docker_runtime_socket_volume_name "$container_name")
  data_volume=$(docker_runtime_data_volume_name "$container_name")

  vm_exec docker rm -f "$runtime_name" >/dev/null 2>&1 || true
  vm_exec docker volume rm "$socket_volume" "$data_volume" >/dev/null 2>&1 || true
}

cleanup_all_docker_runtimes() {
  local runtime_ids volume_ids

  runtime_ids=$(vm_exec docker ps -aq --filter "label=app=$IMAGE_NAME" --filter "label=safe-agentic.type=docker-runtime" 2>/dev/null || echo "")
  if [ -n "$runtime_ids" ]; then
    vm_exec docker rm -f $runtime_ids >/dev/null 2>&1 || true
  fi

  volume_ids=$(vm_exec docker volume ls -q --filter "label=app=$IMAGE_NAME" --filter "label=safe-agentic.type=docker-runtime-volume" 2>/dev/null || echo "")
  if [ -n "$volume_ids" ]; then
    vm_exec docker volume rm $volume_ids >/dev/null 2>&1 || true
  fi
}
