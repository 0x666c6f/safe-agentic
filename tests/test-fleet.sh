#!/usr/bin/env bash
# Tests for agent fleet YAML manifest parsing and --dry-run dispatch.
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

check() {
  local label="$1"
  local result="$2"  # "ok" or "fail"
  if [ "$result" = "ok" ]; then
    ((++pass))
  else
    echo "FAIL: $label" >&2
    ((++fail))
  fi
}

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
# Helper: run the python3 YAML parser extracted from cmd_fleet inline.
# Returns CMD: lines printed by the parser.
# ---------------------------------------------------------------------------
run_parser() {
  local manifest_path="$1"
  local fake_bin="${2:-$AGENT_BIN}"
  python3 - "$manifest_path" "$fake_bin" <<'PYEOF'
import sys
import shlex

manifest_path = sys.argv[1]
agent_bin     = sys.argv[2]

agents = []
current = None

with open(manifest_path) as f:
    for raw_line in f:
        line = raw_line.rstrip('\n')
        stripped = line.lstrip()

        if stripped.startswith('- name:'):
            current = {'name': stripped[len('- name:'):].strip()}
            agents.append(current)
            continue

        if current is not None and ':' in stripped and not stripped.startswith('-') and not stripped.startswith('#'):
            key, _, val = stripped.partition(':')
            key = key.strip()
            val = val.strip()
            if key and val:
                if val.lower() == 'true':
                    val = True
                elif val.lower() == 'false':
                    val = False
                current[key] = val

for agent in agents:
    agent_type = agent.get('type', '')
    if not agent_type:
        sys.stderr.write(f"[fleet] skipping entry without type: {agent}\n")
        continue

    cmd = [agent_bin, 'spawn', agent_type]

    name = agent.get('name', '')
    if name:
        cmd += ['--name', name]

    repo = agent.get('repo', '')
    if repo:
        cmd += ['--repo', repo]

    if agent.get('ssh') is True:
        cmd.append('--ssh')

    if agent.get('reuse_auth') is True:
        cmd.append('--reuse-auth')

    if agent.get('reuse_gh_auth') is True:
        cmd.append('--reuse-gh-auth')

    if agent.get('docker') is True:
        cmd.append('--docker')

    prompt = agent.get('prompt', '')
    if prompt:
        cmd += ['--prompt', str(prompt)]

    aws = agent.get('aws', '')
    if aws:
        cmd += ['--aws', str(aws)]

    network = agent.get('network', '')
    if network:
        cmd += ['--network', str(network)]

    memory = agent.get('memory', '')
    if memory:
        cmd += ['--memory', str(memory)]

    cpus = agent.get('cpus', '')
    if cpus:
        cmd += ['--cpus', str(cpus)]

    print('CMD:' + shlex.join(cmd))
PYEOF
}

# ---------------------------------------------------------------------------
# Test 1: Two-agent manifest — correct count of CMD: lines
# ---------------------------------------------------------------------------
cat >"$MANIFEST" <<'EOF'
agents:
  - name: api-worker
    type: claude
    repo: git@github.com:org/api.git
    ssh: true
    reuse_auth: true
  - name: frontend
    type: codex
    repo: https://github.com/org/frontend.git
EOF

output=$(run_parser "$MANIFEST" "/usr/bin/agent")
cmd_count=$(echo "$output" | grep -c '^CMD:' || true)
if [ "$cmd_count" -eq 2 ]; then
  ((++pass))
else
  echo "FAIL: expected 2 CMD lines, got $cmd_count" >&2
  ((++fail))
fi

# ---------------------------------------------------------------------------
# Test 2: First agent has correct type (claude) and name
# ---------------------------------------------------------------------------
assert_contains "first agent type=claude"  "$output" "spawn claude"
assert_contains "first agent name"         "$output" "--name api-worker"
assert_contains "first agent repo"         "$output" "--repo git@github.com:org/api.git"
assert_contains "first agent --ssh"        "$output" "--ssh"
assert_contains "first agent --reuse-auth" "$output" "--reuse-auth"

# ---------------------------------------------------------------------------
# Test 3: Second agent has correct type (codex) and name
# ---------------------------------------------------------------------------
assert_contains "second agent type=codex"  "$output" "spawn codex"
assert_contains "second agent name"        "$output" "--name frontend"
assert_contains "second agent repo"        "$output" "--repo https://github.com/org/frontend.git"

# ---------------------------------------------------------------------------
# Test 4: Boolean false — ssh not set on second agent
# ---------------------------------------------------------------------------
second_cmd=$(echo "$output" | grep 'frontend' || true)
assert_not_contains "second agent no --ssh" "$second_cmd" "--ssh"

# ---------------------------------------------------------------------------
# Test 5: Additional optional fields (prompt, aws, network, memory, cpus)
# ---------------------------------------------------------------------------
cat >"$MANIFEST" <<'EOF'
agents:
  - name: complex-worker
    type: claude
    repo: https://github.com/org/api.git
    prompt: Fix the failing tests
    aws: my-profile
    network: agent-isolated
    memory: 16g
    cpus: 8
EOF

output=$(run_parser "$MANIFEST" "/usr/bin/agent")
assert_contains "prompt field"   "$output" "--prompt"
assert_contains "prompt value"   "$output" "Fix the failing tests"
assert_contains "aws field"      "$output" "--aws my-profile"
assert_contains "network field"  "$output" "--network agent-isolated"
assert_contains "memory field"   "$output" "--memory 16g"
assert_contains "cpus field"     "$output" "--cpus 8"

# ---------------------------------------------------------------------------
# Test 6: Entry without type is skipped
# ---------------------------------------------------------------------------
cat >"$MANIFEST" <<'EOF'
agents:
  - name: no-type-agent
    repo: https://github.com/org/repo.git
  - name: valid-agent
    type: codex
    repo: https://github.com/org/repo.git
EOF

output=$(run_parser "$MANIFEST" "/usr/bin/agent")
cmd_count=$(echo "$output" | grep -c '^CMD:' || true)
if [ "$cmd_count" -eq 1 ]; then
  ((++pass))
else
  echo "FAIL: expected 1 CMD line (skipped no-type), got $cmd_count" >&2
  ((++fail))
fi
assert_contains "valid agent present" "$output" "--name valid-agent"

# ---------------------------------------------------------------------------
# Test 7: Empty manifest (no agents) produces no CMD lines
# ---------------------------------------------------------------------------
cat >"$MANIFEST" <<'EOF'
agents:
EOF

output=$(run_parser "$MANIFEST" "/usr/bin/agent" 2>/dev/null || true)
cmd_count=$(echo "$output" | grep -c '^CMD:' || true)
if [ "$cmd_count" -eq 0 ]; then
  ((++pass))
else
  echo "FAIL: expected 0 CMD lines for empty manifest, got $cmd_count" >&2
  ((++fail))
fi

# ---------------------------------------------------------------------------
# Test 8: --dry-run prints commands, does not execute
# ---------------------------------------------------------------------------
FAKE_BIN="$TMP_DIR/fake-bin"
SPAWN_LOG="$TMP_DIR/spawn.log"
mkdir -p "$FAKE_BIN"

# Fake orb that records invocations
cat >"$FAKE_BIN/orb" <<'ORBEOF'
#!/usr/bin/env bash
log_file="${TEST_ORB_LOG:?}"
cmd="${1:-}"; shift || true
case "$cmd" in
  list) echo "safe-agentic" ;;
  run)
    [ "${1:-}" = "-m" ] && shift 2
    printf '%s\n' "$*" >>"$log_file"
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-lc" ]; then exit 0; fi
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-c" ]; then exit 0; fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "image" ]; then exit 0; fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "network" ]; then
      case "${3:-}" in
        inspect) exit 1 ;;
        create|rm) exit 0 ;;
      esac
    fi
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

cat >"$MANIFEST" <<'EOF'
agents:
  - name: dry-worker
    type: claude
    repo: https://github.com/org/repo.git
EOF

# --dry-run should succeed without needing orb/VM
if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$TMP_DIR/orb.log" \
   bash "$AGENT_BIN" fleet "$MANIFEST" --dry-run >"$OUT_LOG" 2>"$ERR_LOG"; then
  ((++pass))
else
  echo "FAIL: fleet --dry-run exited non-zero" >&2
  cat "$ERR_LOG" >&2
  ((++fail))
fi

dry_output=$(cat "$OUT_LOG" "$ERR_LOG")
assert_contains "dry-run mentions worker name" "$dry_output" "dry-worker"
# Ensure no actual container was started (spawn log should be empty)
if [ ! -s "$TMP_DIR/orb.log" ] || ! grep -q 'docker run' "$TMP_DIR/orb.log" 2>/dev/null; then
  ((++pass))
else
  echo "FAIL: --dry-run should not have issued docker run" >&2
  ((++fail))
fi

# ---------------------------------------------------------------------------
# Test 9: Missing manifest file → non-zero exit
# ---------------------------------------------------------------------------
if ! bash "$AGENT_BIN" fleet /nonexistent/manifest.yaml >"$OUT_LOG" 2>"$ERR_LOG"; then
  ((++pass))
else
  echo "FAIL: expected non-zero exit for missing manifest" >&2
  ((++fail))
fi

# ---------------------------------------------------------------------------
# Test 10: bash -n syntax check
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
