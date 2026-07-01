#!/bin/sh
# Idempotent VM bootstrap: hardens Apple container machine, installs Docker, configures daemon.
set -eu

TOTAL_STEPS=5
step() {
  step_number="$1"
  shift
  echo "==> [$step_number/$TOTAL_STEPS] $*"
}

as_root() {
  if [ "$(id -u)" -eq 0 ]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    sudo "$@"
  else
    echo "Need root privileges for: $*" >&2
    exit 1
  fi
}

install_exec_helper() {
  as_root mkdir -p /usr/local/bin
  as_root tee /usr/local/bin/safe-ag-exec >/dev/null <<'EOF'
#!/bin/sh
set -eu
if [ "$#" -lt 1 ]; then
  echo "usage: safe-ag-exec <command> [base64-arg...]" >&2
  exit 64
fi
cmd="$1"
shift
encoded_count="$#"
for encoded do
  decoded="$(printf '%s' "$encoded" | base64 -d)"
  set -- "$@" "$decoded"
done
while [ "$encoded_count" -gt 0 ]; do
  shift
  encoded_count=$((encoded_count - 1))
done
exec "$cmd" "$@"
EOF
  as_root chmod 0755 /usr/local/bin/safe-ag-exec
}

wait_for_docker_process_exit() {
  for _ in $(seq 1 10); do
    if ! pidof dockerd >/dev/null 2>&1 && ! pidof containerd >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  as_root pkill -9 dockerd >/dev/null 2>&1 || true
  as_root pkill -9 containerd >/dev/null 2>&1 || true
  sleep 1
}

start_dockerd_once() {
  as_root pkill dockerd >/dev/null 2>&1 || true
  as_root pkill containerd >/dev/null 2>&1 || true
  wait_for_docker_process_exit
  as_root rm -f /var/run/docker.pid /var/run/docker.sock
  as_root mkdir -p /var/log
  as_root sh -c 'nohup dockerd --host=unix:///var/run/docker.sock >/var/log/dockerd.log 2>&1 &'
}

echo "==> Setting up safe-agentic VM..."

# =============================================================================
# Harden VM host access
# =============================================================================
step 1 "Hardening VM: blocking macOS filesystem access..."
cd /

for mnt in /Users /mnt/mac /Volumes /private; do
  if mountpoint -q "$mnt" 2>/dev/null; then
    echo "    Unmounting $mnt"
    as_root umount -l "$mnt" 2>/dev/null || true
  fi
done

# Apple container machines are created with --home-mount none. These tmpfs masks
# keep the guard explicit if a machine is later reconfigured.
for mnt in /Users /mnt/mac /Volumes /private; do
  as_root mkdir -p "$mnt"
  if ! mountpoint -q "$mnt" 2>/dev/null || ! mount | grep -F " $mnt " | grep -q tmpfs; then
    as_root mount -t tmpfs -o ro,noexec,nosuid,size=1k none "$mnt" 2>/dev/null || true
  fi
done

step 2 "Verifying VM hardening..."
HARDENING_OK=true
for mnt in /Users /mnt/mac /Volumes /private; do
  if [ -d "$mnt" ] && find "$mnt" -mindepth 1 -maxdepth 1 -print -quit 2>/dev/null | grep -q .; then
    echo "    WARNING: $mnt still has accessible content"
    HARDENING_OK=false
  else
    echo "    OK: $mnt blocked"
  fi
done
for cmd in open osascript; do
  if command -v "$cmd" >/dev/null 2>&1; then
    echo "    WARNING: $cmd still available"
    HARDENING_OK=false
  else
    echo "    OK: $cmd unavailable"
  fi
done
if ! $HARDENING_OK; then
  echo "==> WARNING: Some hardening checks did not fully pass."
  echo "    Recreate machine with: container machine create alpine:3.22 --home-mount none"
fi

# =============================================================================
# Install Docker
# =============================================================================
step 3 "Installing Docker dependencies..."
# shellcheck disable=SC1091
. /etc/os-release

case "${ID:-}" in
  alpine)
    as_root apk update
    as_root apk add --no-cache \
      bash \
      ca-certificates \
      curl \
      docker \
      docker-cli \
      git \
      gzip \
      iptables \
      ip6tables \
      openssh-client \
      shadow \
      socat \
      tar \
      tzdata
    ;;
  ubuntu|debian)
    as_root apt-get update -qq
    as_root apt-get install -y -qq ca-certificates curl gnupg socat
    as_root install -m 0755 -d /etc/apt/keyrings
    if [ ! -f /etc/apt/keyrings/docker.gpg ]; then
      curl -fsSL "https://download.docker.com/linux/${ID}/gpg" | as_root gpg --dearmor -o /etc/apt/keyrings/docker.gpg
      as_root chmod a+r /etc/apt/keyrings/docker.gpg
    fi
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/${ID} ${VERSION_CODENAME} stable" \
      | as_root tee /etc/apt/sources.list.d/docker.list >/dev/null
    as_root apt-get update -qq
    as_root apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin socat
    ;;
  *)
    echo "Unsupported VM OS: ${ID:-unknown}" >&2
    exit 1
    ;;
esac

# Docker user namespace remap needs explicit subordinate ranges on Alpine.
as_root touch /etc/subuid /etc/subgid
if ! getent passwd dockremap >/dev/null 2>&1; then
  if command -v adduser >/dev/null 2>&1; then
    as_root adduser -S -H -D dockremap 2>/dev/null || as_root useradd -r -M dockremap
  else
    as_root useradd -r -M dockremap
  fi
fi
grep -q '^dockremap:' /etc/subuid || echo 'dockremap:165536:65536' | as_root tee -a /etc/subuid >/dev/null
grep -q '^dockremap:' /etc/subgid || echo 'dockremap:165536:65536' | as_root tee -a /etc/subgid >/dev/null
install_exec_helper

# =============================================================================
# Configure Docker
# =============================================================================
step 4 "Configuring Docker daemon..."
as_root mkdir -p /etc/docker
as_root tee /etc/docker/daemon.json >/dev/null <<'DJEOF'
{
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "10m",
    "max-file": "3"
  },
  "userns-remap": "default",
  "no-new-privileges": true,
  "default-address-pools": [
    {"base": "172.20.0.0/16", "size": 24}
  ]
}
DJEOF

if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
  as_root systemctl enable docker
  as_root systemctl restart docker
else
  DOCKER_READY=false
  for attempt in 1 2; do
    start_dockerd_once
    for _ in $(seq 1 45); do
      if docker info >/dev/null 2>&1; then
        DOCKER_READY=true
        break
      fi
      sleep 1
    done
    if $DOCKER_READY; then
      break
    fi
    echo "    Docker did not become ready (attempt $attempt/2); retrying..."
    as_root tail -80 /var/log/dockerd.log 2>/dev/null || true
  done
fi

if ! docker info >/dev/null 2>&1; then
  echo "==> Docker failed to start. Last daemon log:"
  as_root tail -200 /var/log/dockerd.log 2>/dev/null || true
  exit 1
fi

# =============================================================================
# Egress guardrails for safe-agentic managed bridges
# =============================================================================
step 5 "Configuring egress guardrails..."
as_root iptables -nL DOCKER-USER >/dev/null 2>&1 || as_root iptables -N DOCKER-USER
as_root iptables -N SAFE_AGENTIC_EGRESS >/dev/null 2>&1 || true
as_root iptables -F SAFE_AGENTIC_EGRESS
as_root iptables -C DOCKER-USER -j SAFE_AGENTIC_EGRESS >/dev/null 2>&1 \
  || as_root iptables -I DOCKER-USER 1 -j SAFE_AGENTIC_EGRESS
as_root iptables -A SAFE_AGENTIC_EGRESS -m conntrack --ctstate ESTABLISHED,RELATED -j RETURN
for cidr in \
  0.0.0.0/8 \
  10.0.0.0/8 \
  100.64.0.0/10 \
  127.0.0.0/8 \
  169.254.0.0/16 \
  172.16.0.0/12 \
  192.168.0.0/16 \
  224.0.0.0/4 \
  240.0.0.0/4 \
; do
  as_root iptables -A SAFE_AGENTIC_EGRESS -i 'sa+' -d "$cidr" -j REJECT
done
for port in 22 80 443; do
  as_root iptables -A SAFE_AGENTIC_EGRESS -i 'sa+' -p tcp --dport "$port" -j RETURN
done
as_root iptables -A SAFE_AGENTIC_EGRESS -i 'sa+' -j REJECT
as_root iptables -A SAFE_AGENTIC_EGRESS -j RETURN

echo "==> Docker $(docker version --format '{{.Server.Version}}') is ready."
echo "==> VM setup complete."
