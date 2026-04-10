#!/usr/bin/env bash
# Tests for fleet shared_tasks feature: shared Docker volume for agent-to-agent coordination.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

AGENT_BIN="$REPO_DIR/bin/agent"
MANIFEST="$TMP_DIR/fleet.yaml"
OUT_LOG="$TMP_DIR/out.log"
ERR_LOG="$TMP_DIR/err.log"

pass=0
fail=0

assert_contains() {
  local label="$1"
  local haystack="$2"
  local needle="$3"
  if [[ "$haystack" == *"$needle"* ]]; then
    ((++pass))
  else
    echo "FAIL: $label — expected to find: $needle" >&2
    echo "      in: $haystack" >&2
    ((++fail))
  fi
}

assert_not_contains() {
  local label="$1"
  local haystack="$2"
  local needle="$3"
  if [[ "$haystack" == *"$needle"* ]]; then
    echo "FAIL: $label — expected NOT to find: $needle" >&2
    echo "      in: $haystack" >&2
    ((++fail))
  else
    ((++pass))
  fi
}

# ---------------------------------------------------------------------------
# Helper: fake orb/git binaries so dry-run works without a real VM.
# ---------------------------------------------------------------------------
FAKE_BIN="$TMP_DIR/fake-bin"
mkdir -p "$FAKE_BIN"

cat >"$FAKE_BIN/orb" <<'ORBEOF'
#!/usr/bin/env bash
cmd="${1:-}"; shift || true
case "$cmd" in
  list) echo "safe-agentic" ;;
  run)
    [ "${1:-}" = "-m" ] && shift 2
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "image" ]; then exit 0; fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "network" ]; then
      case "${3:-}" in
        inspect) exit 1 ;;
        create|rm) exit 0 ;;
      esac
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "volume" ]; then exit 0; fi
    if [ "${1:-}" = "bash" ]; then exit 0; fi
    exit 0 ;;
  push|start|stop|create|ssh) ;;
  *) exit 1 ;;
esac
ORBEOF
chmod +x "$FAKE_BIN/orb"

cat >"$FAKE_BIN/git" <<'GITEOF'
#!/usr/bin/env bash
if [ "${1:-}" = "config" ] && [ "${2:-}" = "user.name" ]; then echo "X"; exit 0; fi
if [ "${1:-}" = "config" ] && [ "${2:-}" = "user.email" ]; then echo "x@x"; exit 0; fi
exec /usr/bin/git "$@"
GITEOF
chmod +x "$FAKE_BIN/git"

# ---------------------------------------------------------------------------
# Test 1: Fleet manifest with shared_tasks: true includes --fleet-volume
# ---------------------------------------------------------------------------
cat >"$MANIFEST" <<'EOF'
shared_tasks: true
agents:
  - name: worker-a
    type: claude
    repo: https://github.com/org/repo.git
  - name: worker-b
    type: codex
    repo: https://github.com/org/repo.git
EOF

if PATH="$FAKE_BIN:$PATH" \
   bash "$AGENT_BIN" fleet "$MANIFEST" --dry-run >"$OUT_LOG" 2>"$ERR_LOG"; then
  ((++pass))
else
  echo "FAIL: fleet --dry-run with shared_tasks exited non-zero" >&2
  cat "$ERR_LOG" >&2
  ((++fail))
fi

dry_output=$(cat "$OUT_LOG" "$ERR_LOG")
assert_contains "shared_tasks: true includes --fleet-volume for worker-a" "$dry_output" "--fleet-volume"
assert_contains "shared_tasks: true mentions shared volume" "$dry_output" "shared volume"

# Both agents should have --fleet-volume
worker_a_line=$(echo "$dry_output" | grep 'worker-a' || true)
worker_b_line=$(echo "$dry_output" | grep 'worker-b' || true)
assert_contains "worker-a has --fleet-volume" "$worker_a_line" "--fleet-volume"
assert_contains "worker-b has --fleet-volume" "$worker_b_line" "--fleet-volume"

# ---------------------------------------------------------------------------
# Test 2: Fleet manifest without shared_tasks does NOT include --fleet-volume
# ---------------------------------------------------------------------------
cat >"$MANIFEST" <<'EOF'
agents:
  - name: solo-worker
    type: claude
    repo: https://github.com/org/repo.git
EOF

if PATH="$FAKE_BIN:$PATH" \
   bash "$AGENT_BIN" fleet "$MANIFEST" --dry-run >"$OUT_LOG" 2>"$ERR_LOG"; then
  ((++pass))
else
  echo "FAIL: fleet --dry-run without shared_tasks exited non-zero" >&2
  cat "$ERR_LOG" >&2
  ((++fail))
fi

dry_output=$(cat "$OUT_LOG" "$ERR_LOG")
assert_not_contains "no shared_tasks: no --fleet-volume" "$dry_output" "--fleet-volume"
assert_not_contains "no shared_tasks: no shared volume message" "$dry_output" "shared volume"

# ---------------------------------------------------------------------------
# Test 3: Fleet manifest with shared_tasks: false does NOT include --fleet-volume
# ---------------------------------------------------------------------------
cat >"$MANIFEST" <<'EOF'
shared_tasks: false
agents:
  - name: solo-worker
    type: claude
    repo: https://github.com/org/repo.git
EOF

if PATH="$FAKE_BIN:$PATH" \
   bash "$AGENT_BIN" fleet "$MANIFEST" --dry-run >"$OUT_LOG" 2>"$ERR_LOG"; then
  ((++pass))
else
  echo "FAIL: fleet --dry-run with shared_tasks: false exited non-zero" >&2
  cat "$ERR_LOG" >&2
  ((++fail))
fi

dry_output=$(cat "$OUT_LOG" "$ERR_LOG")
assert_not_contains "shared_tasks: false: no --fleet-volume" "$dry_output" "--fleet-volume"

# ---------------------------------------------------------------------------
# Test 4: The fleet volume name contains the manifest filename
# ---------------------------------------------------------------------------
NAMED_MANIFEST="$TMP_DIR/my-project.yaml"
cat >"$NAMED_MANIFEST" <<'EOF'
shared_tasks: true
agents:
  - name: task-runner
    type: claude
    repo: https://github.com/org/repo.git
EOF

if PATH="$FAKE_BIN:$PATH" \
   bash "$AGENT_BIN" fleet "$NAMED_MANIFEST" --dry-run >"$OUT_LOG" 2>"$ERR_LOG"; then
  ((++pass))
else
  echo "FAIL: fleet --dry-run with named manifest exited non-zero" >&2
  cat "$ERR_LOG" >&2
  ((++fail))
fi

dry_output=$(cat "$OUT_LOG" "$ERR_LOG")
assert_contains "volume name contains manifest base name" "$dry_output" "fleet-my-project-"

# ---------------------------------------------------------------------------
# Test 5: shared_tasks: yes (alternative truthy value) is accepted
# ---------------------------------------------------------------------------
cat >"$MANIFEST" <<'EOF'
shared_tasks: yes
agents:
  - name: yes-worker
    type: claude
    repo: https://github.com/org/repo.git
EOF

if PATH="$FAKE_BIN:$PATH" \
   bash "$AGENT_BIN" fleet "$MANIFEST" --dry-run >"$OUT_LOG" 2>"$ERR_LOG"; then
  ((++pass))
else
  echo "FAIL: fleet --dry-run with shared_tasks: yes exited non-zero" >&2
  cat "$ERR_LOG" >&2
  ((++fail))
fi

dry_output=$(cat "$OUT_LOG" "$ERR_LOG")
assert_contains "shared_tasks: yes includes --fleet-volume" "$dry_output" "--fleet-volume"

# ---------------------------------------------------------------------------
# Test 6: bash -n syntax check
# ---------------------------------------------------------------------------
if bash -n "$AGENT_BIN" 2>"$ERR_LOG"; then
  ((++pass))
else
  echo "FAIL: bash -n failed on bin/agent" >&2
  cat "$ERR_LOG" >&2
  ((++fail))
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
total=$((pass + fail))
echo "$total tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
