#!/usr/bin/env bash
# Tests for git identity auto-detection from host ~/.gitconfig.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"
VERIFY_STATE="$TMP_DIR/verify-state"
FAKE_HOME="$TMP_DIR/home"
mkdir -p "$FAKE_BIN"
mkdir -p "$FAKE_HOME/.config/safe-agentic"

# Fake orb that logs docker commands
cat >"$FAKE_BIN/orb" <<'ORBEOF'
#!/usr/bin/env bash
set -euo pipefail
log_file="${TEST_ORB_LOG:?}"
verify_state="${TEST_VERIFY_STATE:?}"
cmd="${1:-}"
shift || true
case "$cmd" in
  list) echo "safe-agentic" ;;
  run)
    [ "${1:-}" = "-m" ] && shift 2
    printf '%s\n' "$*" >>"$log_file"
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-c" ] && [[ "${3:-}" == *SSH_AUTH_SOCK* ]]; then
      echo "/tmp/fake-ssh.sock"; exit 0
    fi
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-c" ] && [[ "${3:-}" == *socat* ]]; then
      exit 0
    fi
    if [ "${1:-}" = "test" ] && [ "${2:-}" = "-S" ]; then
      exit 0
    fi
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-lc" ] && [[ "${3:-}" == *safe-agentic-hardening-verify* ]]; then
      : >"$verify_state"
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "image" ] && [ "${3:-}" = "inspect" ]; then
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "volume" ]; then
      exit 1
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "network" ]; then
      case "${3:-}" in
        create) exit 0 ;;
        inspect) exit 1 ;;
        rm) exit 0 ;;
      esac
    fi
    exit 0
    ;;
  push|start|stop|create|ssh) ;;
  *) echo "unexpected orb command: $cmd" >&2; exit 1 ;;
esac
ORBEOF
chmod +x "$FAKE_BIN/orb"

# Fake git that returns a known identity from --global config
cat >"$FAKE_BIN/git" <<'GITEOF'
#!/usr/bin/env bash
if [ "${1:-}" = "config" ] && [ "${2:-}" = "--global" ] && [ "${3:-}" = "user.name" ]; then
  echo "Auto User"; exit 0
fi
if [ "${1:-}" = "config" ] && [ "${2:-}" = "--global" ] && [ "${3:-}" = "user.email" ]; then
  echo "auto@example.com"; exit 0
fi
if [ "${1:-}" = "config" ] && [ "${2:-}" = "user.name" ]; then
  echo "Auto User"; exit 0
fi
if [ "${1:-}" = "config" ] && [ "${2:-}" = "user.email" ]; then
  echo "auto@example.com"; exit 0
fi
exec /usr/bin/git "$@"
GITEOF
chmod +x "$FAKE_BIN/git"

pass=0
fail=0

run_agent() {
  : >"$ORB_LOG"
  : >"$VERIFY_STATE"
  PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" HOME="$FAKE_HOME" XDG_CONFIG_HOME="$FAKE_HOME/.config" bash "$@"
}

run_agent_env() {
  : >"$ORB_LOG"
  : >"$VERIFY_STATE"
  PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" HOME="$FAKE_HOME" XDG_CONFIG_HOME="$FAKE_HOME/.config" "$@"
}

last_docker_run() {
  grep 'docker run ' "$ORB_LOG" | tail -n 1
}

assert_contains() {
  local haystack="$1" needle="$2" label="${3:-}"
  if [[ "$haystack" == *"$needle"* ]]; then
    ((++pass))
  else
    echo "FAIL${label:+: $label}: missing '$needle'" >&2
    echo "  in: $haystack" >&2
    ((++fail))
  fi
}

assert_not_contains() {
  local haystack="$1" needle="$2" label="${3:-}"
  if [[ "$haystack" != *"$needle"* ]]; then
    ((++pass))
  else
    echo "FAIL${label:+: $label}: unexpected '$needle'" >&2
    echo "  in: $haystack" >&2
    ((++fail))
  fi
}

# =============================================================================
# Test 1: Auto-detect git identity when no --identity flag
# =============================================================================
run_agent "$REPO_DIR/bin/agent" spawn claude --name autodetect --repo https://github.com/a/b.git >/dev/null 2>&1
run="$(last_docker_run)"
assert_contains "$run" "GIT_AUTHOR_NAME=Auto User"         "auto-detect author name"
assert_contains "$run" "GIT_AUTHOR_EMAIL=auto@example.com"  "auto-detect author email"
assert_contains "$run" "GIT_COMMITTER_NAME=Auto User"       "auto-detect committer name"
assert_contains "$run" "GIT_COMMITTER_EMAIL=auto@example.com" "auto-detect committer email"

# =============================================================================
# Test 2: Explicit --identity overrides auto-detection
# =============================================================================
run_agent "$REPO_DIR/bin/agent" spawn claude --name explicit --identity "Explicit User <explicit@example.com>" --repo https://github.com/a/b.git >/dev/null 2>&1
explicit_run="$(last_docker_run)"
assert_contains "$explicit_run" "GIT_AUTHOR_NAME=Explicit User"         "explicit identity author"
assert_contains "$explicit_run" "GIT_AUTHOR_EMAIL=explicit@example.com"  "explicit identity email"
assert_contains "$explicit_run" "GIT_COMMITTER_NAME=Explicit User"       "explicit identity committer"
assert_contains "$explicit_run" "GIT_COMMITTER_EMAIL=explicit@example.com" "explicit identity committer email"
assert_not_contains "$explicit_run" "Auto User"    "explicit overrides auto-detect name"
assert_not_contains "$explicit_run" "auto@example" "explicit overrides auto-detect email"

# =============================================================================
# Test 3: SAFE_AGENTIC_DEFAULT_IDENTITY overrides auto-detection
# =============================================================================
cat >"$FAKE_HOME/.config/safe-agentic/defaults.sh" <<'EOF'
SAFE_AGENTIC_DEFAULT_IDENTITY="Default User <default@example.com>"
EOF

run_agent "$REPO_DIR/bin/agent" spawn claude --name defaultid --repo https://github.com/a/b.git >/dev/null 2>&1
default_run="$(last_docker_run)"
assert_contains "$default_run" "GIT_AUTHOR_NAME=Default User"           "default identity author"
assert_contains "$default_run" "GIT_AUTHOR_EMAIL=default@example.com"   "default identity email"
assert_not_contains "$default_run" "Auto User"    "default overrides auto-detect name"
assert_not_contains "$default_run" "auto@example" "default overrides auto-detect email"

# Clean up defaults file
rm -f "$FAKE_HOME/.config/safe-agentic/defaults.sh"

# =============================================================================
# Test 4: GIT_AUTHOR_NAME env var overrides auto-detection
# =============================================================================
GIT_AUTHOR_NAME="Env User" GIT_AUTHOR_EMAIL="env@example.com" \
  run_agent_env bash "$REPO_DIR/bin/agent" spawn claude --name envid --repo https://github.com/a/b.git >/dev/null 2>&1
env_run="$(last_docker_run)"
assert_contains "$env_run" "GIT_AUTHOR_NAME=Env User"         "env var identity author"
assert_contains "$env_run" "GIT_AUTHOR_EMAIL=env@example.com"  "env var identity email"
assert_not_contains "$env_run" "Auto User"    "env var overrides auto-detect name"
assert_not_contains "$env_run" "auto@example" "env var overrides auto-detect email"

# =============================================================================
# Test 5: No git config returns empty — no identity injected
# =============================================================================
NO_GIT_BIN="$TMP_DIR/nobin"
mkdir -p "$NO_GIT_BIN"
cat >"$NO_GIT_BIN/git" <<'GITEOF'
#!/usr/bin/env bash
if [ "${1:-}" = "config" ] && [[ "${*}" == *"user.name"* ]]; then
  exit 1
fi
if [ "${1:-}" = "config" ] && [[ "${*}" == *"user.email"* ]]; then
  exit 1
fi
exec /usr/bin/git "$@"
GITEOF
chmod +x "$NO_GIT_BIN/git"

# Copy orb into the no-git bin dir
cp "$FAKE_BIN/orb" "$NO_GIT_BIN/orb"

: >"$ORB_LOG"
: >"$VERIFY_STATE"
PATH="$NO_GIT_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" HOME="$FAKE_HOME" XDG_CONFIG_HOME="$FAKE_HOME/.config" \
  bash "$REPO_DIR/bin/agent" spawn claude --name nogit --repo https://github.com/a/b.git >/dev/null 2>&1
nogit_run="$(last_docker_run)"
assert_not_contains "$nogit_run" "GIT_AUTHOR_NAME="    "no identity without gitconfig"
assert_not_contains "$nogit_run" "GIT_AUTHOR_EMAIL="   "no email without gitconfig"
assert_not_contains "$nogit_run" "GIT_COMMITTER_NAME=" "no committer without gitconfig"
assert_not_contains "$nogit_run" "GIT_COMMITTER_EMAIL=" "no committer email without gitconfig"

# =============================================================================
# Test 6: detect_git_identity function returns correct format
# =============================================================================
detect_output=$(PATH="$FAKE_BIN:$PATH" bash -c 'source "'"$REPO_DIR"'/bin/agent-lib.sh"; detect_git_identity')
if [ "$detect_output" = "Auto User <auto@example.com>" ]; then
  ((++pass))
else
  echo "FAIL: detect_git_identity: expected 'Auto User <auto@example.com>' got '$detect_output'" >&2
  ((++fail))
fi

# Test detect_git_identity returns empty with no config
detect_empty=$(PATH="$NO_GIT_BIN:$PATH" bash -c 'source "'"$REPO_DIR"'/bin/agent-lib.sh"; detect_git_identity')
if [ -z "$detect_empty" ]; then
  ((++pass))
else
  echo "FAIL: detect_git_identity empty: expected '' got '$detect_empty'" >&2
  ((++fail))
fi

echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
