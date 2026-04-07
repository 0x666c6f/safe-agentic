#!/usr/bin/env bash
# Container entrypoint: configure git, clone repos, launch agent or shell.
set -euo pipefail

# ---------------------------------------------------------------------------
# Git configuration
# ---------------------------------------------------------------------------
git config --global user.name "AI Agent"
git config --global user.email "agent@safe-agentic.local"
git config --global url."git@github.com:".insteadOf "https://github.com/"
git config --global core.pager "delta --dark"
git config --global init.defaultBranch main

# ---------------------------------------------------------------------------
# Clone repositories
# ---------------------------------------------------------------------------
if [ -n "${REPOS:-}" ]; then
  IFS=',' read -ra REPO_LIST <<< "$REPOS"
  for repo_url in "${REPO_LIST[@]}"; do
    repo_url=$(echo "$repo_url" | xargs)  # trim whitespace
    # Use org/repo as clone path to avoid collisions (e.g., org-a/api vs org-b/api)
    clone_path=$(echo "$repo_url" | sed -E 's#^.*[:/]([^/]+/[^/]+)(\.git)?$#\1#')
    clone_dir="/workspace/$clone_path"
    if [ -d "$clone_dir" ]; then
      echo "[entrypoint] $clone_dir already exists, skipping clone"
    else
      echo "[entrypoint] Cloning $repo_url → $clone_dir"
      mkdir -p "$(dirname "$clone_dir")"
      git clone "$repo_url" "$clone_dir"
    fi
  done

  # If single repo, cd into it
  if [ "${#REPO_LIST[@]}" -eq 1 ]; then
    clone_path=$(echo "${REPO_LIST[0]}" | sed -E 's#^.*[:/]([^/]+/[^/]+)(\.git)?$#\1#')
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
    exec codex --full-auto "$@"
    ;;
  *)
    echo "[entrypoint] No agent type set. Starting interactive shell."
    echo "[entrypoint] All tools available. Repos in /workspace/."
    exec bash -l "$@"
    ;;
esac
