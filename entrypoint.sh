#!/usr/bin/env bash
# Container entrypoint: set up runtime config on tmpfs, clone repos, launch agent or shell.
set -euo pipefail

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

repo_clone_path() {
  local repo_url="$1"
  local clone_path
  local owner repo

  # Reject URLs that look like flags (git option injection)
  case "$repo_url" in -*) return 1 ;; esac

  # Strip trailing .git suffix
  clone_path="${repo_url%.git}"

  # Extract the path after the host
  case "$clone_path" in
    https://*|ssh://*) clone_path="${clone_path#*://*/}" ;; # URL-style scheme://host/org/repo
    *:*/*)   clone_path="${clone_path##*:}" ;;    # scp-style git@host:org/repo
    *)       return 1 ;;
  esac

  # Must contain exactly one slash (owner/repo, two components)
  [[ "$clone_path" == */* ]] || return 1
  owner="${clone_path%/*}"
  repo="${clone_path#*/}"
  [[ "$owner" == */* ]] && return 1

  # Reject dot-prefixed names, dash-prefixed names, empty, and unsafe characters
  case "$owner" in ""|.*|-*) return 1 ;; esac
  case "$repo"  in ""|.*|-*) return 1 ;; esac
  [[ "$owner" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]] || return 1
  [[ "$repo"  =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]] || return 1

  printf '%s/%s\n' "$owner" "$repo"
}

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
    # Container IS the sandbox — skip permission checks
    # OAuth login: on first run, Claude will display a URL to open in your browser
    exec claude --dangerously-skip-permissions "$@"
    ;;
  codex)
    echo "[entrypoint] Launching Codex..."
    # In a headless container, the localhost callback OAuth flow doesn't work.
    # Use device-auth flow: shows a URL + code to open in your browser.
    if [ ! -f "$HOME/.codex/auth.json" ]; then
      echo "[entrypoint] First run — authenticating via device code flow..."
      echo "[entrypoint] A URL will appear. Open it in your macOS browser to log in."
      codex login --device-auth
    fi
    # Container IS the sandbox — run Codex in yolo mode.
    exec codex --yolo "$@"
    ;;
  *)
    echo "[entrypoint] No agent type set. Starting interactive shell."
    echo "[entrypoint] All tools available. Repos in /workspace/."
    exec bash -l "$@"
    ;;
esac
