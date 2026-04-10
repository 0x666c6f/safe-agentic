#!/usr/bin/env bash
# Executable coverage for entrypoint launch flow without a real container.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENTRYPOINT="$REPO_DIR/entrypoint.sh"
TMP_DIR="$(mktemp -d)"
FAKE_BIN="$TMP_DIR/bin"
STATE_DIR="$TMP_DIR/state"
RUN_DIR="$TMP_DIR/run"
HOME_DIR="$TMP_DIR/home"
GIT_LOG="$STATE_DIR/git.log"
EXEC_LOG="$STATE_DIR/exec.log"
STDOUT_LOG="$STATE_DIR/stdout.log"
STDERR_LOG="$STATE_DIR/stderr.log"

trap 'rm -rf "$TMP_DIR"' EXIT

mkdir -p "$FAKE_BIN" "$STATE_DIR" "$RUN_DIR" "$HOME_DIR/.codex" "$HOME_DIR/.claude"

cat >"$FAKE_BIN/cp" <<'EOF'
#!/bin/bash
exit 0
EOF

cat >"$FAKE_BIN/chmod" <<'EOF'
#!/bin/bash
exit 0
EOF

cat >"$FAKE_BIN/mkdir" <<'EOF'
#!/bin/bash
if [ "${1:-}" = "-p" ]; then
  case "${2:-}" in
    /workspace/*|/run/safe-agentic-docker|/var/lib/docker)
      exit 0
      ;;
  esac
fi
exec /bin/mkdir "$@"
EOF

cat >"$FAKE_BIN/git" <<'EOF'
#!/bin/bash
set -euo pipefail
log_file="${TEST_GIT_LOG:?}"
printf '%s\n' "$*" >>"$log_file"
if [ "${1:-}" = "config" ] && [ "${2:-}" = "--global" ]; then
  exit 0
fi
if [ "${1:-}" = "clone" ]; then
  exit 0
fi
echo "unexpected git invocation: $*" >&2
exit 1
EOF

cat >"$FAKE_BIN/bash" <<'EOF'
#!/bin/bash
set -euo pipefail
printf 'bash|%s|%s\n' "$PWD" "$*" >>"${TEST_EXEC_LOG:?}"
exit 0
EOF

cat >"$FAKE_BIN/script" <<'EOF'
#!/bin/bash
set -euo pipefail

if [ "${1:-}" = "-qfc" ]; then
  shift
  command_string="${1:-}"
  shift || true
  shift || true
  exec /bin/bash -c "$command_string"
fi

echo "unexpected script invocation: $*" >&2
exit 1
EOF

cat >"$FAKE_BIN/tmux" <<'EOF'
#!/bin/bash
set -euo pipefail

state_dir="${TEST_TMUX_STATE_DIR:?}"
repo_dir="${TEST_REPO_DIR:?}"

case "${1:-}" in
  start-server)
    exit 0
    ;;
  set-option)
    exit 0
    ;;
  new-session)
    shift
    [ "${1:-}" = "-d" ] || { echo "unexpected tmux new-session args: $*" >&2; exit 1; }
    shift
    [ "${1:-}" = "-s" ] || { echo "unexpected tmux new-session args: $*" >&2; exit 1; }
    shift
    session_name="${1:-}"
    shift
    session_file="$state_dir/tmux-$session_name"
    printf '0\n' >"$session_file"
    if [ "${1:-}" = "/usr/local/lib/safe-agentic/agent-session.sh" ]; then
      shift
      SAFE_AGENTIC_SESSION_STATE_DIR="$state_dir/session-$session_name" /bin/bash "$repo_dir/bin/agent-session.sh" "$@"
    else
      "$@"
    fi
    exit 0
    ;;
  has-session)
    shift
    [ "${1:-}" = "-t" ] || exit 1
    shift
    session_name="${1:-}"
    session_file="$state_dir/tmux-$session_name"
    [ -f "$session_file" ] || exit 1
    checks=$(cat "$session_file")
    if [ "$checks" -eq 0 ]; then
      printf '1\n' >"$session_file"
      exit 0
    fi
    exit 1
    ;;
esac

echo "unexpected tmux invocation: $*" >&2
exit 1
EOF

cat >"$FAKE_BIN/claude" <<'EOF'
#!/bin/bash
set -euo pipefail
printf 'claude|%s|%s\n' "$PWD" "$*" >>"${TEST_EXEC_LOG:?}"
exit 0
EOF

cat >"$FAKE_BIN/codex" <<'EOF'
#!/bin/bash
set -euo pipefail
printf 'codex|%s|%s\n' "$PWD" "$*" >>"${TEST_EXEC_LOG:?}"
if [ "${1:-}" = "login" ] && [ "${2:-}" = "--device-auth" ]; then
  mkdir -p "${HOME:?}/.codex"
  printf '{}\n' >"${HOME:?}/.codex/auth.json"
fi
exit 0
EOF

cat >"$FAKE_BIN/sleep" <<'EOF'
#!/bin/bash
exit 0
EOF

cat >"$FAKE_BIN/dockerd" <<'EOF'
#!/bin/bash
set -euo pipefail
printf 'dockerd|%s|%s\n' "$PWD" "$*" >>"${TEST_EXEC_LOG:?}"
exit 0
EOF

chmod +x "$FAKE_BIN/cp" "$FAKE_BIN/chmod" "$FAKE_BIN/mkdir" "$FAKE_BIN/git" "$FAKE_BIN/bash" "$FAKE_BIN/script" "$FAKE_BIN/tmux" "$FAKE_BIN/claude" "$FAKE_BIN/codex" "$FAKE_BIN/sleep" "$FAKE_BIN/dockerd"

pass=0
fail=0

run_entrypoint() {
  : >"$GIT_LOG"
  : >"$EXEC_LOG"
  : >"$STDOUT_LOG"
  : >"$STDERR_LOG"
  rm -f "$STATE_DIR"/tmux-* 2>/dev/null || true
  rm -rf "$STATE_DIR"/session-* 2>/dev/null || true

  local status=0
  local assignment
  local -a cmd=(/bin/bash "$ENTRYPOINT")
  if [ "${#RUN_ARGS[@]}" -gt 0 ]; then
    cmd+=("${RUN_ARGS[@]}")
  fi
  (
    cd "$RUN_DIR"
    unset AGENT_TYPE REPOS GIT_AUTHOR_NAME GIT_AUTHOR_EMAIL
    export PATH="$FAKE_BIN:$PATH"
    export HOME="$HOME_DIR"
    export GIT_CONFIG_GLOBAL="$TMP_DIR/gitconfig"
    export TEST_GIT_LOG="$GIT_LOG"
    export TEST_EXEC_LOG="$EXEC_LOG"
    export TEST_REPO_DIR="$REPO_DIR"
    export TEST_TMUX_STATE_DIR="$STATE_DIR"
    for assignment in "$@"; do
      export "$assignment"
    done
    "${cmd[@]}"
  ) >"$STDOUT_LOG" 2>"$STDERR_LOG" || status=$?

  LAST_STATUS="$status"
}

assert_contains() {
  local haystack="$1"
  local needle="$2"
  local label="$3"
  if [[ "$haystack" == *"$needle"* ]]; then
    ((++pass))
  else
    echo "FAIL: $label — missing '$needle'" >&2
    echo "  in: $haystack" >&2
    ((++fail))
  fi
}

assert_not_contains() {
  local haystack="$1"
  local needle="$2"
  local label="$3"
  if [[ "$haystack" != *"$needle"* ]]; then
    ((++pass))
  else
    echo "FAIL: $label — unexpected '$needle'" >&2
    echo "  in: $haystack" >&2
    ((++fail))
  fi
}

assert_status() {
  local expected="$1"
  local label="$2"
  if [ "$LAST_STATUS" -eq "$expected" ]; then
    ((++pass))
  else
    echo "FAIL: $label — exit $LAST_STATUS (expected $expected)" >&2
    ((++fail))
  fi
}

RUN_ARGS=()
LAST_STATUS=0

# --- internal dockerd branch starts daemon and exits cleanly ---
rm -f "$HOME_DIR/.codex/auth.json"
RUN_ARGS=()
run_entrypoint SAFE_AGENTIC_INTERNAL_DOCKERD=1
assert_status 0 "internal dockerd exits cleanly"
assert_contains "$(cat "$EXEC_LOG")" "dockerd|$RUN_DIR|--group agent --host unix:///run/safe-agentic-docker/docker.sock --data-root /var/lib/docker" "internal dockerd exec"

# --- shell branch uses safe git fallbacks + login shell ---
rm -f "$HOME_DIR/.codex/auth.json"
RUN_ARGS=(--noprofile)
run_entrypoint
assert_status 0 "shell branch exits cleanly"
assert_contains "$(cat "$GIT_LOG")" "config --global user.name Agent" "shell fallback git name"
assert_contains "$(cat "$GIT_LOG")" "config --global user.email agent@localhost" "shell fallback git email"
assert_contains "$(cat "$GIT_LOG")" "config --global core.pager delta --dark" "shell pager config"
assert_contains "$(cat "$GIT_LOG")" "config --global init.defaultBranch main" "shell init branch"
assert_contains "$(cat "$EXEC_LOG")" "bash|$RUN_DIR|-l --noprofile" "shell execs login bash"
assert_contains "$(cat "$HOME_DIR/.codex/config.toml")" 'approval_policy = "never"' "shell codex default approval"
assert_contains "$(cat "$HOME_DIR/.codex/config.toml")" 'sandbox_mode = "danger-full-access"' "shell codex default sandbox"
assert_contains "$(cat "$HOME_DIR/.claude/settings.json")" '"defaultMode": "bypassPermissions"' "shell claude default config"
assert_contains "$(cat "$HOME_DIR/.claude/.claude.json")" '"firstStartTime"' "shell claude legacy metadata created"

# --- env overrides flow into git config and Claude launch ---
rm -f "$HOME_DIR/.codex/auth.json"
RUN_ARGS=(--print foo)
run_entrypoint AGENT_TYPE=claude GIT_AUTHOR_NAME='Test Agent' GIT_AUTHOR_EMAIL='agent@example.com'
assert_status 0 "claude branch exits cleanly"
assert_contains "$(cat "$GIT_LOG")" "config --global user.name Test Agent" "claude custom git name"
assert_contains "$(cat "$GIT_LOG")" "config --global user.email agent@example.com" "claude custom git email"
assert_contains "$(cat "$EXEC_LOG")" "claude|$RUN_DIR|--dangerously-skip-permissions --print foo" "claude exec flags"

# --- codex first run triggers device auth, then yolo mode ---
rm -f "$HOME_DIR/.codex/auth.json"
RUN_ARGS=(plan)
run_entrypoint AGENT_TYPE=codex
assert_status 0 "codex first run exits cleanly"
codex_first_log="$(cat "$EXEC_LOG")"
assert_contains "$codex_first_log" "codex|$RUN_DIR|login --device-auth" "codex first run login"
assert_contains "$codex_first_log" "codex|$RUN_DIR|--yolo plan" "codex yolo after login"

# --- existing codex auth skips login ---
printf '{}\n' >"$HOME_DIR/.codex/auth.json"
RUN_ARGS=(fix)
run_entrypoint AGENT_TYPE=codex
assert_status 0 "codex with auth exits cleanly"
codex_existing_log="$(cat "$EXEC_LOG")"
assert_not_contains "$codex_existing_log" "login --device-auth" "codex skips login with auth"
assert_contains "$codex_existing_log" "codex|$RUN_DIR|--yolo fix" "codex exec with auth"

# --- existing config files are preserved ---
cat >"$HOME_DIR/.codex/config.toml" <<'EOF'
approval_policy = "on-request"
EOF
cat >"$HOME_DIR/.claude/settings.json" <<'EOF'
{"permissions":{"defaultMode":"ask"}}
EOF
RUN_ARGS=(--noprofile)
run_entrypoint
assert_status 0 "existing config shell exits cleanly"
assert_contains "$(cat "$HOME_DIR/.codex/config.toml")" 'approval_policy = "on-request"' "existing codex config preserved"
assert_contains "$(cat "$HOME_DIR/.claude/settings.json")" '"defaultMode":"ask"' "existing claude config preserved"

# --- multi-repo clone trims whitespace, clones under /workspace, stays in run dir ---
rm -f "$HOME_DIR/.codex/auth.json"
RUN_ARGS=()
run_entrypoint REPOS=' https://github.com/acme/one.git , git@github.com:team/two.git '
assert_status 0 "multi repo exits cleanly"
multi_git_log="$(cat "$GIT_LOG")"
assert_contains "$multi_git_log" "clone -- https://github.com/acme/one.git /workspace/acme/one" "multi repo first clone"
assert_contains "$multi_git_log" "clone -- git@github.com:team/two.git /workspace/team/two" "multi repo second clone"
assert_contains "$(cat "$EXEC_LOG")" "bash|$RUN_DIR|-l" "multi repo stays outside /workspace for shell"

# --- unsafe repo URL is rejected before clone/exec ---
rm -f "$HOME_DIR/.codex/auth.json"
RUN_ARGS=()
run_entrypoint REPOS='git://github.com/acme/repo.git'
assert_status 1 "unsafe repo exits non-zero"
assert_contains "$(cat "$STDERR_LOG")" "Refusing repo URL with unsafe clone path" "unsafe repo rejection"
assert_not_contains "$(cat "$GIT_LOG")" "clone" "unsafe repo no clone"
assert_not_contains "$(cat "$EXEC_LOG")" "bash|" "unsafe repo no shell exec"

# =============================================================================
# Test: SAFE_AGENTIC_CODEX_CONFIG_B64 seeds config.toml when file doesn't exist
# =============================================================================
rm -f "$HOME_DIR/.codex/config.toml" "$HOME_DIR/.codex/auth.json"
rm -f "$HOME_DIR/.claude/settings.json"
CODEX_B64=$(printf 'model = "gpt-4"\n' | base64)
RUN_ARGS=(fix)
run_entrypoint AGENT_TYPE=codex "SAFE_AGENTIC_CODEX_CONFIG_B64=$CODEX_B64"
assert_status 0 "codex config B64 seed exits cleanly"
assert_contains "$(cat "$HOME_DIR/.codex/config.toml")" 'model = "gpt-4"' "codex config created from B64 env"

# =============================================================================
# Test: SAFE_AGENTIC_CODEX_CONFIG_B64 does NOT overwrite existing config.toml
# =============================================================================
cat >"$HOME_DIR/.codex/config.toml" <<'EOF'
model = "o3"
EOF
printf '{}\n' >"$HOME_DIR/.codex/auth.json"
CODEX_B64_NEW=$(printf 'model = "gpt-4"\n' | base64)
RUN_ARGS=(fix)
run_entrypoint AGENT_TYPE=codex "SAFE_AGENTIC_CODEX_CONFIG_B64=$CODEX_B64_NEW"
assert_status 0 "codex config B64 preserve exits cleanly"
assert_contains "$(cat "$HOME_DIR/.codex/config.toml")" 'model = "o3"' "existing codex config preserved over B64"
assert_not_contains "$(cat "$HOME_DIR/.codex/config.toml")" 'model = "gpt-4"' "B64 config not written over existing"

# =============================================================================
# Test: SAFE_AGENTIC_CLAUDE_CONFIG_B64 seeds settings.json when file doesn't exist
# =============================================================================
rm -f "$HOME_DIR/.claude/settings.json" "$HOME_DIR/.codex/auth.json"
rm -f "$HOME_DIR/.codex/config.toml"
CLAUDE_B64=$(printf '{"permissions":{"defaultMode":"ask"}}\n' | base64)
RUN_ARGS=(--print foo)
run_entrypoint AGENT_TYPE=claude "SAFE_AGENTIC_CLAUDE_CONFIG_B64=$CLAUDE_B64"
assert_status 0 "claude config B64 seed exits cleanly"
assert_contains "$(cat "$HOME_DIR/.claude/settings.json")" '"defaultMode":"ask"' "claude config created from B64 env"
assert_contains "$(cat "$HOME_DIR/.claude/.claude.json")" '"firstStartTime"' "claude legacy metadata created from B64 seed"

# =============================================================================
# Test: SAFE_AGENTIC_CLAUDE_CONFIG_B64 does NOT overwrite existing settings.json
# =============================================================================
cat >"$HOME_DIR/.claude/settings.json" <<'EOF'
{"permissions":{"defaultMode":"bypassPermissions"}}
EOF
CLAUDE_B64_NEW=$(printf '{"permissions":{"defaultMode":"ask"}}\n' | base64)
RUN_ARGS=(--print foo)
run_entrypoint AGENT_TYPE=claude "SAFE_AGENTIC_CLAUDE_CONFIG_B64=$CLAUDE_B64_NEW"
assert_status 0 "claude config B64 preserve exits cleanly"
assert_contains "$(cat "$HOME_DIR/.claude/settings.json")" '"defaultMode":"bypassPermissions"' "existing claude config preserved over B64"
assert_not_contains "$(cat "$HOME_DIR/.claude/settings.json")" '"defaultMode":"ask"' "B64 claude config not written over existing"

# =============================================================================
# Test: Codex agent type only writes .codex config, not .claude
# =============================================================================
rm -f "$HOME_DIR/.codex/config.toml" "$HOME_DIR/.codex/auth.json"
rm -f "$HOME_DIR/.claude/settings.json"
RUN_ARGS=(fix)
run_entrypoint AGENT_TYPE=codex
assert_status 0 "codex-only config exits cleanly"
assert_contains "$(cat "$HOME_DIR/.codex/config.toml")" 'approval_policy = "never"' "codex config written for codex agent"
if [ -f "$HOME_DIR/.claude/settings.json" ]; then
  echo "FAIL: codex agent should not create .claude config" >&2
  ((++fail))
else
  ((++pass))
fi

# =============================================================================
# Test: Claude agent type only writes .claude config, not .codex
# =============================================================================
rm -f "$HOME_DIR/.codex/config.toml" "$HOME_DIR/.codex/auth.json"
rm -f "$HOME_DIR/.claude/settings.json"
RUN_ARGS=(--print foo)
run_entrypoint AGENT_TYPE=claude
assert_status 0 "claude-only config exits cleanly"
assert_contains "$(cat "$HOME_DIR/.claude/settings.json")" '"defaultMode": "bypassPermissions"' "claude config written for claude agent"
assert_contains "$(cat "$HOME_DIR/.claude/.claude.json")" '"firstStartTime"' "claude legacy metadata written for claude agent"
if [ -f "$HOME_DIR/.codex/config.toml" ]; then
  echo "FAIL: claude agent should not create .codex config" >&2
  ((++fail))
else
  ((++pass))
fi

# =============================================================================
# Test: SAFE_AGENTIC_CLAUDE_SUPPORT_B64 restores Claude support files
# =============================================================================
rm -rf "$HOME_DIR/.claude/hooks" "$HOME_DIR/.claude/commands"
rm -f "$HOME_DIR/.claude/statusline-command.sh" "$HOME_DIR/.claude/CLAUDE.md"
SUPPORT_DIR="$TMP_DIR/claude-support"
mkdir -p "$SUPPORT_DIR/hooks" "$SUPPORT_DIR/commands"
printf '#!/bin/bash\necho ok\n' >"$SUPPORT_DIR/hooks/check-linear-ticket.sh"
printf 'status\n' >"$SUPPORT_DIR/statusline-command.sh"
printf '# note\n' >"$SUPPORT_DIR/CLAUDE.md"
printf '# prove\n' >"$SUPPORT_DIR/commands/prove.md"
CLAUDE_SUPPORT_B64=$(tar -C "$SUPPORT_DIR" -czf - hooks commands statusline-command.sh CLAUDE.md | base64 | tr -d '\n')
RUN_ARGS=(--print foo)
run_entrypoint AGENT_TYPE=claude "SAFE_AGENTIC_CLAUDE_SUPPORT_B64=$CLAUDE_SUPPORT_B64"
assert_status 0 "claude support B64 exits cleanly"
assert_contains "$(cat "$HOME_DIR/.claude/hooks/check-linear-ticket.sh")" 'echo ok' "claude hook restored from support B64"
assert_contains "$(cat "$HOME_DIR/.claude/statusline-command.sh")" 'status' "claude statusline restored from support B64"
assert_contains "$(cat "$HOME_DIR/.claude/commands/prove.md")" '# prove' "claude commands restored from support B64"
assert_contains "$(cat "$HOME_DIR/.claude/CLAUDE.md")" '# note' "claude CLAUDE.md restored from support B64"

# =============================================================================
# Test: Claude legacy metadata restores from latest backup when missing
# =============================================================================
mkdir -p "$HOME_DIR/.claude/backups"
cat >"$HOME_DIR/.claude/backups/.claude.json.backup.2" <<'EOF'
{"firstStartTime":"2026-04-09T10:16:04.180Z"}
EOF
rm -f "$HOME_DIR/.claude/.claude.json"
RUN_ARGS=(--print foo)
run_entrypoint AGENT_TYPE=claude
assert_status 0 "claude legacy metadata backup restore exits cleanly"
assert_contains "$(cat "$HOME_DIR/.claude/.claude.json")" '2026-04-09T10:16:04.180Z' "claude legacy metadata restored from backup"

# =============================================================================
# Test: Security preamble creates CLAUDE.md when none exists
# =============================================================================
rm -f "$HOME_DIR/.claude/CLAUDE.md" "$HOME_DIR/.claude/settings.json"
rm -f "$HOME_DIR/.codex/config.toml" "$HOME_DIR/.codex/auth.json"
PREAMBLE_FILE="$TMP_DIR/security-preamble.md"
cat >"$PREAMBLE_FILE" <<'EOF'
<!-- safe-agentic:security-preamble -->
# Container Security Context
- SSH agent: {{SSH_STATUS}}
- AWS credentials: {{AWS_STATUS}}
- Network: {{NETWORK_STATUS}}
- Docker: {{DOCKER_STATUS}}
- Resources: {{RESOURCES}}
<!-- /safe-agentic:security-preamble -->
EOF
RUN_ARGS=(--print foo)
run_entrypoint AGENT_TYPE=claude \
  "SAFE_AGENTIC_PREAMBLE_TEMPLATE=$PREAMBLE_FILE" \
  "SAFE_AGENTIC_NETWORK_MODE=managed" \
  "SAFE_AGENTIC_DOCKER_MODE=off" \
  "SAFE_AGENTIC_RESOURCES=memory=8g,cpus=4,pids=512"
assert_status 0 "security preamble creates CLAUDE.md exits cleanly"
assert_contains "$(cat "$HOME_DIR/.claude/CLAUDE.md")" "safe-agentic:security-preamble" "security preamble marker present in CLAUDE.md"
assert_contains "$(cat "$HOME_DIR/.claude/CLAUDE.md")" "SSH agent: disabled" "SSH disabled by default in preamble"
assert_contains "$(cat "$HOME_DIR/.claude/CLAUDE.md")" "Network: managed" "network mode substituted in preamble"
assert_contains "$(cat "$HOME_DIR/.claude/CLAUDE.md")" "Docker: not available" "docker mode substituted in preamble"
assert_contains "$(cat "$HOME_DIR/.claude/CLAUDE.md")" "Resources: memory=8g,cpus=4,pids=512" "resources substituted in preamble"

# =============================================================================
# Test: Security preamble appends to existing CLAUDE.md
# =============================================================================
rm -f "$HOME_DIR/.claude/settings.json"
cat >"$HOME_DIR/.claude/CLAUDE.md" <<'EOF'
# My Project Notes
Important user instructions here.
EOF
RUN_ARGS=(--print foo)
run_entrypoint AGENT_TYPE=claude \
  "SAFE_AGENTIC_PREAMBLE_TEMPLATE=$PREAMBLE_FILE" \
  "SAFE_AGENTIC_SSH_ENABLED=1" \
  "SAFE_AGENTIC_NETWORK_MODE=none" \
  "SAFE_AGENTIC_DOCKER_MODE=dind"
assert_status 0 "security preamble append exits cleanly"
assert_contains "$(cat "$HOME_DIR/.claude/CLAUDE.md")" "My Project Notes" "existing CLAUDE.md content preserved"
assert_contains "$(cat "$HOME_DIR/.claude/CLAUDE.md")" "safe-agentic:security-preamble" "security preamble appended to existing CLAUDE.md"
assert_contains "$(cat "$HOME_DIR/.claude/CLAUDE.md")" "SSH agent: enabled" "SSH enabled reflected in preamble"
assert_contains "$(cat "$HOME_DIR/.claude/CLAUDE.md")" "Network: none" "network none reflected in preamble"
assert_contains "$(cat "$HOME_DIR/.claude/CLAUDE.md")" "Docker: internal daemon" "docker dind reflected in preamble"

# =============================================================================
# Test: Security preamble not double-injected on re-run
# =============================================================================
RUN_ARGS=(--print foo)
run_entrypoint AGENT_TYPE=claude \
  "SAFE_AGENTIC_PREAMBLE_TEMPLATE=$PREAMBLE_FILE" \
  "SAFE_AGENTIC_NETWORK_MODE=managed" \
  "SAFE_AGENTIC_DOCKER_MODE=off"
assert_status 0 "security preamble double-inject prevention exits cleanly"
# Count occurrences of the marker — should be exactly 1 (from first injection)
marker_count=$(grep -c "safe-agentic:security-preamble" "$HOME_DIR/.claude/CLAUDE.md" || true)
if [ "$marker_count" -eq 2 ]; then
  ((++pass))
else
  echo "FAIL: expected exactly 2 marker lines (open+close), got $marker_count" >&2
  ((++fail))
fi

# =============================================================================
# Test: Security preamble creates AGENTS.md for Codex
# =============================================================================
rm -f "$HOME_DIR/.codex/AGENTS.md" "$HOME_DIR/.codex/config.toml"
rm -f "$HOME_DIR/.codex/auth.json"
RUN_ARGS=(fix)
run_entrypoint AGENT_TYPE=codex \
  "SAFE_AGENTIC_PREAMBLE_TEMPLATE=$PREAMBLE_FILE" \
  "SAFE_AGENTIC_SSH_ENABLED=1" \
  "SAFE_AGENTIC_NETWORK_MODE=custom" \
  "SAFE_AGENTIC_DOCKER_MODE=host-socket" \
  "SAFE_AGENTIC_RESOURCES=memory=16g,cpus=8,pids=1024"
assert_status 0 "security preamble creates AGENTS.md exits cleanly"
assert_contains "$(cat "$HOME_DIR/.codex/AGENTS.md")" "safe-agentic:security-preamble" "security preamble marker present in AGENTS.md"
assert_contains "$(cat "$HOME_DIR/.codex/AGENTS.md")" "SSH agent: enabled" "SSH enabled in AGENTS.md preamble"
assert_contains "$(cat "$HOME_DIR/.codex/AGENTS.md")" "Network: custom" "network custom in AGENTS.md preamble"
assert_contains "$(cat "$HOME_DIR/.codex/AGENTS.md")" "Docker: host daemon" "docker host-socket in AGENTS.md preamble"
assert_contains "$(cat "$HOME_DIR/.codex/AGENTS.md")" "memory=16g,cpus=8,pids=1024" "resources in AGENTS.md preamble"

# =============================================================================
# Test: Security preamble skipped when template file missing
# =============================================================================
rm -f "$HOME_DIR/.claude/CLAUDE.md" "$HOME_DIR/.claude/settings.json"
RUN_ARGS=(--print foo)
run_entrypoint AGENT_TYPE=claude \
  "SAFE_AGENTIC_PREAMBLE_TEMPLATE=/nonexistent/path.md"
assert_status 0 "missing template exits cleanly"
if [ -f "$HOME_DIR/.claude/CLAUDE.md" ]; then
  echo "FAIL: CLAUDE.md should not be created when template is missing" >&2
  ((++fail))
else
  ((++pass))
fi

# =============================================================================
# Test: AWS credentials reflected in security preamble
# =============================================================================
rm -f "$HOME_DIR/.claude/CLAUDE.md" "$HOME_DIR/.claude/settings.json"
RUN_ARGS=(--print foo)
run_entrypoint AGENT_TYPE=claude \
  "SAFE_AGENTIC_PREAMBLE_TEMPLATE=$PREAMBLE_FILE" \
  "SAFE_AGENTIC_AWS_CREDS_B64=dGVzdA==" \
  "AWS_PROFILE=my-profile" \
  "SAFE_AGENTIC_NETWORK_MODE=managed" \
  "SAFE_AGENTIC_DOCKER_MODE=off"
assert_status 0 "AWS preamble exits cleanly"
assert_contains "$(cat "$HOME_DIR/.claude/CLAUDE.md")" "AWS credentials: injected (profile: my-profile)" "AWS profile reflected in preamble"

echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
