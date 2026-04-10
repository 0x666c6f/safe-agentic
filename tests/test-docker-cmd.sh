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
OUT_LOG="$TMP_DIR/out.log"
VERIFY_STATE="$TMP_DIR/verify-state"
DEFAULT_HOME="$TMP_DIR/home"
mkdir -p "$FAKE_BIN"
mkdir -p "$DEFAULT_HOME/.config/safe-agentic"

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
    # Handle socat relay setup (pretend it succeeds)
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-c" ] && [[ "${3:-}" == *socat* ]]; then
      exit 0
    fi
    # Handle test -S for relay socket (pretend it exists)
    if [ "${1:-}" = "test" ] && [ "${2:-}" = "-S" ]; then
      exit 0
    fi
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-c" ] && [[ "${3:-}" == *'test -S /var/run/docker.sock'*'printf "%s\n" /var/run/docker.sock'* ]]; then
      echo "/var/run/docker.sock"; exit 0
    fi
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-c" ] && [[ "${3:-}" == "stat -c %g /var/run/docker.sock" ]]; then
      echo "998"; exit 0
    fi
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-lc" ] && [[ "${3:-}" == *safe-agentic-hardening-verify* ]]; then
      : >"$verify_state"
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "image" ] && [ "${3:-}" = "inspect" ]; then
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "volume" ]; then
      case "${3:-}" in
        inspect)
          if [ "${TEST_CODEX_AUTH_READY:-0}" = "1" ]; then
            if [ "${4:-}" = "--format" ]; then
              echo "/var/lib/docker/volumes/agent-codex-auth/_data"
            fi
            exit 0
          fi
          exit 1
          ;;
        create|rm) exit 0 ;;
      esac
    fi
    if [ "${1:-}" = "test" ] && [ "${2:-}" = "-f" ] && [[ "${3:-}" == */agent-codex-auth/_data/auth.json ]]; then
      [ "${TEST_CODEX_AUTH_READY:-0}" = "1" ] && exit 0
      exit 1
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "exec" ] && [ "${4:-}" = "docker" ] && [ "${5:-}" = "--host" ] && [[ "${6:-}" == *safe-agentic-docker/docker.sock ]] && [ "${7:-}" = "info" ]; then
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "logs" ]; then
      echo "dockerd ok"
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

run_agent "$REPO_DIR/bin/agent" spawn claude --name identity --identity "CLI User <cli@example.com>" --repo https://github.com/a/b.git >/dev/null 2>&1
identity_run="$(last_docker_run)"
assert_contains "$identity_run" "GIT_AUTHOR_NAME=CLI User" "identity flag author"
assert_contains "$identity_run" "GIT_AUTHOR_EMAIL=cli@example.com" "identity flag email"
assert_contains "$identity_run" "GIT_COMMITTER_NAME=CLI User" "identity flag committer"
assert_contains "$identity_run" "GIT_COMMITTER_EMAIL=cli@example.com" "identity flag committer email"

cat >"$DEFAULT_HOME/.config/safe-agentic/defaults.sh" <<'EOF'
SAFE_AGENTIC_DEFAULT_MEMORY=12g
SAFE_AGENTIC_DEFAULT_CPUS=6
SAFE_AGENTIC_DEFAULT_NETWORK=custom-net
SAFE_AGENTIC_DEFAULT_REUSE_AUTH=true
SAFE_AGENTIC_DEFAULT_REUSE_GH_AUTH=true
SAFE_AGENTIC_DEFAULT_DOCKER=true
SAFE_AGENTIC_DEFAULT_IDENTITY="Default User <default@example.com>"
EOF

HOME="$DEFAULT_HOME" XDG_CONFIG_HOME="$DEFAULT_HOME/.config" \
  run_agent_env bash "$REPO_DIR/bin/agent" spawn claude --name defaults --repo https://github.com/a/b.git >/dev/null 2>&1
defaults_run="$(last_docker_run)"
assert_contains "$defaults_run" "--memory 12g" "defaults memory"
assert_contains "$defaults_run" "--cpus 6" "defaults cpus"
assert_contains "$defaults_run" "--network custom-net" "defaults network"
assert_contains "$defaults_run" "src=agent-claude-auth,dst=/home/agent/.claude" "defaults reuse-auth"
assert_contains "$defaults_run" "src=agent-gh-auth,dst=/home/agent/.config/gh" "defaults gh auth"
assert_contains "$defaults_run" "DOCKER_HOST=unix:///run/safe-agentic-docker/docker.sock" "defaults docker host"
assert_contains "$defaults_run" "src=agent-claude-defaults-docker-sock,dst=/run/safe-agentic-docker" "defaults docker socket volume"
assert_contains "$defaults_run" "GIT_AUTHOR_NAME=Default User" "defaults identity"
assert_contains "$(cat "$ORB_LOG")" "docker run -d --name safe-agentic-docker-agent-claude-defaults" "defaults docker sidecar"

BAD_HOME="$TMP_DIR/bad-home"
BAD_MARKER="$TMP_DIR/defaults-ran"
mkdir -p "$BAD_HOME/.config/safe-agentic"
cat >"$BAD_HOME/.config/safe-agentic/defaults.sh" <<EOF
SAFE_AGENTIC_DEFAULT_MEMORY=14g
touch "$BAD_MARKER"
EOF

if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" HOME="$BAD_HOME" XDG_CONFIG_HOME="$BAD_HOME/.config" \
  bash "$REPO_DIR/bin/agent" spawn claude --name bad-defaults --repo https://github.com/a/b.git >"$OUT_LOG" 2>"$ERR_LOG"; then
  echo "FAIL: defaults parser should reject shell commands" >&2
  ((++fail))
else
  ((++pass))
fi
assert_not_contains "$(cat "$ERR_LOG")" "source " "defaults parser does not source shell"
assert_contains "$(cat "$ERR_LOG")" "Use simple KEY=value assignments only" "defaults parser error"
if [ -e "$BAD_MARKER" ]; then
  echo "FAIL: defaults parser executed shell code" >&2
  ((++fail))
else
  ((++pass))
fi

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

run_agent "$REPO_DIR/bin/agent" spawn claude --reuse-gh-auth --name regh --repo https://github.com/a/b.git >/dev/null 2>&1
regh_run="$(last_docker_run)"
assert_contains "$regh_run" "GH_CONFIG_DIR=/home/agent/.config/gh" "reuse-gh-auth config path"
assert_contains "$regh_run" "src=agent-gh-auth,dst=/home/agent/.config/gh" "reuse-gh-auth named volume"

# =============================================================================
# Test: Docker support can use DinD sidecar or host socket
# =============================================================================
run_agent "$REPO_DIR/bin/agent" spawn claude --docker --name dind --repo https://github.com/a/b.git >/dev/null 2>&1
dind_run="$(last_docker_run)"
dind_log="$(cat "$ORB_LOG")"
assert_contains "$dind_run" "DOCKER_HOST=unix:///run/safe-agentic-docker/docker.sock" "dind docker host"
assert_contains "$dind_run" "src=agent-claude-dind-docker-sock,dst=/run/safe-agentic-docker" "dind socket mount"
assert_contains "$dind_run" "--mount type=volume,dst=/home/agent/.docker" "dind docker cli state"
assert_contains "$dind_log" "docker run -d --name safe-agentic-docker-agent-claude-dind" "dind sidecar start"
assert_contains "$dind_log" "SAFE_AGENTIC_INTERNAL_DOCKERD=1" "dind sidecar env"
assert_contains "$dind_log" "docker exec safe-agentic-docker-agent-claude-dind docker --host unix:///run/safe-agentic-docker/docker.sock info" "dind readiness check"

run_agent "$REPO_DIR/bin/agent" spawn claude --docker-socket --name dockersock --repo https://github.com/a/b.git >/dev/null 2>&1
dockersock_run="$(last_docker_run)"
dockersock_log="$(cat "$ORB_LOG")"
assert_contains "$dockersock_run" "DOCKER_HOST=unix:///run/docker-host.sock" "docker-socket host env"
assert_contains "$dockersock_run" "-v /var/run/docker.sock:/run/docker-host.sock" "docker-socket bind mount"
assert_contains "$dockersock_run" "--group-add 998" "docker-socket group add"
assert_not_contains "$dockersock_log" "safe-agentic-docker-agent-claude-dockersock" "docker-socket skips dind sidecar"

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
# .claude tmpfs is present (entrypoint writes claude config even for codex agents)
# but no auth volume mount for .claude
assert_not_contains "$codex_run" "--mount type=volume,dst=/home/agent/.claude" "codex no claude auth volume"

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
# Test: SSH socket relay is mounted
# =============================================================================
run_agent "$REPO_DIR/bin/agent" spawn claude --ssh --name sshro --repo git@github.com:a/b.git >/dev/null 2>&1
sshro_run="$(last_docker_run)"
assert_contains "$sshro_run" "/run/ssh-agent.sock" "ssh relay socket mounted"
assert_contains "$sshro_run" "SSH_AUTH_SOCK=/run/ssh-agent.sock" "ssh auth sock env set"

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
# Test: Dry run prints command but does not execute docker run/create network
# =============================================================================
: >"$ORB_LOG"
PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" \
  bash "$REPO_DIR/bin/agent" spawn claude --dry-run --name preview --repo https://github.com/a/b.git >"$ERR_LOG" 2>&1
dry_log="$(cat "$ORB_LOG")"
assert_not_contains "$dry_log" "docker run " "dry-run skips docker run"
assert_not_contains "$dry_log" "docker network create" "dry-run skips network create"
assert_contains "$(cat "$ERR_LOG")" "Would run:" "dry-run prints docker command"

# =============================================================================
# Test: Dry run with --docker also previews sidecar without starting it
# =============================================================================
: >"$ORB_LOG"
PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" \
  bash "$REPO_DIR/bin/agent" spawn claude --dry-run --docker --name preview-dind --repo https://github.com/a/b.git >"$ERR_LOG" 2>&1
dry_docker_log="$(cat "$ORB_LOG")"
dry_docker_output="$(cat "$ERR_LOG")"
assert_not_contains "$dry_docker_log" "docker run -d --name safe-agentic-docker" "dry-run docker skips sidecar start"
assert_not_contains "$dry_docker_log" "docker network create" "dry-run docker skips network create"
assert_contains "$dry_docker_output" "Would start Docker sidecar:" "dry-run docker previews sidecar"
assert_contains "$dry_docker_output" "safe-agentic-docker-agent-claude-preview-dind" "dry-run docker sidecar name"

# =============================================================================
# Test: Shell supports Docker + persisted gh auth
# =============================================================================
run_agent "$REPO_DIR/bin/agent" shell --docker --reuse-gh-auth >/dev/null 2>&1
shell_docker_run="$(last_docker_run)"
shell_docker_log="$(cat "$ORB_LOG")"
assert_not_contains "$shell_docker_run" "AGENT_TYPE=" "shell docker no agent type"
assert_contains "$shell_docker_run" "src=agent-gh-auth,dst=/home/agent/.config/gh" "shell docker gh auth reuse"
assert_contains "$shell_docker_run" "DOCKER_HOST=unix:///run/safe-agentic-docker/docker.sock" "shell docker host"
assert_contains "$shell_docker_run" "--mount type=volume,dst=/home/agent/.docker" "shell docker cli state"
assert_contains "$shell_docker_log" "docker run -d --name safe-agentic-docker-agent-shell" "shell docker sidecar start"

# =============================================================================
# Test: Image name is the last argument (after all flags)
# =============================================================================
assert_contains "$codex_run" "safe-agentic:latest" "image name present"

# =============================================================================
# Test: GIT_CONFIG_GLOBAL env var is always set
# =============================================================================
assert_contains "$codex_run" "GIT_CONFIG_GLOBAL=/home/agent/.config/git/config" "codex git config path"

# =============================================================================
# Test: --prompt flag for codex appends positional arg after image name
# =============================================================================
run_agent "$REPO_DIR/bin/agent" spawn codex --prompt 'do stuff' --name p --repo https://github.com/a/b.git >/dev/null 2>&1
prompt_codex_run="$(last_docker_run)"
assert_contains "$prompt_codex_run" "safe-agentic:latest do stuff" "codex prompt as positional arg"

# =============================================================================
# Test: Codex background mode requires saved shared auth
# =============================================================================
: >"$ORB_LOG"
assert_fails "codex background without saved auth blocked" "$REPO_DIR/bin/agent" spawn codex --background --reuse-auth --name bg-noauth --repo https://github.com/a/b.git
assert_contains "$(cat "$ERR_LOG")" "Codex --background requires saved auth" "codex background auth guard message"
assert_not_contains "$(cat "$ORB_LOG")" "docker run " "codex background auth guard skips docker run"

TEST_CODEX_AUTH_READY=1 run_agent "$REPO_DIR/bin/agent" spawn codex --background --reuse-auth --name bg-auth --repo https://github.com/a/b.git >/dev/null 2>&1
bg_auth_run="$(last_docker_run)"
assert_contains "$bg_auth_run" "SAFE_AGENTIC_BACKGROUND=1" "codex background allowed with saved auth"

# =============================================================================
# Test: --prompt flag for claude appends -p "prompt" after image name
# =============================================================================
run_agent "$REPO_DIR/bin/agent" spawn claude --prompt 'do stuff' --name pc --repo https://github.com/a/b.git >/dev/null 2>&1
prompt_claude_run="$(last_docker_run)"
assert_contains "$prompt_claude_run" "safe-agentic:latest -p do stuff" "claude prompt with -p flag"

# =============================================================================
# Test: --prompt without value should error
# =============================================================================
assert_fails "prompt missing value" "$REPO_DIR/bin/agent" spawn codex --prompt

# =============================================================================
# Test: Container persistence — docker run does NOT contain --rm
# =============================================================================
run_agent "$REPO_DIR/bin/agent" spawn claude --name persist --repo https://github.com/a/b.git >/dev/null 2>&1
persist_run="$(last_docker_run)"
assert_not_contains "$persist_run" "--rm " "no --rm for container persistence"
assert_not_contains "$persist_run" " --rm" "no --rm flag anywhere"

# =============================================================================
# Test: Host config injection — codex config.toml injected as B64 env var
# =============================================================================
CODEX_CONFIG_HOME="$TMP_DIR/codex-home"
mkdir -p "$CODEX_CONFIG_HOME"
cat >"$CODEX_CONFIG_HOME/config.toml" <<'TOMLEOF'
model = "o3"
TOMLEOF
CODEX_HOME="$CODEX_CONFIG_HOME" \
  run_agent_env bash "$REPO_DIR/bin/agent" spawn codex --name cfgcodex --repo https://github.com/a/b.git >/dev/null 2>&1
cfgcodex_run="$(last_docker_run)"
assert_contains "$cfgcodex_run" "SAFE_AGENTIC_CODEX_CONFIG_B64=" "codex config B64 env var present"

# =============================================================================
# Test: Host config injection — claude settings.json injected as B64 env var
# =============================================================================
CLAUDE_CONFIG_HOME="$TMP_DIR/claude-home"
mkdir -p "$CLAUDE_CONFIG_HOME/hooks"
cat >"$CLAUDE_CONFIG_HOME/settings.json" <<'JSONEOF'
{"permissions":{"defaultMode":"bypassPermissions"}}
JSONEOF
cat >"$CLAUDE_CONFIG_HOME/statusline-command.sh" <<'EOF'
#!/bin/bash
echo status
EOF
cat >"$CLAUDE_CONFIG_HOME/hooks/check-linear-ticket.sh" <<'EOF'
#!/bin/bash
echo hook
EOF
CLAUDE_CONFIG_DIR="$CLAUDE_CONFIG_HOME" \
  run_agent_env bash "$REPO_DIR/bin/agent" spawn claude --name cfgclaude --repo https://github.com/a/b.git >/dev/null 2>&1
cfgclaude_run="$(last_docker_run)"
assert_contains "$cfgclaude_run" "SAFE_AGENTIC_CLAUDE_CONFIG_B64=" "claude config B64 env var present"
assert_contains "$cfgclaude_run" "SAFE_AGENTIC_CLAUDE_SUPPORT_B64=" "claude support files B64 env var present"

# =============================================================================
# Test: Shell mode does NOT inject host config env vars
# =============================================================================
CODEX_HOME="$CODEX_CONFIG_HOME" CLAUDE_CONFIG_DIR="$CLAUDE_CONFIG_HOME" \
  run_agent_env bash "$REPO_DIR/bin/agent" shell >/dev/null 2>&1
shell_cfg_run="$(last_docker_run)"
assert_not_contains "$shell_cfg_run" "SAFE_AGENTIC_CODEX_CONFIG_B64=" "shell no codex config env var"
assert_not_contains "$shell_cfg_run" "SAFE_AGENTIC_CLAUDE_CONFIG_B64=" "shell no claude config env var"

# =============================================================================
# Test: Security preamble metadata env vars set for claude spawn
# =============================================================================
run_agent_env bash "$REPO_DIR/bin/agent" spawn claude --name preamble1 --repo https://github.com/a/b.git >/dev/null 2>&1
preamble_run="$(last_docker_run)"
assert_contains "$preamble_run" "SAFE_AGENTIC_NETWORK_MODE=" "network mode env var present"
assert_contains "$preamble_run" "SAFE_AGENTIC_DOCKER_MODE=" "docker mode env var present"
assert_contains "$preamble_run" "SAFE_AGENTIC_RESOURCES=" "resources env var present"
assert_not_contains "$preamble_run" "SAFE_AGENTIC_SSH_ENABLED=" "SSH env var absent when --ssh not used"

# =============================================================================
# Test: SSH enabled metadata env var set when --ssh flag used
# =============================================================================
run_agent_env bash "$REPO_DIR/bin/agent" spawn claude --ssh --name preamble2 --repo git@github.com:a/b.git >/dev/null 2>&1
ssh_preamble_run="$(last_docker_run)"
assert_contains "$ssh_preamble_run" "SAFE_AGENTIC_SSH_ENABLED=1" "SSH enabled env var present with --ssh"

echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
