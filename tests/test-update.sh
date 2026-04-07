#!/usr/bin/env bash
# Tests for `agent update`: tracked-only build context and build-mode flags.
set -euo pipefail

SRC_REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

TEST_REPO="$TMP_DIR/repo"
FAKE_BIN="$TMP_DIR/bin"
VM_DIR="$TMP_DIR/vm"
VM_TARBALL="$TMP_DIR/vm-ctx.tar.gz"
ORB_LOG="$TMP_DIR/orb.log"
VERIFY_STATE="$TMP_DIR/verify-state"
mkdir -p "$TEST_REPO/bin" "$TEST_REPO/config" "$TEST_REPO/vm" "$FAKE_BIN" "$VM_DIR"

cp "$SRC_REPO_DIR/bin/agent" "$TEST_REPO/bin/agent"
cp "$SRC_REPO_DIR/bin/agent-lib.sh" "$TEST_REPO/bin/agent-lib.sh"
cp "$SRC_REPO_DIR/config/seccomp.json" "$TEST_REPO/config/seccomp.json"
cp "$SRC_REPO_DIR/vm/setup.sh" "$TEST_REPO/vm/setup.sh"

cat >"$FAKE_BIN/orb" <<'ORBEOF'
#!/usr/bin/env bash
set -euo pipefail

log_file="${TEST_ORB_LOG:?}"
vm_tarball="${TEST_VM_TARBALL:?}"
vm_dir="${TEST_VM_DIR:?}"
verify_state="${TEST_VERIFY_STATE:?}"
cmd="${1:-}"
shift || true

case "$cmd" in
  list)
    echo "safe-agentic"
    ;;
  run)
    [ "${1:-}" = "-m" ] && shift 2
    printf '%s\n' "$*" >>"$log_file"

    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-lc" ] && [[ "${3:-}" == *safe-agentic-hardening-verify* ]]; then
      : >"$verify_state"
      exit 0
    fi

    if [ "${1:-}" = "env" ] && [[ "${2:-}" == DEST=* ]] && [ "${3:-}" = "bash" ] && [ "${4:-}" = "-c" ]; then
      case "${2#DEST=}" in
        /tmp/safe-agentic-ctx.tar.gz)
          cat >"$vm_tarball"
          ;;
        *)
          cat > /dev/null
          ;;
      esac
      exit 0
    fi

    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-c" ] && [[ "${3:-}" == *"tar -xzf /tmp/safe-agentic-ctx.tar.gz -C /tmp/safe-agentic"* ]]; then
      rm -rf "$vm_dir/safe-agentic"
      mkdir -p "$vm_dir/safe-agentic"
      tar -xzf "$vm_tarball" -C "$vm_dir/safe-agentic"
      exit 0
    fi

    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-c" ] && [[ "${3:-}" == install\ -m\ 0644\ -D\ /tmp/seccomp.json* ]]; then
      exit 0
    fi

    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "build" ]; then
      exit 0
    fi

    echo "unexpected orb run payload: $*" >&2
    exit 1
    ;;
  *)
    echo "unexpected orb command: $cmd" >&2
    exit 1
    ;;
esac
ORBEOF
chmod +x "$FAKE_BIN/orb"

pass=0
fail=0

assert_file_exists() {
  local path="$1" label="$2"
  if [ -e "$path" ]; then
    ((++pass))
  else
    echo "FAIL: $label — missing $path" >&2
    ((++fail))
  fi
}

assert_file_absent() {
  local path="$1" label="$2"
  if [ ! -e "$path" ]; then
    ((++pass))
  else
    echo "FAIL: $label — unexpected $path" >&2
    ((++fail))
  fi
}

assert_contains() {
  local haystack="$1" needle="$2" label="$3"
  if [[ "$haystack" == *"$needle"* ]]; then
    ((++pass))
  else
    echo "FAIL: $label — missing '$needle'" >&2
    echo "  in: $haystack" >&2
    ((++fail))
  fi
}

assert_not_contains() {
  local haystack="$1" needle="$2" label="$3"
  if [[ "$haystack" != *"$needle"* ]]; then
    ((++pass))
  else
    echo "FAIL: $label — unexpected '$needle'" >&2
    echo "  in: $haystack" >&2
    ((++fail))
  fi
}

run_update() {
  : >"$ORB_LOG"
  local status=0
  (
    cd "$TEST_REPO"
    PATH="$FAKE_BIN:$PATH" \
    TEST_ORB_LOG="$ORB_LOG" \
    TEST_VM_TARBALL="$VM_TARBALL" \
    TEST_VM_DIR="$VM_DIR" \
    TEST_VERIFY_STATE="$VERIFY_STATE" \
    bash "$TEST_REPO/bin/agent" update "$@"
  ) >/dev/null 2>&1 || status=$?
  return "$status"
}

last_build_cmd() {
  grep 'docker build ' "$ORB_LOG" | tail -n 1
}

(
  cd "$TEST_REPO"
  git init -q
  git config user.name "Test User"
  git config user.email "test@example.com"
  git config commit.gpgsign false
  mkdir -p nested
  printf 'space\n' > 'space name.txt'
  printf 'dash\n' > '--leading-dash.txt'
  printf 'tracked\n' > tracked.txt
  printf 'nested\n' > nested/keep.txt
  printf 'gone\n' > gone.txt
  printf 'ignore me\n' > scratch.txt
  git add -- bin/agent bin/agent-lib.sh config/seccomp.json vm/setup.sh tracked.txt nested/keep.txt gone.txt 'space name.txt' '--leading-dash.txt'
  git commit -qm "init"
  rm -f gone.txt
)

# --- default update ships tracked files only, skipping deleted tracked files ---
run_update
assert_file_exists "$VM_DIR/safe-agentic/bin/agent" "tracked bin/agent copied"
assert_file_exists "$VM_DIR/safe-agentic/bin/agent-lib.sh" "tracked agent-lib copied"
assert_file_exists "$VM_DIR/safe-agentic/tracked.txt" "tracked file copied"
assert_file_exists "$VM_DIR/safe-agentic/nested/keep.txt" "nested tracked file copied"
assert_file_exists "$VM_DIR/safe-agentic/space name.txt" "tracked file with space copied"
assert_file_exists "$VM_DIR/safe-agentic/--leading-dash.txt" "tracked file with leading dash copied"
assert_file_absent "$VM_DIR/safe-agentic/scratch.txt" "untracked file excluded"
assert_file_absent "$VM_DIR/safe-agentic/gone.txt" "deleted tracked file excluded"
default_build="$(last_build_cmd)"
assert_not_contains "$default_build" "--no-cache" "default build no no-cache"
assert_not_contains "$default_build" "CLI_CACHE_BUST=" "default build no quick cache bust"

# --- quick update sets CLI cache bust build arg ---
run_update --quick
quick_build="$(last_build_cmd)"
assert_contains "$quick_build" "--build-arg CLI_CACHE_BUST=" "quick build cache bust arg"

# --- full update disables cache ---
run_update --full
full_build="$(last_build_cmd)"
assert_contains "$full_build" "docker build --no-cache -t safe-agentic:latest /tmp/safe-agentic/" "full build no-cache"

# --- quick + full is rejected ---
if run_update --quick --full; then
  echo "FAIL: update should reject --quick with --full" >&2
  ((++fail))
else
  ((++pass))
fi

echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
