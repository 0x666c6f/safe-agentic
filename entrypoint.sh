#!/usr/bin/env bash
# Container entrypoint: set up runtime config on tmpfs, clone repos, launch agent or shell.
set -euo pipefail

ENTRYPOINT_DIR="$(cd "$(dirname "$(realpath "${BASH_SOURCE[0]}")")" && pwd)"
REPO_URL_LIB="/usr/local/lib/safe-agentic/repo-url.sh"

if [ -f "$REPO_URL_LIB" ]; then
  # shellcheck disable=SC1091
  source "$REPO_URL_LIB"
else
  # shellcheck disable=SC1091
  source "$ENTRYPOINT_DIR/bin/repo-url.sh"
fi

if [ "${SAFE_AGENTIC_INTERNAL_DOCKERD:-}" = "1" ]; then
  SOCKET_PATH="${SAFE_AGENTIC_DOCKER_SOCKET:-/run/safe-agentic-docker/docker.sock}"
  DATA_ROOT="${SAFE_AGENTIC_DOCKER_DATA_ROOT:-/var/lib/docker}"

  echo "[entrypoint] Launching internal Docker daemon..."
  mkdir -p "$(dirname "$SOCKET_PATH")" "$DATA_ROOT"
  exec dockerd --group agent --host "unix://$SOCKET_PATH" --data-root "$DATA_ROOT"
fi

# ---------------------------------------------------------------------------
# Runtime config (written to tmpfs — rootfs is read-only)
# ---------------------------------------------------------------------------

# Copy baked-in SSH config and known_hosts to writable tmpfs mount
cp /home/agent/.ssh.baked/* /home/agent/.ssh/ 2>/dev/null || true
chmod 700 /home/agent/.ssh
chmod 600 /home/agent/.ssh/* 2>/dev/null || true

# Git configuration (writes to tmpfs-backed /home/agent/.config/git/config)
: "${GIT_CONFIG_GLOBAL:=/home/agent/.config/git/config}"
mkdir -p "$(dirname "$GIT_CONFIG_GLOBAL")"
git config --global user.name  "${GIT_AUTHOR_NAME:-Agent}"
git config --global user.email "${GIT_AUTHOR_EMAIL:-agent@localhost}"
git config --global core.pager "delta --dark"
git config --global init.defaultBranch main

# ---------------------------------------------------------------------------
# Clone repositories
# ---------------------------------------------------------------------------
if [ -n "${REPOS:-}" ]; then
  IFS=',' read -ra REPO_LIST <<< "$REPOS"
  for repo_url in "${REPO_LIST[@]}"; do
    repo_url=$(echo "$repo_url" | xargs)  # trim whitespace
    clone_path=$(repo_clone_path "$repo_url") || {
      echo "[entrypoint] Refusing repo URL with unsafe clone path: $repo_url" >&2
      exit 1
    }
    clone_dir="/workspace/$clone_path"
    if [ -d "$clone_dir" ]; then
      echo "[entrypoint] $clone_dir already exists, skipping clone"
    else
      echo "[entrypoint] Cloning $repo_url → $clone_dir"
      mkdir -p "$(dirname "$clone_dir")"
      git clone -- "$repo_url" "$clone_dir"
    fi
  done

  # If single repo, cd into it
  if [ "${#REPO_LIST[@]}" -eq 1 ]; then
    clone_path=$(repo_clone_path "${REPO_LIST[0]}") || {
      echo "[entrypoint] Refusing repo URL with unsafe clone path: ${REPO_LIST[0]}" >&2
      exit 1
    }
    cd "/workspace/$clone_path"
  fi
fi

# ---------------------------------------------------------------------------
# Launch agent or interactive shell
# ---------------------------------------------------------------------------
AGENT_TYPE="${AGENT_TYPE:-}"

case "$AGENT_TYPE" in
  claude)
    echo "[entrypoint] Launching Claude Code..."
    echo "[entrypoint] Container is the sandbox; Claude permission prompts are intentionally skipped."
    # OAuth login: on first run, Claude will display a URL to open in your browser
    exec claude --dangerously-skip-permissions "$@"
    ;;
  codex)
    echo "[entrypoint] Launching Codex..."
    echo "[entrypoint] Container is the sandbox; Codex yolo mode is intentional here."
    # In a headless container, the localhost callback OAuth flow doesn't work.
    # Use device-auth flow: shows a URL + code to open in your browser.
    if [ ! -f "$HOME/.codex/auth.json" ]; then
      echo "[entrypoint] First run — authenticating via device code flow..."
      echo "[entrypoint] A URL will appear. Open it in your macOS browser to log in."
      codex login --device-auth
    fi
    exec codex --yolo "$@"
    ;;
  *)
    echo "[entrypoint] No agent type set. Starting interactive shell."
    echo "[entrypoint] All tools available. Repos in /workspace/."
    exec bash -l "$@"
    ;;
esac
