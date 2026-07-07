#!/usr/bin/env bash
# Container entrypoint: set up runtime config on tmpfs, clone repos, launch agent or shell.
set -euo pipefail

ENTRYPOINT_DIR="$(cd "$(dirname "$(realpath "${BASH_SOURCE[0]}")")" && pwd)"
REPO_URL_LIB="/usr/local/lib/berth/repo-url.sh"
AGENT_SESSION_LIB="/usr/local/lib/berth/agent-session.sh"
TMUX_SESSION_NAME="${BERTH_TMUX_SESSION_NAME:-berth}"
TMUX_HISTORY_LIMIT="${BERTH_TMUX_HISTORY_LIMIT:-500000}"
# shellcheck disable=SC2034  # used by agent-session.sh via env
SESSION_STATE_DIR="${BERTH_SESSION_STATE_DIR:-/workspace/.berth}"

if [ -f "$REPO_URL_LIB" ]; then
  # shellcheck disable=SC1090,SC1091
  source "$REPO_URL_LIB"
else
  # shellcheck disable=SC1091
  source "$ENTRYPOINT_DIR/bin/repo-url.sh"
fi

if [ "${BERTH_INTERNAL_DOCKERD:-}" = "1" ]; then
  SOCKET_PATH="${BERTH_DOCKER_SOCKET:-/run/berth-docker/docker.sock}"
  DATA_ROOT="${BERTH_DOCKER_DATA_ROOT:-/var/lib/docker}"

  echo "[entrypoint] Launching internal Docker daemon..."
  mkdir -p "$(dirname "$SOCKET_PATH")" "$DATA_ROOT"
  exec dockerd --group agent --host "unix://$SOCKET_PATH" --data-root "$DATA_ROOT"
fi

ensure_codex_config() {
  local codex_dir="${CODEX_HOME:-$HOME/.codex}"
  local codex_config="$codex_dir/config.toml"

  mkdir -p "$codex_dir" 2>/dev/null || return 0
  [ -w "$codex_dir" ] || return 0
  if [ -n "${BERTH_CODEX_CONFIG_B64:-}" ]; then
    echo "$BERTH_CODEX_CONFIG_B64" | base64 -d > "$codex_config" 2>/dev/null || true
    return 0
  fi
  [ -f "$codex_config" ] && return 0

  cat >"$codex_config" 2>/dev/null <<'EOF' || true
approval_policy = "never"
sandbox_mode = "danger-full-access"

[projects."/workspace"]
trust_level = "trusted"
EOF
}

ensure_codex_auth() {
  local codex_dir="${CODEX_HOME:-$HOME/.codex}"
  local auth_path="$codex_dir/auth.json"

  [ -n "${BERTH_CODEX_AUTH_B64:-}" ] || return 0
  mkdir -p "$codex_dir" 2>/dev/null || return 0
  [ -w "$codex_dir" ] || return 0
  [ -f "$auth_path" ] && return 0
  echo "$BERTH_CODEX_AUTH_B64" | base64 -d > "$auth_path" 2>/dev/null || true
  chmod 600 "$auth_path" 2>/dev/null || true
}

ensure_codex_support_files() {
  local codex_dir="${CODEX_HOME:-$HOME/.codex}"

  [ -n "${BERTH_CODEX_SUPPORT_B64:-}" ] || return 0
  mkdir -p "$codex_dir" 2>/dev/null || return 0
  [ -w "$codex_dir" ] || return 0
  echo "$BERTH_CODEX_SUPPORT_B64" | base64 -d | tar -xzf - -C "$codex_dir" 2>/dev/null || return 0
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

  # One-shot staged settings from `berth config-sync --restart`: newer than
  # the spawn-time env, consumed once so fresh spawns never inherit it.
  local staged="$claude_dir/settings.host.json"
  if [ -f "$staged" ]; then
    cat "$staged" > "$claude_config" 2>/dev/null || true
    rm -f "$staged"
    return 0
  fi

  if [ -n "${BERTH_CLAUDE_CONFIG_B64:-}" ]; then
    # Host settings.json is the source of truth: refresh on EVERY start so
    # reused auth volumes pick up current preferences (output style, hooks,
    # statusline). Auth (.claude.json / credentials) stays seed-only above.
    echo "$BERTH_CLAUDE_CONFIG_B64" | base64 -d > "$claude_config" 2>/dev/null || true
    return 0
  fi
  [ -f "$claude_config" ] && return 0

  cat >"$claude_config" 2>/dev/null <<'EOF' || true
{
  "permissions": {
    "defaultMode": "bypassPermissions"
  }
}
EOF
}

ensure_claude_auth() {
  local claude_dir="${CLAUDE_CONFIG_DIR:-$HOME/.claude}"
  local auth_path="$claude_dir/.claude.json"
  local tmp_auth=""

  mkdir -p "$claude_dir" 2>/dev/null || true
  # Live OAuth credentials: refresh on EVERY start so a re-logged-in host
  # replaces an expired token in a reused-auth volume. This IS the login.
  if [ -n "${BERTH_CLAUDE_CREDS_B64:-}" ] && [ -w "$claude_dir" ]; then
    if echo "$BERTH_CLAUDE_CREDS_B64" | base64 -d > "$claude_dir/.credentials.json" 2>/dev/null; then
      chmod 600 "$claude_dir/.credentials.json" 2>/dev/null || true
    fi
  fi

  [ -n "${BERTH_CLAUDE_AUTH_B64:-}" ] || return 0
  [ -w "$claude_dir" ] || return 0
  [ -f "$auth_path" ] && return 0
  tmp_auth=$(mktemp) || return 0
  if echo "$BERTH_CLAUDE_AUTH_B64" | base64 -d | gzip -d > "$tmp_auth" 2>/dev/null; then
    mv "$tmp_auth" "$auth_path"
  else
    rm -f "$tmp_auth"
    echo "$BERTH_CLAUDE_AUTH_B64" | base64 -d > "$auth_path" 2>/dev/null || true
  fi
  chmod 600 "$auth_path" 2>/dev/null || true
}

ensure_claude_support_files() {
  local claude_dir="${CLAUDE_CONFIG_DIR:-$HOME/.claude}"

  [ -n "${BERTH_CLAUDE_SUPPORT_B64:-}" ] || return 0
  mkdir -p "$claude_dir" 2>/dev/null || return 0
  echo "$BERTH_CLAUDE_SUPPORT_B64" | base64 -d | tar -xzf - -C "$claude_dir" 2>/dev/null || return 0
  # The support tar loses the executable bit; Claude runs statusline/hooks
  # scripts directly, so restore +x or the custom status line never shows.
  chmod +x "$claude_dir/statusline-command.sh" 2>/dev/null || true
  find "$claude_dir/hooks" -maxdepth 1 -name '*.sh' -exec chmod +x {} + 2>/dev/null || true
}

# ensure_claude_onboarded marks onboarding complete in .claude.json so Claude
# Code never shows the first-run theme/color picker inside the container.
# Runs on EVERY start (not seed-only) because reused auth volumes were seeded
# before this and keep prompting. Merges the key; never clobbers auth fields.
ensure_claude_onboarded() {
  local auth_path="${CLAUDE_CONFIG_DIR:-$HOME/.claude}/.claude.json"
  command -v python3 >/dev/null 2>&1 || return 0
  python3 - "$auth_path" <<'PY' 2>/dev/null || true
import json, os, sys
p = sys.argv[1]
try:
    d = json.load(open(p)) if os.path.exists(p) else {}
except Exception:
    d = {}
if d.get("hasCompletedOnboarding") is True:
    sys.exit(0)
d["hasCompletedOnboarding"] = True
os.makedirs(os.path.dirname(p), exist_ok=True)
json.dump(d, open(p, "w"), indent=2)
PY
}

inject_security_preamble() {
  local agent_type="${AGENT_TYPE:-}"
  local template="${BERTH_PREAMBLE_TEMPLATE:-/usr/local/lib/berth/security-preamble.md}"
  local marker="berth:security-preamble"

  [ -f "$template" ] || return 0

  # Build dynamic values from env vars set by bin/agent
  local ssh_status="disabled"
  [ "${BERTH_SSH_ENABLED:-}" = "1" ] && ssh_status="enabled (forwarded from host)"

  local aws_status="not injected"
  [ -n "${BERTH_AWS_CREDS_B64:-}" ] && aws_status="injected (profile: ${AWS_PROFILE:-default})"

  local network_status="${BERTH_NETWORK_MODE:-managed}"
  case "$network_status" in
    managed) network_status="managed (dedicated bridge, internet access)" ;;
    none)    network_status="none (no network access)" ;;
    custom)  network_status="custom network" ;;
  esac

  local docker_status="${BERTH_DOCKER_MODE:-off}"
  case "$docker_status" in
    off)          docker_status="not available" ;;
    dind)         docker_status="internal daemon (DinD)" ;;
    host-socket)  docker_status="host daemon socket (dangerous)" ;;
  esac

  local resources="${BERTH_RESOURCES:-memory=8g,cpus=4,pids=512}"

  local preamble
  preamble=$(cat "$template")
  preamble=${preamble//'{{SSH_STATUS}}'/$ssh_status}
  preamble=${preamble//'{{AWS_STATUS}}'/$aws_status}
  preamble=${preamble//'{{NETWORK_STATUS}}'/$network_status}
  preamble=${preamble//'{{DOCKER_STATUS}}'/$docker_status}
  preamble=${preamble//'{{RESOURCES}}'/$resources}

  # Inject into the appropriate user-level config file
  case "$agent_type" in
    claude)
      local claude_md="${CLAUDE_CONFIG_DIR:-$HOME/.claude}/CLAUDE.md"
      mkdir -p "$(dirname "$claude_md")" 2>/dev/null || return 0
      if [ -f "$claude_md" ]; then
        # Skip if already injected (e.g., reuse-auth volume from previous run)
        grep -Fq "$marker" "$claude_md" 2>/dev/null && return 0
        # Append to existing user CLAUDE.md
        printf '\n%s\n' "$preamble" >> "$claude_md" 2>/dev/null || true
      else
        printf '%s\n' "$preamble" > "$claude_md" 2>/dev/null || true
      fi
      ;;
    codex)
      local codex_dir="${CODEX_HOME:-$HOME/.codex}"
      local agents_md="$codex_dir/AGENTS.md"
      mkdir -p "$codex_dir" 2>/dev/null || return 0
      if [ -f "$agents_md" ]; then
        grep -Fq "$marker" "$agents_md" 2>/dev/null && return 0
        printf '\n%s\n' "$preamble" >> "$agents_md" 2>/dev/null || true
      else
        printf '%s\n' "$preamble" > "$agents_md" 2>/dev/null || true
      fi
      ;;
    *)
      # Shell / unknown: inject both for flexibility
      local claude_md="${CLAUDE_CONFIG_DIR:-$HOME/.claude}/CLAUDE.md"
      local agents_md="${CODEX_HOME:-$HOME/.codex}/AGENTS.md"
      for target in "$claude_md" "$agents_md"; do
        mkdir -p "$(dirname "$target")" 2>/dev/null || continue
        if [ -f "$target" ]; then
          grep -Fq "$marker" "$target" 2>/dev/null && continue
          printf '\n%s\n' "$preamble" >> "$target" 2>/dev/null || true
        else
          printf '%s\n' "$preamble" > "$target" 2>/dev/null || true
        fi
      done
      ;;
  esac
}

start_tmux_session() {
  local session_name="$1"
  shift

  # Give the detached session a concrete default size: with window-size
  # manual (set in tmux.conf so the desktop app can drive the size), a
  # session created with no client has an undefined 0x0 window and the tmux
  # server dies. The app resizes to the real xterm size on attach.
  tmux new-session -x 200 -y 50 -d -s "$session_name" "$AGENT_SESSION_LIB" "$@"
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
# Wire gh auth into git for HTTPS clones of private repos. Runs when gh has a
# hosts.yml OR a token env (GH_TOKEN, seeded from the host by --reuse-gh-auth):
# gh's credential helper serves the token so `git clone https://github.com/…`
# authenticates instead of prompting for a username (exit 128).
if command -v gh >/dev/null 2>&1 && { [ -s /home/agent/.config/gh/hosts.yml ] || [ -n "${GH_TOKEN:-}" ]; }; then
  gh auth setup-git -h github.com >/dev/null 2>&1 || true
fi
case "${AGENT_TYPE:-}" in
  claude) ensure_claude_auth; ensure_claude_support_files; ensure_claude_config; ensure_claude_onboarded ;;
  codex)  ensure_codex_auth; ensure_codex_support_files; ensure_codex_config ;;
  *)      ensure_codex_auth; ensure_codex_support_files; ensure_codex_config; ensure_claude_auth; ensure_claude_support_files; ensure_claude_config; ensure_claude_onboarded ;;
esac

# Claude Code expects ~/.claude.json at HOME root, but auth volume mounts at ~/.claude/
# which places .claude.json at ~/.claude/.claude.json. Symlink so both paths work.
if [ -f "$HOME/.claude/.claude.json" ] && [ ! -e "$HOME/.claude.json" ]; then
  ln -sf "$HOME/.claude/.claude.json" "$HOME/.claude.json" 2>/dev/null || true
fi

# Inject security preamble into user-level CLAUDE.md / AGENTS.md
inject_security_preamble

# AWS credentials (written to tmpfs-backed ~/.aws)
if [ -n "${BERTH_AWS_CREDS_B64:-}" ]; then
  mkdir -p /home/agent/.aws 2>/dev/null || true
  echo "$BERTH_AWS_CREDS_B64" | base64 -d > /home/agent/.aws/credentials 2>/dev/null || true
  chmod 600 /home/agent/.aws/credentials 2>/dev/null || true
fi

# ---------------------------------------------------------------------------
# Clone repositories
# ---------------------------------------------------------------------------
if [ -n "${REPOS:-}" ]; then
  # Support both comma and space delimiters (bash CLI used commas, Go CLI uses spaces)
  repos_normalized="${REPOS//,/ }"
  IFS=' ' read -ra REPO_LIST <<< "$repos_normalized"
  for repo_url in "${REPO_LIST[@]}"; do
    repo_url="${repo_url#"${repo_url%%[![:space:]]*}"}"
    repo_url="${repo_url%"${repo_url##*[![:space:]]}"}"
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
elif [ "${BERTH_WORKTREE:-}" = "1" ]; then
  cd /workspace
fi

# ---------------------------------------------------------------------------
# Run setup script from berth.json (if present)
# ---------------------------------------------------------------------------
run_lifecycle_script() {
  local script_name="$1"
  local config_file=""

  [ "${BERTH_ALLOW_SETUP_SCRIPTS:-}" = "1" ] || return 0

  while IFS= read -r config_file; do
    [ -n "$config_file" ] || continue

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

    [ -n "$script_cmd" ] || continue

    echo "[entrypoint] Running $script_name script from $config_file..."
    (cd "$(dirname "$config_file")" && bash -c "$script_cmd") || {
      echo "[entrypoint] WARNING: $script_name script failed (exit $?)" >&2
    }
  done < <(find /workspace -mindepth 1 -maxdepth 3 -type f -name berth.json 2>/dev/null | sort)
}

run_lifecycle_script "setup"

# ---------------------------------------------------------------------------
# Inject agent instructions (AGENT-INSTRUCTIONS.md)
# ---------------------------------------------------------------------------
if [ -n "${BERTH_INSTRUCTIONS_B64:-}" ]; then
  echo "[entrypoint] Writing AGENT-INSTRUCTIONS.md to /workspace..."
  mkdir -p /workspace 2>/dev/null || true
  if [ -w /workspace ]; then
    printf '%s' "$BERTH_INSTRUCTIONS_B64" | base64 -d > /workspace/AGENT-INSTRUCTIONS.md 2>/dev/null || true
    echo "[entrypoint] AGENT-INSTRUCTIONS.md written."
  else
    echo "[entrypoint] WARNING: /workspace not writable, cannot write AGENT-INSTRUCTIONS.md" >&2
  fi
fi

# shellcheck disable=SC2317,SC2329 # Invoked indirectly by EXIT trap.
run_on_exit_callback() {
  run_callback "on-exit" "${BERTH_ON_EXIT_B64:-}"
}

run_callback() {
  local name="$1"
  local encoded="${2:-}"
  local callback_cmd=""

  [ -n "$encoded" ] || return 0
  callback_cmd=$(printf '%s' "$encoded" | base64 -d 2>/dev/null || true)
  [ -n "$callback_cmd" ] || return 0

  echo "[entrypoint] Running $name callback..."
  bash -c "$callback_cmd" || {
    echo "[entrypoint] WARNING: $name callback exited with status $?" >&2
  }
}

run_completion_callback() {
  local exit_code="${1:-1}"

  if [ "$exit_code" = "0" ]; then
    run_callback "on-complete" "${BERTH_ON_COMPLETE_B64:-}"
  else
    run_callback "on-fail" "${BERTH_ON_FAIL_B64:-}"
  fi
}

session_exit_code() {
  local events_file="$SESSION_STATE_DIR/session-events.jsonl"

  [ -f "$events_file" ] || {
    printf '1\n'
    return 0
  }

  python3 - "$events_file" <<'PY' 2>/dev/null || printf '1\n'
import json
import sys

exit_code = 1
try:
    with open(sys.argv[1]) as f:
        for line in f:
            try:
                event = json.loads(line)
            except Exception:
                continue
            if event.get("type") == "session.end":
                data = event.get("data") or {}
                exit_code = int(data.get("exit_code", 1))
except Exception:
    pass
print(exit_code)
PY
}

trap run_on_exit_callback EXIT

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
    # Background mode runs inside tmux too: keeps the session attachable
    # (berth attach, desktop app terminals) and steer/peek functional,
    # and makes the berth.terminal=tmux label truthful. Headless
    # prompt runs behave identically — the session exits when the CLI does.
    if [ "${#launch_args[@]}" -gt 0 ]; then
      start_tmux_session "$TMUX_SESSION_NAME" "${launch_args[@]}"
    else
      start_tmux_session "$TMUX_SESSION_NAME"
    fi
    wait_for_tmux_session_start "$TMUX_SESSION_NAME" || {
      echo "[entrypoint] tmux session failed to start" >&2
      exit 1
    }
    # Decode base64 prompt from Go CLI if present
    if [ -n "${BERTH_PROMPT_B64:-}" ] && [ "${#launch_args[@]}" -eq 0 ] && [ ! -f "$SESSION_STATE_DIR/pending-prompt" ]; then
      mkdir -p "$SESSION_STATE_DIR"
      echo "$BERTH_PROMPT_B64" | base64 -d > "$SESSION_STATE_DIR/pending-prompt"
    fi

    # Auto-accept trust prompt and/or send pending prompt via tmux keystrokes.
    if [ "${BERTH_AUTO_TRUST:-}" = "1" ] || [ -f "$SESSION_STATE_DIR/pending-prompt" ]; then
      (
        if [ "${BERTH_AUTO_TRUST:-}" = "1" ]; then
          # Wait for Claude to show the trust prompt, then press Enter to accept
          sleep 4
          tmux send-keys -t "$TMUX_SESSION_NAME" Enter
          sleep 3
        fi
        if [ -f "$SESSION_STATE_DIR/pending-prompt" ]; then
          # Wait for Claude to be ready, then type the prompt
          [ "${BERTH_AUTO_TRUST:-}" = "1" ] || sleep 5
          prompt=$(cat "$SESSION_STATE_DIR/pending-prompt")
          rm -f "$SESSION_STATE_DIR/pending-prompt"
          tmux send-keys -t "$TMUX_SESSION_NAME" -l "$prompt"
          tmux send-keys -t "$TMUX_SESSION_NAME" Enter
        fi
      ) &
    fi
    wait_for_tmux_session_exit "$TMUX_SESSION_NAME"
    agent_status=$(session_exit_code)
    run_completion_callback "$agent_status"
    exit "$agent_status"
    ;;
  codex)
    echo "[entrypoint] Launching Codex..."
    echo "[entrypoint] Container is the sandbox; Codex yolo mode is intentional here."
    # Background mode runs inside tmux too (see claude arm).
    if [ "${#launch_args[@]}" -gt 0 ]; then
      start_tmux_session "$TMUX_SESSION_NAME" "${launch_args[@]}"
    else
      start_tmux_session "$TMUX_SESSION_NAME"
    fi
    wait_for_tmux_session_start "$TMUX_SESSION_NAME" || {
      echo "[entrypoint] tmux session failed to start" >&2
      exit 1
    }
    if [ "${BERTH_AUTO_TRUST:-}" = "1" ]; then
      (
        # Codex trust prompt is interactive-only; accept it in tmux so
        # fleet/pipeline runs can progress without manual input.
        sleep 4
        tmux send-keys -t "$TMUX_SESSION_NAME" Enter
      ) &
    fi
    wait_for_tmux_session_exit "$TMUX_SESSION_NAME"
    agent_status=$(session_exit_code)
    run_completion_callback "$agent_status"
    exit "$agent_status"
    ;;
  shell)
    echo "[entrypoint] Starting shell session in tmux."
    echo "[entrypoint] All tools available. Repos in /workspace/."
    # Shell sessions live in tmux like the AI agents: survives background
    # spawns (bash keeps a pty), attachable from berth attach and the
    # desktop app, steer/peek work.
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
    agent_status=$(session_exit_code)
    run_completion_callback "$agent_status"
    exit "$agent_status"
    ;;
  *)
    echo "[entrypoint] No agent type set. Starting interactive shell."
    echo "[entrypoint] All tools available. Repos in /workspace/."
    shell_status=0
    if [ "${#launch_args[@]}" -gt 0 ]; then
      bash -l "${launch_args[@]}" || shell_status=$?
    else
      bash -l || shell_status=$?
    fi
    run_completion_callback "$shell_status"
    exit "$shell_status"
    ;;
esac
