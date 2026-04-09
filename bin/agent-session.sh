#!/usr/bin/env bash
set -euo pipefail

SESSION_STATE_DIR="${SAFE_AGENTIC_SESSION_STATE_DIR:-/workspace/.safe-agentic}"
SESSION_STATE_FILE="$SESSION_STATE_DIR/started"
AGENT_TYPE="${AGENT_TYPE:-shell}"

mkdir -p "$SESSION_STATE_DIR"

resuming=false
if [ -f "$SESSION_STATE_FILE" ]; then
  resuming=true
fi
touch "$SESSION_STATE_FILE"

quote_cmd() {
  local rendered=""
  local arg

  for arg in "$@"; do
    printf -v rendered '%s%q ' "$rendered" "$arg"
  done

  printf '%s\n' "${rendered% }"
}

launch_codex() {
  trust_workspace
  if [ ! -f "$HOME/.codex/auth.json" ]; then
    echo "[entrypoint] First run — authenticating via device code flow..."
    echo "[entrypoint] A URL will appear. Open it in your macOS browser to log in."
    codex login --device-auth
  fi

  if $resuming; then
    exec codex --yolo resume --last
  fi

  if [ "${SAFE_AGENTIC_BACKGROUND:-}" = "1" ]; then
    # Background mode: run via script PTY (needed for auth refresh).
    # Output goes to stdout → docker logs. No tmux.
    local rendered
    rendered=$(quote_cmd codex --yolo "$@")
    exec script -qfc "$rendered" /dev/null
  fi

  if [ $# -gt 0 ]; then
    # Run the prompt first, then continue into interactive mode so the
    # session stays alive for attach/peek in detached containers.
    codex --yolo "$@"
    exec codex --yolo resume --last
  fi

  exec codex --yolo
}

trust_workspace() {
  # Auto-trust the current workspace so Claude doesn't prompt
  # "Do you trust this project?" which blocks non-interactive sessions.
  # Only enabled when SAFE_AGENTIC_AUTO_TRUST=1 (set via --auto-trust flag).
  [ "${SAFE_AGENTIC_AUTO_TRUST:-}" = "1" ] || return 0
  local claude_dir="${CLAUDE_CONFIG_DIR:-$HOME/.claude}"
  local settings_local="$claude_dir/settings.json"
  local cwd
  cwd="$(pwd)"

  mkdir -p "$claude_dir" 2>/dev/null || return 0

  python3 -c "
import json, sys, os, glob

path = '$settings_local'
cwd = '$cwd'

try:
    with open(path) as f:
        data = json.load(f)
except:
    data = {}

trusted = data.get('trustedDirectories', [])
changed = False

# Trust the current directory
if cwd not in trusted:
    trusted.append(cwd)
    changed = True

# Trust /workspace and all immediate subdirectories (cloned repos)
for d in ['/workspace'] + glob.glob('/workspace/*/*'):
    if os.path.isdir(d) and d not in trusted:
        trusted.append(d)
        changed = True

if changed:
    data['trustedDirectories'] = trusted
    with open(path, 'w') as f:
        json.dump(data, f, indent=2)
" 2>/dev/null || true
}

launch_claude() {
  trust_workspace
  local -a cmd=(claude --dangerously-skip-permissions)
  local rendered has_prompt=false

  if $resuming; then
    cmd+=(--continue)
  else
    # Check if args contain a -p prompt flag
    for arg in "$@"; do
      if [ "$arg" = "-p" ]; then
        has_prompt=true
        break
      fi
    done
    cmd+=("$@")
  fi

  if [ "${SAFE_AGENTIC_BACKGROUND:-}" = "1" ]; then
    # Background mode: run with -p via script PTY (needed for OAuth refresh).
    # Output goes to stdout → docker logs. No tmux.
    rendered=$(quote_cmd "${cmd[@]}")
    exec script -qfc "$rendered" /dev/null
  fi

  if $has_prompt; then
    # Extract the prompt text from args (-p "prompt text")
    local prompt_text=""
    local skip_next=false
    for arg in "$@"; do
      if $skip_next; then
        prompt_text="$arg"
        break
      fi
      if [ "$arg" = "-p" ]; then
        skip_next=true
      fi
    done

    # Save prompt to a file; the entrypoint will send it via tmux send-keys
    # after the interactive session starts. This keeps Claude in full
    # interactive mode with live output visible in the tmux pane.
    echo "$prompt_text" > "$SESSION_STATE_DIR/pending-prompt"
    rendered=$(quote_cmd claude --dangerously-skip-permissions)
    exec script -qfc "$rendered" /dev/null
  else
    rendered=$(quote_cmd "${cmd[@]}")
    exec script -qfc "$rendered" /dev/null
  fi
}

launch_shell() {
  exec bash -il "$@"
}

case "$AGENT_TYPE" in
  codex)
    launch_codex "$@"
    ;;
  claude)
    launch_claude "$@"
    ;;
  *)
    launch_shell "$@"
    ;;
esac
