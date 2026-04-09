#!/usr/bin/env bash
# Container entrypoint: set up runtime config on tmpfs, clone repos, launch agent or shell.
set -euo pipefail

ENTRYPOINT_DIR="$(cd "$(dirname "$(realpath "${BASH_SOURCE[0]}")")" && pwd)"
REPO_URL_LIB="/usr/local/lib/safe-agentic/repo-url.sh"
AGENT_SESSION_LIB="/usr/local/lib/safe-agentic/agent-session.sh"
TMUX_SESSION_NAME="${SAFE_AGENTIC_TMUX_SESSION_NAME:-safe-agentic}"
TMUX_HISTORY_LIMIT="${SAFE_AGENTIC_TMUX_HISTORY_LIMIT:-500000}"
# shellcheck disable=SC2034  # used by agent-session.sh via env
SESSION_STATE_DIR="${SAFE_AGENTIC_SESSION_STATE_DIR:-/workspace/.safe-agentic}"

if [ -f "$REPO_URL_LIB" ]; then
  # shellcheck disable=SC1090,SC1091
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

ensure_codex_config() {
  local codex_dir="${CODEX_HOME:-$HOME/.codex}"
  local codex_config="$codex_dir/config.toml"

  mkdir -p "$codex_dir" 2>/dev/null || return 0
  [ -w "$codex_dir" ] || return 0
  [ -f "$codex_config" ] && return 0
  if [ -n "${SAFE_AGENTIC_CODEX_CONFIG_B64:-}" ]; then
    echo "$SAFE_AGENTIC_CODEX_CONFIG_B64" | base64 -d > "$codex_config" 2>/dev/null || true
    return 0
  fi

  cat >"$codex_config" 2>/dev/null <<'EOF' || true
approval_policy = "never"
sandbox_mode = "danger-full-access"
EOF
}

ensure_claude_config() {
  local claude_dir="${CLAUDE_CONFIG_DIR:-$HOME/.claude}"
  local claude_config="$claude_dir/settings.json"
  local claude_legacy="$claude_dir/.claude.json"
  local legacy_backup=""

  mkdir -p "$claude_dir" 2>/dev/null || return 0
  [ -w "$claude_dir" ] || return 0
  if [ ! -f "$claude_legacy" ]; then
    legacy_backup=$(find "$claude_dir/backups" -maxdepth 1 -name '.claude.json.backup.*' -type f 2>/dev/null | sort | tail -1 || true)
    if [ -n "$legacy_backup" ]; then
      cat "$legacy_backup" > "$claude_legacy" 2>/dev/null || true
    else
      printf '{\n  "firstStartTime": "%s"\n}\n' "$(date -u +"%Y-%m-%dT%H:%M:%SZ")" > "$claude_legacy" 2>/dev/null || true
    fi
  fi

  [ -f "$claude_config" ] && return 0
  if [ -n "${SAFE_AGENTIC_CLAUDE_CONFIG_B64:-}" ]; then
    echo "$SAFE_AGENTIC_CLAUDE_CONFIG_B64" | base64 -d > "$claude_config" 2>/dev/null || true
    return 0
  fi

  cat >"$claude_config" 2>/dev/null <<'EOF' || true
{
  "permissions": {
    "defaultMode": "bypassPermissions"
  }
}
EOF
}

ensure_claude_support_files() {
  local claude_dir="${CLAUDE_CONFIG_DIR:-$HOME/.claude}"

  [ -n "${SAFE_AGENTIC_CLAUDE_SUPPORT_B64:-}" ] || return 0
  mkdir -p "$claude_dir" 2>/dev/null || return 0
  echo "$SAFE_AGENTIC_CLAUDE_SUPPORT_B64" | base64 -d | tar -xzf - -C "$claude_dir" 2>/dev/null || return 0
}

start_tmux_session() {
  local session_name="$1"
  shift

  tmux new-session -d -s "$session_name" "$AGENT_SESSION_LIB" "$@"
  tmux set-option -t "$session_name" history-limit "$TMUX_HISTORY_LIMIT" >/dev/null 2>&1 || true
}

wait_for_tmux_session_start() {
  local session_name="$1"
  local i=0

  while [ "$i" -lt 50 ]; do
    if tmux has-session -t "$session_name" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.1
    i=$((i + 1))
  done

  return 1
}

wait_for_tmux_session_exit() {
  local session_name="$1"
  local misses=0

  while true; do
    if tmux has-session -t "$session_name" >/dev/null 2>&1; then
      misses=0
    else
      misses=$((misses + 1))
      if [ "$misses" -ge 5 ]; then
        return 0
      fi
    fi
    sleep 1
  done
}

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
case "${AGENT_TYPE:-}" in
  claude) ensure_claude_support_files; ensure_claude_config ;;
  codex)  ensure_codex_config ;;
  *)      ensure_codex_config; ensure_claude_support_files; ensure_claude_config ;;
esac

# AWS credentials (written to tmpfs-backed ~/.aws)
if [ -n "${SAFE_AGENTIC_AWS_CREDS_B64:-}" ]; then
  mkdir -p /home/agent/.aws 2>/dev/null || true
  echo "$SAFE_AGENTIC_AWS_CREDS_B64" | base64 -d > /home/agent/.aws/credentials 2>/dev/null || true
  chmod 600 /home/agent/.aws/credentials 2>/dev/null || true
fi

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
# Run setup script from safe-agentic.json (if present)
# ---------------------------------------------------------------------------
run_lifecycle_script() {
  local script_name="$1"
  local config_file=""

  # Find safe-agentic.json in any cloned repo
  for dir in /workspace/*/; do
    if [ -f "${dir}safe-agentic.json" ]; then
      config_file="${dir}safe-agentic.json"
      break
    fi
  done

  [ -n "$config_file" ] || return 0

  local script_cmd
  script_cmd=$(python3 -c "
import json, sys
try:
    with open(sys.argv[1]) as f:
        data = json.load(f)
    print(data.get('scripts', {}).get(sys.argv[2], ''))
except Exception:
    pass
" "$config_file" "$script_name" 2>/dev/null || true)

  [ -n "$script_cmd" ] || return 0

  echo "[entrypoint] Running $script_name script from safe-agentic.json..."
  (cd "$(dirname "$config_file")" && bash -c "$script_cmd") || {
    echo "[entrypoint] WARNING: $script_name script failed (exit $?)" >&2
  }
}

run_lifecycle_script "setup"

# ---------------------------------------------------------------------------
# Inject agent instructions (AGENT-INSTRUCTIONS.md)
# ---------------------------------------------------------------------------
if [ -n "${SAFE_AGENTIC_INSTRUCTIONS_B64:-}" ]; then
  echo "[entrypoint] Writing AGENT-INSTRUCTIONS.md to /workspace..."
  mkdir -p /workspace 2>/dev/null || true
  if [ -w /workspace ]; then
    printf '%s' "$SAFE_AGENTIC_INSTRUCTIONS_B64" | base64 -d > /workspace/AGENT-INSTRUCTIONS.md 2>/dev/null || true
    echo "[entrypoint] AGENT-INSTRUCTIONS.md written."
  else
    echo "[entrypoint] WARNING: /workspace not writable, cannot write AGENT-INSTRUCTIONS.md" >&2
  fi
fi

# ---------------------------------------------------------------------------
# Launch agent or interactive shell
# ---------------------------------------------------------------------------
AGENT_TYPE="${AGENT_TYPE:-}"
launch_args=("$@")

if [ -n "$AGENT_TYPE" ] && [ "${#launch_args[@]}" -eq 1 ] && [ "${launch_args[0]}" = "bash" ]; then
  launch_args=()
fi

# Short-circuit for --help: run the CLI directly without tmux (needed for CI smoke tests)
if [ "${#launch_args[@]}" -ge 1 ] && [ "${launch_args[0]}" = "--help" ]; then
  case "$AGENT_TYPE" in
    claude) exec claude "${launch_args[@]}" 2>&1 ;;
    codex)  exec codex "${launch_args[@]}" ;;
    *)      exec bash -l "${launch_args[@]}" ;;
  esac
fi

case "$AGENT_TYPE" in
  claude)
    echo "[entrypoint] Launching Claude Code..."
    echo "[entrypoint] Container is the sandbox; Claude permission prompts are intentionally skipped."
    if [ "${SAFE_AGENTIC_BACKGROUND:-}" = "1" ]; then
      # Background mode: run directly, no tmux. Output goes to docker logs.
      echo "[entrypoint] Background mode — output to docker logs, not attachable."
      exec "$AGENT_SESSION_LIB" "${launch_args[@]}"
    fi
    if [ "${#launch_args[@]}" -gt 0 ]; then
      start_tmux_session "$TMUX_SESSION_NAME" "${launch_args[@]}"
    else
      start_tmux_session "$TMUX_SESSION_NAME"
    fi
    wait_for_tmux_session_start "$TMUX_SESSION_NAME" || {
      echo "[entrypoint] tmux session failed to start" >&2
      exit 1
    }
    # Auto-accept trust prompt and/or send pending prompt via tmux keystrokes.
    if [ "${SAFE_AGENTIC_AUTO_TRUST:-}" = "1" ] || [ -f "$SESSION_STATE_DIR/pending-prompt" ]; then
      (
        if [ "${SAFE_AGENTIC_AUTO_TRUST:-}" = "1" ]; then
          # Wait for Claude to show the trust prompt, then press Enter to accept
          sleep 4
          tmux send-keys -t "$TMUX_SESSION_NAME" Enter
          sleep 3
        fi
        if [ -f "$SESSION_STATE_DIR/pending-prompt" ]; then
          # Wait for Claude to be ready, then type the prompt
          [ "${SAFE_AGENTIC_AUTO_TRUST:-}" = "1" ] || sleep 5
          prompt=$(cat "$SESSION_STATE_DIR/pending-prompt")
          rm -f "$SESSION_STATE_DIR/pending-prompt"
          tmux send-keys -t "$TMUX_SESSION_NAME" -l "$prompt"
          tmux send-keys -t "$TMUX_SESSION_NAME" Enter
        fi
      ) &
    fi
    wait_for_tmux_session_exit "$TMUX_SESSION_NAME"
    ;;
  codex)
    echo "[entrypoint] Launching Codex..."
    echo "[entrypoint] Container is the sandbox; Codex yolo mode is intentional here."
    if [ "${SAFE_AGENTIC_BACKGROUND:-}" = "1" ]; then
      echo "[entrypoint] Background mode — output to docker logs, not attachable."
      exec "$AGENT_SESSION_LIB" "${launch_args[@]}"
    fi
    if [ "${#launch_args[@]}" -gt 0 ]; then
      start_tmux_session "$TMUX_SESSION_NAME" "${launch_args[@]}"
    else
      start_tmux_session "$TMUX_SESSION_NAME"
    fi
    wait_for_tmux_session_start "$TMUX_SESSION_NAME" || {
      echo "[entrypoint] tmux session failed to start" >&2
      exit 1
    }
    wait_for_tmux_session_exit "$TMUX_SESSION_NAME"
    ;;
  *)
    echo "[entrypoint] No agent type set. Starting interactive shell."
    echo "[entrypoint] All tools available. Repos in /workspace/."
    if [ "${#launch_args[@]}" -gt 0 ]; then
      exec bash -l "${launch_args[@]}"
    else
      exec bash -l
    fi
    ;;
esac

# ---------------------------------------------------------------------------
# On-exit callback (runs after agent exits; not for interactive shell which exec'd)
# ---------------------------------------------------------------------------
if [ -n "${SAFE_AGENTIC_ON_EXIT_B64:-}" ]; then
  on_exit_cmd=$(printf '%s' "$SAFE_AGENTIC_ON_EXIT_B64" | base64 -d 2>/dev/null || true)
  if [ -n "$on_exit_cmd" ]; then
    echo "[entrypoint] Running on-exit callback..."
    bash -c "$on_exit_cmd" || {
      echo "[entrypoint] WARNING: on-exit callback exited with status $?" >&2
    }
  fi
fi
