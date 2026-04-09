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
  if [ ! -f "$HOME/.codex/auth.json" ]; then
    echo "[entrypoint] First run — authenticating via device code flow..."
    echo "[entrypoint] A URL will appear. Open it in your macOS browser to log in."
    codex login --device-auth
  fi

  if $resuming; then
    exec codex --yolo resume --last
  fi

  if [ $# -gt 0 ]; then
    # Run the prompt first, then continue into interactive mode so the
    # session stays alive for attach/peek in detached containers.
    codex --yolo "$@"
    exec codex --yolo resume --last
  fi

  exec codex --yolo
}

launch_claude() {
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

  if $has_prompt; then
    # Run the prompt first, then continue into interactive mode so the
    # session stays alive for attach/peek in detached containers.
    rendered=$(quote_cmd "${cmd[@]}")
    rendered="$rendered; $(quote_cmd claude --dangerously-skip-permissions --continue)"
    script -qfc "$rendered" /dev/null
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
