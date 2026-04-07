#!/usr/bin/env bash
# Tests that docker commands are constructed with correct security flags.
# Uses a fake orb/git to capture the docker run command without a real VM.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"
ERR_LOG="$TMP_DIR/error.log"
VERIFY_STATE="$TMP_DIR/verify-state"
mkdir -p "$FAKE_BIN"

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
    # Handle SSH_AUTH_SOCK query
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-c" ] && [[ "${3:-}" == *SSH_AUTH_SOCK* ]]; then
      echo "/tmp/fake-ssh.sock"; exit 0
    fi
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-lc" ] && [[ "${3:-}" == *safe-agentic-hardening-verify* ]]; then
      : >"$verify_state"
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "image" ] && [ "${3:-}" = "inspect" ]; then
      exit 0
    fi
    # Handle docker network inspect — managed networks pass, unknown fail
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "network" ]; then
      case "${3:-}" in
        inspect)
          case "${4:-}" in
            custom-net|none) exit 0 ;;
            *-net) exit 1 ;;  # managed networks don't pre-exist
            *) exit 1 ;;
          esac ;;
        create) exit 0 ;;
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

# Fake git config that returns a known identity
cat >"$FAKE_BIN/git" <<'GITEOF'
#!/usr/bin/env bash
if [ "${1:-}" = "config" ] && [ "${2:-}" = "user.name" ]; then
  echo "Test User"; exit 0
fi
if [ "${1:-}" = "config" ] && [ "${2:-}" = "user.email" ]; then
  echo "test@example.com"; exit 0
fi
# Delegate to real git for everything else
exec /usr/bin/git "$@"
GITEOF
chmod +x "$FAKE_BIN/git"

pass=0
fail=0

run_agent() {
  : >"$ORB_LOG"
  : >"$VERIFY_STATE"
  PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" bash "$@"
}

run_agent_env() {
  : >"$ORB_LOG"
  : >"$VERIFY_STATE"
  PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" "$@"
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

assert_fails() {
  local label="$1"; shift
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" bash "$@" >"$ERR_LOG" 2>&1; then
    echo "FAIL: $label: expected non-zero exit" >&2
    ((++fail))
  else
    ((++pass))
  fi
}

# =============================================================================
# Test: Security hardening flags are always present
# =============================================================================
run_agent "$REPO_DIR/bin/agent" spawn claude --name sec --repo https://github.com/a/b.git >/dev/null 2>&1
run="$(last_docker_run)"

assert_contains "$run" "--cap-drop=ALL"                       "cap-drop"
assert_contains "$run" "--security-opt=no-new-privileges:true" "no-new-privileges"
assert_contains "$run" "--read-only"                           "read-only rootfs"
assert_contains "$run" "--pull=never"                          "local image only"
assert_contains "$run" "--memory 8g"                           "memory limit"
assert_contains "$run" "--cpus 4"                              "cpu limit"
assert_contains "$run" "--pids-limit 512"                      "pids limit"
assert_contains "$run" "--security-opt seccomp=/etc/safe-agentic/seccomp.json" "custom seccomp profile"
assert_not_contains "$run" "--cap-add"                         "no cap-add"
assert_not_contains "$run" "--privileged"                      "no privileged"

# =============================================================================
# Test: Tmpfs mounts have noexec,nosuid
# =============================================================================
assert_contains "$run" "--tmpfs /tmp:rw,noexec,nosuid"             "tmp noexec"
assert_contains "$run" "--tmpfs /var/tmp:rw,noexec,nosuid"         "var/tmp noexec"
assert_contains "$run" "--tmpfs /run:rw,noexec,nosuid"             "run noexec"
assert_contains "$run" "--tmpfs /dev/shm:rw,noexec,nosuid"         "shm noexec"
assert_contains "$run" "--tmpfs /home/agent/.ssh:rw,noexec,nosuid" "ssh noexec"
assert_contains "$run" "--ulimit nofile=65536:65536"                "nofile ulimit"

# =============================================================================
# Test: Ephemeral volumes use anonymous Docker volumes
# =============================================================================
assert_contains "$run" "--mount type=volume,dst=/workspace"               "ephemeral workspace"
assert_contains "$run" "--mount type=volume,dst=/home/agent/.npm"         "ephemeral npm cache"
assert_contains "$run" "--mount type=volume,dst=/home/agent/.cache/pip"   "ephemeral pip cache"
assert_contains "$run" "--mount type=volume,dst=/home/agent/go"           "ephemeral go cache"
assert_contains "$run" "--mount type=volume,dst=/home/agent/.claude"      "ephemeral claude auth"
assert_not_contains "$run" "volume-nocopy"                                "no volume-nocopy"

# =============================================================================
# Test: Per-container network is created
# =============================================================================
assert_contains "$run" "--network agent-claude-sec-net"  "managed network"
assert_contains "$(cat "$ORB_LOG")" "--opt com.docker.network.bridge.name=" "managed bridge named"

# =============================================================================
# Test: Git identity is not leaked from host defaults
# =============================================================================
assert_not_contains "$run" "GIT_AUTHOR_NAME=Test User"         "no host author leak"
assert_not_contains "$run" "GIT_COMMITTER_NAME=Test User"      "no host committer leak"
assert_not_contains "$run" "GIT_AUTHOR_EMAIL=test@example.com" "no host author email leak"
assert_not_contains "$run" "GIT_COMMITTER_EMAIL=test@example.com" "no host committer email leak"
assert_contains "$run" "GIT_CONFIG_GLOBAL=/home/agent/.config/git/config" "git config path"
assert_not_contains "$run" "--tmpfs /home/agent/.gitconfig" "no file tmpfs mount"

GIT_AUTHOR_NAME="Explicit User" GIT_AUTHOR_EMAIL="explicit@example.com" \
  run_agent_env bash "$REPO_DIR/bin/agent" spawn claude --name gitenv --repo https://github.com/a/b.git >/dev/null 2>&1
gitenv_run="$(last_docker_run)"
assert_contains "$gitenv_run" "GIT_AUTHOR_NAME=Explicit User" "explicit author forwarded"
assert_contains "$gitenv_run" "GIT_AUTHOR_EMAIL=explicit@example.com" "explicit email forwarded"

# =============================================================================
# Test: AGENT_TYPE is set for spawn, absent for shell
# =============================================================================
assert_contains "$run" "AGENT_TYPE=claude"  "agent type set"

run_agent "$REPO_DIR/bin/agent" shell >/dev/null 2>&1
shell_run="$(last_docker_run)"
assert_not_contains "$shell_run" "AGENT_TYPE="  "no agent type for shell"

# =============================================================================
# Test: SSH agent NOT forwarded by default
# =============================================================================
run_agent "$REPO_DIR/bin/agent" spawn claude --name nossh --repo https://github.com/a/b.git >/dev/null 2>&1
nossh_run="$(last_docker_run)"
assert_not_contains "$nossh_run" "SSH_AUTH_SOCK"  "no ssh by default"

# =============================================================================
# Test: SSH agent IS forwarded with --ssh
# =============================================================================
run_agent "$REPO_DIR/bin/agent" spawn claude --ssh --name withssh --repo git@github.com:a/b.git >/dev/null 2>&1
ssh_run="$(last_docker_run)"
assert_contains "$ssh_run" "SSH_AUTH_SOCK=/run/ssh-agent.sock"  "ssh forwarded with --ssh"

# =============================================================================
# Test: --reuse-auth uses named volume, not ephemeral
# =============================================================================
run_agent "$REPO_DIR/bin/agent" spawn claude --reuse-auth --name reauth --repo https://github.com/a/b.git >/dev/null 2>&1
reauth_run="$(last_docker_run)"
assert_contains "$reauth_run" "src=agent-claude-auth,dst=/home/agent/.claude"  "reuse-auth named volume"

run_agent "$REPO_DIR/bin/agent" spawn codex --reuse-auth --name recodex --repo https://github.com/a/b.git >/dev/null 2>&1
recodex_run="$(last_docker_run)"
assert_contains "$recodex_run" "src=agent-codex-auth,dst=/home/agent/.codex"  "reuse-auth codex volume"

# =============================================================================
# Test: Custom resource limits
# =============================================================================
run_agent "$REPO_DIR/bin/agent" spawn claude --name limits --memory 16g --cpus 8 --pids-limit 1024 --repo https://github.com/a/b.git >/dev/null 2>&1
limits_run="$(last_docker_run)"
assert_contains "$limits_run" "--memory 16g"      "custom memory"
assert_contains "$limits_run" "--cpus 8"          "custom cpus"
assert_contains "$limits_run" "--pids-limit 1024" "custom pids"

# =============================================================================
# Test: Reject dangerous arguments
# =============================================================================
assert_fails "-- passthrough blocked"          "$REPO_DIR/bin/agent" spawn claude -- --privileged
assert_fails "unknown flag blocked"            "$REPO_DIR/bin/agent" spawn claude --unknown-flag
assert_fails "host network blocked"            "$REPO_DIR/bin/agent" spawn claude --network host
assert_fails "bridge network blocked"          "$REPO_DIR/bin/agent" spawn claude --network bridge
assert_fails "container network mode blocked"  "$REPO_DIR/bin/agent" spawn claude --network container:foo
assert_fails "missing agent type"              "$REPO_DIR/bin/agent" spawn

# =============================================================================
# Test: Container name validation
# =============================================================================
assert_fails "name with semicolon"   "$REPO_DIR/bin/agent" spawn claude --name 'bad;name' --repo https://github.com/a/b.git
assert_fails "name with space"       "$REPO_DIR/bin/agent" spawn claude --name 'bad name' --repo https://github.com/a/b.git
assert_fails "name with dollar"      "$REPO_DIR/bin/agent" spawn claude --name 'bad$name' --repo https://github.com/a/b.git

# =============================================================================
# Test: Shell command has same hardening as spawn
# =============================================================================
run_agent "$REPO_DIR/bin/agent" shell >/dev/null 2>&1
shell_run="$(last_docker_run)"
assert_contains "$shell_run" "--cap-drop=ALL"                       "shell: cap-drop"
assert_contains "$shell_run" "--security-opt=no-new-privileges:true" "shell: no-new-privileges"
assert_contains "$shell_run" "--read-only"                           "shell: read-only"
assert_contains "$shell_run" "--memory 8g"                           "shell: memory limit"

# =============================================================================
# Test: Codex agent type gets correct auth volume path
# =============================================================================
run_agent "$REPO_DIR/bin/agent" spawn codex --name cdx --repo https://github.com/a/b.git >/dev/null 2>&1
codex_run="$(last_docker_run)"
assert_contains "$codex_run" "AGENT_TYPE=codex"                                "codex agent type"
assert_contains "$codex_run" "--mount type=volume,dst=/home/agent/.codex"      "codex ephemeral auth"
assert_not_contains "$codex_run" "/home/agent/.claude"                         "codex no claude path"

# =============================================================================
# Test: Multiple repos are comma-joined in REPOS env var
# =============================================================================
run_agent "$REPO_DIR/bin/agent" spawn claude --name multi --repo https://github.com/a/one.git --repo https://github.com/a/two.git >/dev/null 2>&1
multi_run="$(last_docker_run)"
assert_contains "$multi_run" "REPOS=https://github.com/a/one.git,https://github.com/a/two.git" "multi repos joined"

# =============================================================================
# Test: No repos means no REPOS env var
# =============================================================================
run_agent "$REPO_DIR/bin/agent" spawn claude --name norepo >/dev/null 2>&1
norepo_run="$(last_docker_run)"
assert_not_contains "$norepo_run" "REPOS=" "no repos no env var"

# =============================================================================
# Test: SSH socket mount is read-only (:ro)
# =============================================================================
run_agent "$REPO_DIR/bin/agent" spawn claude --ssh --name sshro --repo git@github.com:a/b.git >/dev/null 2>&1
sshro_run="$(last_docker_run)"
assert_contains "$sshro_run" "/run/ssh-agent.sock:ro" "ssh socket mounted read-only"

# =============================================================================
# Test: Shell with repos gets REPOS env var
# =============================================================================
run_agent "$REPO_DIR/bin/agent" shell --repo https://github.com/a/b.git >/dev/null 2>&1
shell_repo_run="$(last_docker_run)"
assert_contains "$shell_repo_run" "REPOS=https://github.com/a/b.git" "shell passes repos"

# =============================================================================
# Test: Shell with --ssh forwards agent
# =============================================================================
run_agent "$REPO_DIR/bin/agent" shell --ssh >/dev/null 2>&1
shell_ssh_run="$(last_docker_run)"
assert_contains "$shell_ssh_run" "SSH_AUTH_SOCK=/run/ssh-agent.sock" "shell ssh forwarding"

# =============================================================================
# Test: Image name is the last argument (after all flags)
# =============================================================================
assert_contains "$codex_run" "safe-agentic:latest" "image name present"

# =============================================================================
# Test: GIT_CONFIG_GLOBAL env var is always set
# =============================================================================
assert_contains "$codex_run" "GIT_CONFIG_GLOBAL=/home/agent/.config/git/config" "codex git config path"

echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
