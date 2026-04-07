#!/usr/bin/env bash
# Tests for `agent diagnose`.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

FAKE_BIN="$TMP_DIR/bin"
OUT_LOG="$TMP_DIR/out.log"
ERR_LOG="$TMP_DIR/err.log"
VERIFY_STATE="$TMP_DIR/verify-state"
HOME_DIR="$TMP_DIR/home"
mkdir -p "$FAKE_BIN" "$HOME_DIR/.config/safe-agentic"

cat >"$HOME_DIR/.config/safe-agentic/defaults.sh" <<'EOF'
SAFE_AGENTIC_DEFAULT_MEMORY=10g
EOF

cat >"$FAKE_BIN/orb" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
verify_state="${TEST_VERIFY_STATE:?}"
cmd="${1:-}"
shift || true
case "$cmd" in
  list)
    echo "safe-agentic"
    ;;
  run)
    [ "${1:-}" = "-m" ] && shift 2
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-lc" ] && [[ "${3:-}" == *safe-agentic-hardening-verify* ]]; then
      : >"$verify_state"
      exit 0
    fi
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-c" ] && [[ "${3:-}" == *SSH_AUTH_SOCK* ]]; then
      echo "/tmp/fake-ssh.sock"
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "info" ]; then
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "image" ] && [ "${3:-}" = "inspect" ]; then
      exit 0
    fi
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-c" ] && [ "${3:-}" = "test -S /var/run/docker.sock" ]; then
      exit 0
    fi
    ;;
  *)
    echo "unexpected orb command: $cmd" >&2
    exit 1
    ;;
esac
EOF
chmod +x "$FAKE_BIN/orb"

PATH="$FAKE_BIN:$PATH" TEST_VERIFY_STATE="$VERIFY_STATE" HOME="$HOME_DIR" XDG_CONFIG_HOME="$HOME_DIR/.config" \
  bash "$REPO_DIR/bin/agent" diagnose >"$OUT_LOG" 2>"$ERR_LOG"

output="$(cat "$OUT_LOG" "$ERR_LOG")"
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
    ((++fail))
  fi
}

assert_contains "$output" "orb installed" "orb check"
assert_contains "$output" "VM 'safe-agentic' exists" "vm check"
assert_contains "$output" "Docker reachable inside VM" "docker check"
assert_contains "$output" "Image 'safe-agentic:latest' present" "image check"
assert_contains "$output" "VM Docker socket visible: /var/run/docker.sock" "docker socket check"
assert_contains "$output" "Defaults file loaded:" "defaults check"
assert_contains "$output" "VM SSH agent socket visible:" "ssh check"

echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
