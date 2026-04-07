#!/usr/bin/env bash
# Idempotent VM bootstrap: hardens OrbStack VM, installs Docker CE, configures daemon.
# Designed to run inside the OrbStack "safe-agentic" Ubuntu VM.
set -euo pipefail

TOTAL_STEPS=5
step() {
  local number="$1"
  shift
  echo "==> [$number/$TOTAL_STEPS] $*"
}

echo "==> Setting up safe-agentic VM..."

# =============================================================================
# Harden OrbStack: block macOS filesystem access and mac integration commands
# OrbStack mounts macOS paths into VMs by default (/Users, /mnt/mac, etc.)
# and links macOS commands (open, osascript, code). We disable all of this.
# =============================================================================
step 1 "Hardening VM: blocking macOS filesystem access..."

# Unmount any macOS-shared paths
for mnt in /Users /mnt/mac /Volumes /private /opt/orbstack; do
  if mountpoint -q "$mnt" 2>/dev/null; then
    echo "    Unmounting $mnt"
    sudo umount -l "$mnt" 2>/dev/null || true
  fi
done

# Prevent remounting on reboot via fstab override
# OrbStack uses gvproxy/virtio mounts; blocking them here
for mnt in /Users /mnt/mac /Volumes /private /opt/orbstack; do
  if ! grep -q "^# safe-agentic: block $mnt" /etc/fstab 2>/dev/null; then
    echo "# safe-agentic: block $mnt" | sudo tee -a /etc/fstab > /dev/null
    echo "none $mnt tmpfs ro,noexec,nosuid,size=1k 0 0" | sudo tee -a /etc/fstab > /dev/null
  fi
done

# Mount tiny read-only tmpfs over macOS paths to block access persistently
for mnt in /Users /mnt/mac /Volumes /private /opt/orbstack; do
  sudo mkdir -p "$mnt"
  if ! mountpoint -q "$mnt" 2>/dev/null || ! mount | grep "$mnt" | grep -q tmpfs; then
    sudo mount -t tmpfs -o ro,noexec,nosuid,size=1k none "$mnt" 2>/dev/null || true
  fi
done

# Remove OrbStack macOS integration commands
step 2 "Removing macOS integration commands..."
MASK_DIRS=""
for cmd in open osascript code mac; do
  CMDPATH=$(command -v "$cmd" 2>/dev/null || true)
  if [ -n "$CMDPATH" ] && echo "$CMDPATH" | grep -q '^/opt/orbstack-guest/'; then
    CMDDIR=$(dirname "$CMDPATH")
    if ! echo " $MASK_DIRS " | grep -Fq " $CMDDIR "; then
      MASK_DIRS="$MASK_DIRS $CMDDIR"
    fi
  elif [ -n "$CMDPATH" ] && [ -L "$CMDPATH" ]; then
    echo "    Removing $CMDPATH (symlink to macOS)"
    sudo rm -f "$CMDPATH" 2>/dev/null || true
  elif [ -n "$CMDPATH" ] && file "$CMDPATH" 2>/dev/null | grep -qi "orbstack\|mac"; then
    echo "    Removing $CMDPATH (OrbStack binary)"
    sudo rm -f "$CMDPATH" 2>/dev/null || true
  fi
done

for dir in $MASK_DIRS; do
  echo "    Masking $dir (OrbStack integration dir)"
  sudo mkdir -p "$dir"
  sudo mount -t tmpfs -o ro,noexec,nosuid,size=1k none "$dir" 2>/dev/null || true
done

# Verify hardening
step 3 "Verifying VM hardening..."
HARDENING_OK=true
for mnt in /Users /mnt/mac; do
  if [ -d "$mnt" ] && find "$mnt" -mindepth 1 -maxdepth 1 -print -quit 2>/dev/null | grep -q .; then
    echo "    WARNING: $mnt still has accessible content"
    HARDENING_OK=false
  else
    echo "    OK: $mnt blocked"
  fi
done
for cmd in open osascript; do
  if command -v "$cmd" &>/dev/null; then
    echo "    WARNING: $cmd still available"
    HARDENING_OK=false
  else
    echo "    OK: $cmd removed"
  fi
done
if ! $HARDENING_OK; then
  echo "==> WARNING: Some hardening steps did not fully apply."
  echo "    OrbStack may re-enable file sharing on VM restart."
  echo "    Consider disabling file sharing in OrbStack settings (Settings > Linux)."
fi

# =============================================================================
# Install Docker CE
# =============================================================================
if ! command -v docker &>/dev/null; then
  step 4 "Installing Docker CE..."
  sudo apt-get update -qq
  sudo apt-get install -y -qq ca-certificates curl gnupg

  sudo install -m 0755 -d /etc/apt/keyrings
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
  sudo chmod a+r /etc/apt/keyrings/docker.gpg

  # shellcheck disable=SC1091
  echo \
    "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
    $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | \
    sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

  sudo apt-get update -qq
  sudo apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
else
  step 4 "Docker already installed: $(docker --version)"
fi

# Add current user to docker group
if ! groups | grep -q docker; then
  echo "==> Adding $(whoami) to docker group..."
  sudo usermod -aG docker "$(whoami)"
fi

# Configure Docker daemon
DAEMON_JSON=/etc/docker/daemon.json
step 5 "Configuring Docker daemon and egress guardrails..."
sudo tee "$DAEMON_JSON" > /dev/null <<'DJEOF'
{
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "10m",
    "max-file": "3"
  },
  "userns-remap": "default",
  "default-address-pools": [
    {"base": "172.20.0.0/16", "size": 24}
  ]
}
DJEOF
sudo systemctl restart docker

# Enable and start Docker
sudo systemctl enable docker
sudo systemctl start docker

# Install egress guardrails for safe-agentic managed bridges (bridge names start with "sa")
sudo iptables -nL DOCKER-USER >/dev/null 2>&1 || sudo iptables -N DOCKER-USER
sudo iptables -N SAFE_AGENTIC_EGRESS >/dev/null 2>&1 || true
sudo iptables -F SAFE_AGENTIC_EGRESS
sudo iptables -C DOCKER-USER -j SAFE_AGENTIC_EGRESS >/dev/null 2>&1 \
  || sudo iptables -I DOCKER-USER 1 -j SAFE_AGENTIC_EGRESS
sudo iptables -A SAFE_AGENTIC_EGRESS -m conntrack --ctstate ESTABLISHED,RELATED -j RETURN
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
  sudo iptables -A SAFE_AGENTIC_EGRESS -i 'sa+' -d "$cidr" -j REJECT
done
for port in 22 80 443; do
  sudo iptables -A SAFE_AGENTIC_EGRESS -i 'sa+' -p tcp --dport "$port" -j RETURN
done
sudo iptables -A SAFE_AGENTIC_EGRESS -i 'sa+' -j REJECT
sudo iptables -A SAFE_AGENTIC_EGRESS -j RETURN

# Verify Docker
echo "==> Verifying Docker..."
docker info --format '{{.ServerVersion}}' 2>/dev/null && echo "==> Docker is ready." || echo "==> WARNING: Docker may need a re-login for group changes. Run: newgrp docker"

echo "==> VM setup complete."
