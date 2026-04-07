#!/usr/bin/env bash
# Optional live e2e test: real Claude and Codex CLIs inside real VM containers.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VM_NAME="safe-agentic"

skip() {
  echo "SKIP: $*"
  exit 77
}

pass=0
fail=0

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

assert_ok() {
  local label="$1"
  shift

  if "$@" >/dev/null 2>&1; then
    ((++pass))
  else
    echo "FAIL: $label" >&2
    ((++fail))
  fi
}

[ "${SAFE_AGENTIC_SKIP_LIVE:-}" = "1" ] && skip "SAFE_AGENTIC_SKIP_LIVE=1"
command -v orb >/dev/null 2>&1 || skip "orb not installed"
orb list 2>/dev/null | awk '{print $1}' | grep -qx "$VM_NAME" || skip "VM '$VM_NAME' not found"
orb run -m "$VM_NAME" docker info >/dev/null 2>&1 || skip "docker unavailable in VM"

IMAGE_NAME=""
for candidate in safe-agentic:validation safe-agentic:latest; do
  if orb run -m "$VM_NAME" docker image inspect "$candidate" >/dev/null 2>&1; then
    IMAGE_NAME="$candidate"
    break
  fi
done
[ -n "$IMAGE_NAME" ] || skip "no built safe-agentic image found"

readonly_flags=(
  --read-only
  --tmpfs /tmp:rw,noexec,nosuid,size=64m
  --tmpfs /var/tmp:rw,noexec,nosuid,size=32m
  --tmpfs /run:rw,noexec,nosuid,size=8m
  --tmpfs /home/agent/.config:rw,noexec,nosuid,uid=1000,gid=1000,size=8m
  --tmpfs /home/agent/.ssh:rw,noexec,nosuid,uid=1000,gid=1000,size=1m
)

claude_version="$(
  orb run -m "$VM_NAME" docker run --rm \
    "${readonly_flags[@]}" \
    -e AGENT_TYPE=claude \
    "$IMAGE_NAME" --version
)"
assert_contains "$claude_version" "[entrypoint] Launching Claude Code..." "claude entrypoint launches"
assert_contains "$claude_version" "Claude Code" "claude version output"

claude_help="$(
  orb run -m "$VM_NAME" docker run --rm \
    "${readonly_flags[@]}" \
    -e AGENT_TYPE=claude \
    "$IMAGE_NAME" --help
)"
assert_contains "$claude_help" "Usage: claude" "claude help usage"
assert_contains "$claude_help" "--print" "claude help includes non-interactive flag"

codex_version="$(
  orb run -m "$VM_NAME" bash -lc '
    tmpdir=$(mktemp -d)
    mkdir -p "$tmpdir/.codex"
    printf "{}\n" > "$tmpdir/.codex/auth.json"
    docker run --rm \
      --read-only \
      --tmpfs /tmp:rw,noexec,nosuid,size=64m \
      --tmpfs /var/tmp:rw,noexec,nosuid,size=32m \
      --tmpfs /run:rw,noexec,nosuid,size=8m \
      --tmpfs /home/agent/.config:rw,noexec,nosuid,uid=1000,gid=1000,size=8m \
      --tmpfs /home/agent/.ssh:rw,noexec,nosuid,uid=1000,gid=1000,size=1m \
      --mount type=bind,src="$tmpdir/.codex",dst=/home/agent/.codex,readonly \
      -e AGENT_TYPE=codex \
      '"$IMAGE_NAME"' --version
    status=$?
    rm -rf "$tmpdir"
    exit $status
  '
)"
assert_contains "$codex_version" "[entrypoint] Launching Codex..." "codex entrypoint launches"
assert_contains "$codex_version" "codex-cli" "codex version output"

codex_exec_help="$(
  orb run -m "$VM_NAME" bash -lc '
    tmpdir=$(mktemp -d)
    mkdir -p "$tmpdir/.codex"
    printf "{}\n" > "$tmpdir/.codex/auth.json"
    docker run --rm \
      --read-only \
      --tmpfs /tmp:rw,noexec,nosuid,size=64m \
      --tmpfs /var/tmp:rw,noexec,nosuid,size=32m \
      --tmpfs /run:rw,noexec,nosuid,size=8m \
      --tmpfs /home/agent/.config:rw,noexec,nosuid,uid=1000,gid=1000,size=8m \
      --tmpfs /home/agent/.ssh:rw,noexec,nosuid,uid=1000,gid=1000,size=1m \
      --mount type=bind,src="$tmpdir/.codex",dst=/home/agent/.codex,readonly \
      -e AGENT_TYPE=codex \
      '"$IMAGE_NAME"' exec --help
    status=$?
    rm -rf "$tmpdir"
    exit $status
  '
)"
assert_contains "$codex_exec_help" "Run Codex non-interactively" "codex exec help"
assert_contains "$codex_exec_help" "Usage: codex exec" "codex exec usage"

assert_ok "real image still exposes raw cli versions" \
  orb run -m "$VM_NAME" docker run --rm --entrypoint bash "$IMAGE_NAME" -lc 'claude --version >/dev/null && codex --version >/dev/null'

echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
