#!/usr/bin/env bash
# Idempotent VM bootstrap: hardens OrbStack VM, installs Docker CE, configures daemon.
# Designed to run inside the OrbStack "safe-agentic" Ubuntu VM.
set -euo pipefail

echo "==> Setting up safe-agentic VM..."

# =============================================================================
# Harden OrbStack: block macOS filesystem access and mac integration commands
# OrbStack mounts macOS paths into VMs by default (/Users, /mnt/mac, etc.)
# and links macOS commands (open, osascript, code). We disable all of this.
# =============================================================================
echo "==> Hardening VM: blocking macOS filesystem access..."

# Unmount any macOS-shared paths
for mnt in /Users /mnt/mac /Volumes /private /opt/orbstack; do
  if mountpoint -q "$mnt" 2>/dev/null; then
    echo "    Unmounting $mnt"
    sudo umount -l "$mnt" 2>/dev/null || true
  fi
done

# Prevent remounting on reboot via fstab override
# OrbStack uses gvproxy/virtio mounts; blocking them here
for mnt in /Users /mnt/mac /Volumes /private; do
  if ! grep -q "^# safe-agentic: block $mnt" /etc/fstab 2>/dev/null; then
    echo "# safe-agentic: block $mnt" | sudo tee -a /etc/fstab > /dev/null
    echo "none $mnt tmpfs ro,noexec,nosuid,size=1k 0 0" | sudo tee -a /etc/fstab > /dev/null
  fi
done

# Mount tiny read-only tmpfs over macOS paths to block access persistently
for mnt in /Users /mnt/mac /Volumes /private; do
  sudo mkdir -p "$mnt"
  if ! mountpoint -q "$mnt" 2>/dev/null || ! mount | grep "$mnt" | grep -q tmpfs; then
    sudo mount -t tmpfs -o ro,noexec,nosuid,size=1k none "$mnt" 2>/dev/null || true
  fi
done

# Remove OrbStack macOS integration commands
echo "==> Removing macOS integration commands..."
for cmd in open osascript code mac; do
  CMDPATH=$(which "$cmd" 2>/dev/null || true)
  if [ -n "$CMDPATH" ] && [ -L "$CMDPATH" ]; then
    # Only remove if it's a symlink (OrbStack installs these as symlinks)
    echo "    Removing $CMDPATH (symlink to macOS)"
    sudo rm -f "$CMDPATH"
  elif [ -n "$CMDPATH" ] && file "$CMDPATH" 2>/dev/null | grep -qi "orbstack\|mac"; then
    echo "    Removing $CMDPATH (OrbStack binary)"
    sudo rm -f "$CMDPATH"
  fi
done

# Verify hardening
echo "==> Verifying VM hardening..."
HARDENING_OK=true
for mnt in /Users /mnt/mac; do
  if [ -d "$mnt" ] && ls "$mnt" 2>/dev/null | grep -q .; then
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
  echo "==> Installing Docker CE..."
  sudo apt-get update -qq
  sudo apt-get install -y -qq ca-certificates curl gnupg

  sudo install -m 0755 -d /etc/apt/keyrings
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
  sudo chmod a+r /etc/apt/keyrings/docker.gpg

  echo \
    "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
    $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | \
    sudo tee /etc/apt/sources.list.d/docker.list > /dev/null

  sudo apt-get update -qq
  sudo apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
else
  echo "==> Docker already installed: $(docker --version)"
fi

# Add current user to docker group
if ! groups | grep -q docker; then
  echo "==> Adding $(whoami) to docker group..."
  sudo usermod -aG docker "$(whoami)"
fi

# Configure Docker daemon
DAEMON_JSON=/etc/docker/daemon.json
if [ ! -f "$DAEMON_JSON" ]; then
  echo "==> Configuring Docker daemon..."
  sudo tee "$DAEMON_JSON" > /dev/null <<'DJEOF'
{
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "10m",
    "max-file": "3"
  },
  "default-address-pools": [
    {"base": "172.20.0.0/16", "size": 24}
  ]
}
DJEOF
  sudo systemctl restart docker
fi

# Enable and start Docker
sudo systemctl enable docker
sudo systemctl start docker

# Verify Docker
echo "==> Verifying Docker..."
docker info --format '{{.ServerVersion}}' 2>/dev/null && echo "==> Docker is ready." || echo "==> WARNING: Docker may need a re-login for group changes. Run: newgrp docker"

echo "==> VM setup complete."
