#!/usr/bin/env bash
set -euo pipefail

SESSION_STATE_DIR="${SAFE_AGENTIC_SESSION_STATE_DIR:-/workspace/.safe-agentic}"
SESSION_STATE_FILE="$SESSION_STATE_DIR/started"
SESSION_EVENTS_DIR="$SESSION_STATE_DIR"
SESSION_EVENTS_FILE="$SESSION_EVENTS_DIR/session-events.jsonl"
AGENT_TYPE="${AGENT_TYPE:-shell}"

mkdir -p "$SESSION_STATE_DIR"

write_session_event() {
  local event_type="$1" data="${2:-"{}"}"
  local ts
  ts=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
  mkdir -p "$SESSION_EVENTS_DIR"
  printf '{"ts":"%s","type":"%s","data":%s}\n' "$ts" "$event_type" "$data" >> "$SESSION_EVENTS_FILE"
}

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
    codex --yolo resume --last || return $?
    return 0
  fi

  if [ "${SAFE_AGENTIC_BACKGROUND:-}" = "1" ]; then
    # Background mode: run via script PTY (needed for auth refresh).
    # Output goes to stdout → docker logs. No tmux.
    local rendered
    rendered=$(quote_cmd codex --yolo "$@")
    script -qfc "$rendered" /dev/null || return $?
    return 0
  fi

  if [ $# -gt 0 ]; then
    if [ "${SAFE_AGENTIC_FLEET:-}" = "1" ]; then
      # Fleet/pipeline mode: run prompt and exit when done.
      exec codex --yolo "$@"
      return $?
    fi
    # Interactive mode: run prompt, then keep session alive for attach/peek.
    codex --yolo "$@"
    codex --yolo resume --last || return $?
    return 0
  fi

  codex --yolo || return $?
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
    script -qfc "$rendered" /dev/null || return $?
    return 0
  fi

  if $has_prompt; then
    if [ "${SAFE_AGENTIC_FLEET:-}" = "1" ]; then
      # Fleet/pipeline mode: pass -p directly to Claude so it runs
      # non-interactively and exits when done. Run directly (no script
      # wrapper) so output is visible in the tmux pane for preview.
      exec "${cmd[@]}"
      return $?
    fi

    # Interactive mode: extract prompt, save to file, send via tmux send-keys.
    # This keeps Claude in full interactive mode with live output in the pane.
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

    echo "$prompt_text" > "$SESSION_STATE_DIR/pending-prompt"
    rendered=$(quote_cmd claude --dangerously-skip-permissions)
    script -qfc "$rendered" /dev/null || return $?
    return 0
  else
    rendered=$(quote_cmd "${cmd[@]}")
    script -qfc "$rendered" /dev/null || return $?
    return 0
  fi
}

launch_shell() {
  bash -il "$@" || return $?
}

write_session_event "session.start" \
  "$(printf '{"agent":"%s","repos":"%s"}' "${AGENT_TYPE:-unknown}" "${REPOS:-}")"

agent_exit_code=0
case "$AGENT_TYPE" in
  codex)
    launch_codex "$@" || agent_exit_code=$?
    ;;
  claude)
    launch_claude "$@" || agent_exit_code=$?
    ;;
  *)
    launch_shell "$@" || agent_exit_code=$?
    ;;
esac

write_session_event "session.end" \
  "$(printf '{"exit_code":%d}' "$agent_exit_code")"

exit "$agent_exit_code"
