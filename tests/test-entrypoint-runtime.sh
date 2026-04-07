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

mkdir -p "$FAKE_BIN" "$STATE_DIR" "$RUN_DIR" "$HOME_DIR/.codex"

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
if [ "${1:-}" = "-p" ] && [[ "${2:-}" == /workspace/* ]]; then
  exit 0
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

chmod +x "$FAKE_BIN/cp" "$FAKE_BIN/chmod" "$FAKE_BIN/mkdir" "$FAKE_BIN/git" "$FAKE_BIN/bash" "$FAKE_BIN/claude" "$FAKE_BIN/codex"

pass=0
fail=0

run_entrypoint() {
  : >"$GIT_LOG"
  : >"$EXEC_LOG"
  : >"$STDOUT_LOG"
  : >"$STDERR_LOG"

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

# --- env overrides flow into git config and Claude launch ---
rm -f "$HOME_DIR/.codex/auth.json"
RUN_ARGS=(--print foo)
run_entrypoint AGENT_TYPE=claude GIT_AUTHOR_NAME='Test Agent' GIT_AUTHOR_EMAIL='agent@example.com'
assert_status 0 "claude branch exits cleanly"
assert_contains "$(cat "$GIT_LOG")" "config --global user.name Test Agent" "claude custom git name"
assert_contains "$(cat "$GIT_LOG")" "config --global user.email agent@example.com" "claude custom git email"
assert_contains "$(cat "$EXEC_LOG")" "claude|$RUN_DIR|--dangerously-skip-permissions --print foo" "claude exec flags"

# --- codex first run triggers device auth, then full-auto ---
rm -f "$HOME_DIR/.codex/auth.json"
RUN_ARGS=(plan)
run_entrypoint AGENT_TYPE=codex
assert_status 0 "codex first run exits cleanly"
codex_first_log="$(cat "$EXEC_LOG")"
assert_contains "$codex_first_log" "codex|$RUN_DIR|login --device-auth" "codex first run login"
assert_contains "$codex_first_log" "codex|$RUN_DIR|--full-auto plan" "codex full-auto after login"

# --- existing codex auth skips login ---
printf '{}\n' >"$HOME_DIR/.codex/auth.json"
RUN_ARGS=(fix)
run_entrypoint AGENT_TYPE=codex
assert_status 0 "codex with auth exits cleanly"
codex_existing_log="$(cat "$EXEC_LOG")"
assert_not_contains "$codex_existing_log" "login --device-auth" "codex skips login with auth"
assert_contains "$codex_existing_log" "codex|$RUN_DIR|--full-auto fix" "codex exec with auth"

# --- multi-repo clone trims whitespace, clones under /workspace, stays in run dir ---
rm -f "$HOME_DIR/.codex/auth.json"
RUN_ARGS=()
run_entrypoint REPOS=' https://github.com/acme/one.git , git@github.com:team/two.git '
assert_status 0 "multi repo exits cleanly"
multi_git_log="$(cat "$GIT_LOG")"
assert_contains "$multi_git_log" "clone https://github.com/acme/one.git /workspace/acme/one" "multi repo first clone"
assert_contains "$multi_git_log" "clone git@github.com:team/two.git /workspace/team/two" "multi repo second clone"
assert_contains "$(cat "$EXEC_LOG")" "bash|$RUN_DIR|-l" "multi repo stays outside /workspace for shell"

# --- unsafe repo URL is rejected before clone/exec ---
rm -f "$HOME_DIR/.codex/auth.json"
RUN_ARGS=()
run_entrypoint REPOS='git://github.com/acme/repo.git'
assert_status 1 "unsafe repo exits non-zero"
assert_contains "$(cat "$STDERR_LOG")" "Refusing repo URL with unsafe clone path" "unsafe repo rejection"
assert_not_contains "$(cat "$GIT_LOG")" "clone" "unsafe repo no clone"
assert_not_contains "$(cat "$EXEC_LOG")" "bash|" "unsafe repo no shell exec"

echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
