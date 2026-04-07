#!/usr/bin/env bash
# Tests for agent cp file and directory export via VM staging.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"
ERR_LOG="$TMP_DIR/error.log"
OUT_LOG="$TMP_DIR/out.log"
mkdir -p "$FAKE_BIN"

cat >"$FAKE_BIN/orb" <<'ORBEOF'
#!/usr/bin/env bash
set -euo pipefail

log_file="${TEST_ORB_LOG:?}"
cmd="${1:-}"
shift || true

case "$cmd" in
  list)
    echo "safe-agentic"
    ;;
  run)
    [ "${1:-}" = "-m" ] && shift 2
    printf '%s\n' "$*" >>"$log_file"

    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "ps" ] && [ "${3:-}" = "--latest" ]; then
      echo "agent-codex-latest"
      exit 0
    fi

    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "ps" ] && [ "${3:-}" = "--format" ]; then
      printf 'agent-claude-export\nagent-codex-latest\n'
      exit 0
    fi

    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "cp" ]; then
      src="${3:-}"
      dest="${4:-}"
      mkdir -p "$dest"
      case "$src" in
        agent-claude-export:/workspace/report.txt)
          printf 'report from agent\n' >"$dest/report.txt"
          ;;
        agent-claude-export:/workspace/report-dir)
          mkdir -p "$dest/report-dir"
          printf 'dir artifact\n' >"$dest/report-dir/build.txt"
          ;;
        agent-claude-export:/workspace/.)
          printf 'root payload\n' >"$dest/root.txt"
          mkdir -p "$dest/assets"
          printf 'nested payload\n' >"$dest/assets/app.txt"
          ;;
        agent-codex-latest:/workspace/dist)
          mkdir -p "$dest/dist"
          printf 'artifact\n' >"$dest/dist/build.txt"
          ;;
        *)
          exit 1
          ;;
      esac
      exit 0
    fi

    case "${1:-}" in
      mktemp|tar|rm)
        exec "$@"
        ;;
    esac

    exit 0
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

run_ok() {
  local label="$1"
  shift
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
    ((++pass))
  else
    echo "FAIL: $label: expected zero exit" >&2
    cat "$ERR_LOG" >&2 || true
    ((++fail))
  fi
}

run_fails() {
  local label="$1"
  shift
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
    echo "FAIL: $label: expected non-zero exit" >&2
    ((++fail))
  else
    ((++pass))
  fi
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

assert_file_contains() {
  local path="$1" needle="$2" label="${3:-}"
  if [ -f "$path" ] && grep -q -- "$needle" "$path"; then
    ((++pass))
  else
    echo "FAIL${label:+: $label}: '$needle' not found in $path" >&2
    ((++fail))
  fi
}

# --- file copy to explicit host path ---
: >"$ORB_LOG"
mkdir -p "$TMP_DIR/exports"
run_ok "copy file by name" \
  bash "$REPO_DIR/bin/agent" cp export /workspace/report.txt "$TMP_DIR/exports/report-copy.txt"
assert_file_contains "$TMP_DIR/exports/report-copy.txt" "report from agent" "copied file content"
file_copy_log="$(cat "$ORB_LOG")"
assert_contains "$file_copy_log" "docker cp agent-claude-export:/workspace/report.txt" "docker cp file source"
assert_contains "$file_copy_log" "tar -C /tmp/safe-agentic-cp." "file tar stream"

# --- directory copy with --latest into existing directory ---
: >"$ORB_LOG"
mkdir -p "$TMP_DIR/collect"
run_ok "copy dir by latest" \
  bash "$REPO_DIR/bin/agent" cp --latest /workspace/dist "$TMP_DIR/collect"
assert_file_contains "$TMP_DIR/collect/dist/build.txt" "artifact" "copied dir content"
dir_copy_log="$(cat "$ORB_LOG")"
assert_contains "$dir_copy_log" "docker ps --latest --filter name=^agent- --format {{.Names}}" "latest container lookup"
assert_contains "$dir_copy_log" "docker cp agent-codex-latest:/workspace/dist" "docker cp dir source"

# --- file copy into existing directory preserves basename ---
: >"$ORB_LOG"
mkdir -p "$TMP_DIR/drop"
run_ok "copy file into existing dir" \
  bash "$REPO_DIR/bin/agent" cp export /workspace/report.txt "$TMP_DIR/drop"
assert_file_contains "$TMP_DIR/drop/report.txt" "report from agent" "existing dir gets basename"

# --- directory copy to new path renames directory ---
: >"$ORB_LOG"
run_ok "copy dir to renamed path" \
  bash "$REPO_DIR/bin/agent" cp export /workspace/report-dir "$TMP_DIR/renamed-dir"
assert_file_contains "$TMP_DIR/renamed-dir/build.txt" "dir artifact" "renamed dir payload"

# --- copying contents path expands multiple top-level entries into destination dir ---
: >"$ORB_LOG"
mkdir -p "$TMP_DIR/content-collect"
run_ok "copy contents into dir" \
  bash "$REPO_DIR/bin/agent" cp export /workspace/. "$TMP_DIR/content-collect"
assert_file_contains "$TMP_DIR/content-collect/root.txt" "root payload" "root content copied"
assert_file_contains "$TMP_DIR/content-collect/assets/app.txt" "nested payload" "nested content copied"

# --- directory source cannot overwrite existing file path ---
: >"$ORB_LOG"
printf 'existing file\n' >"$TMP_DIR/existing-file"
run_fails "copy dir onto file path" \
  bash "$REPO_DIR/bin/agent" cp export /workspace/report-dir "$TMP_DIR/existing-file"
assert_contains "$(cat "$ERR_LOG")" "Destination exists and is not a directory" "dir onto file error"

# --- multi-entry copy requires directory destination ---
: >"$ORB_LOG"
printf 'existing file\n' >"$TMP_DIR/content-file"
run_fails "copy contents onto file path" \
  bash "$REPO_DIR/bin/agent" cp export /workspace/. "$TMP_DIR/content-file"
assert_contains "$(cat "$ERR_LOG")" "Destination must be a directory" "multi-entry destination error"

# --- missing source reports clear failure ---
: >"$ORB_LOG"
run_fails "copy missing path" \
  bash "$REPO_DIR/bin/agent" cp export /workspace/missing "$TMP_DIR/exports/missing"
assert_contains "$(cat "$ERR_LOG")" "not found or unreadable" "missing path error"

echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
