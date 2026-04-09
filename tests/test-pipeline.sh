#!/usr/bin/env bash
# Tests for agent pipeline: YAML parsing, dry-run plan, depends_on, retry, on_failure.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

pass=0
fail=0

assert_eq() {
  local label="$1" got="$2" want="$3"
  if [ "$got" = "$want" ]; then
    ((++pass))
  else
    echo "FAIL: $label: got='$got' want='$want'" >&2
    ((++fail))
  fi
}

assert_contains() {
  local label="$1" haystack="$2" needle="$3"
  if [[ "$haystack" == *"$needle"* ]]; then
    ((++pass))
  else
    echo "FAIL: $label: '$needle' not found in output" >&2
    ((++fail))
  fi
}

assert_not_contains() {
  local label="$1" haystack="$2" needle="$3"
  if [[ "$haystack" != *"$needle"* ]]; then
    ((++pass))
  else
    echo "FAIL: $label: '$needle' unexpectedly found in output" >&2
    ((++fail))
  fi
}

AGENT="$REPO_DIR/bin/agent"

# =============================================================================
# Helper: run parse_pipeline_yaml by invoking the embedded Python directly.
# We extract the heredoc from bin/agent and run it.
# =============================================================================

# Extract the Python parser from bin/agent (the parse_pipeline_yaml PYEOF block)
PYPARSER="$TMP_DIR/parse_pipeline.py"
python3 - "$AGENT" "$PYPARSER" <<'EXTRACT'
import sys, re

agent_path = sys.argv[1]
out_path   = sys.argv[2]

with open(agent_path) as f:
    content = f.read()

# Find the parse_pipeline_yaml function's PYEOF block specifically
# Look for the function marker then extract the heredoc within it
m = re.search(r"parse_pipeline_yaml\(\).*?<<'PYEOF'\n(.*?)\nPYEOF", content, re.DOTALL)
if not m:
    print("ERROR: could not find parse_pipeline_yaml PYEOF block in bin/agent", file=sys.stderr)
    sys.exit(1)

with open(out_path, 'w') as f:
    f.write(m.group(1) + '\n')

print("OK")
EXTRACT

call_parse() {
  local yaml_file="$1"
  python3 "$PYPARSER" "$yaml_file"
}

decode_field() {
  local line="$1" field="$2"
  # Extract everything after "field=" up to next space (or end of line)
  # Then base64-decode. Use sed to strip only the leading "field=" prefix.
  local raw
  raw=$(echo "$line" | grep -o "${field}=[^ ]*" | sed "s/^${field}=//" || echo "")
  [ -z "$raw" ] && echo "" && return 0
  echo "$raw" | base64 -d 2>/dev/null || echo ""
}

# =============================================================================
# Fake orb/git for CLI dispatch tests
# =============================================================================
FAKE_BIN="$TMP_DIR/fakebin"
mkdir -p "$FAKE_BIN"

cat >"$FAKE_BIN/orb" <<'EOF'
#!/usr/bin/env bash
case "${1:-}" in
  list) echo "safe-agentic" ;;
  *) exit 0 ;;
esac
EOF
chmod +x "$FAKE_BIN/orb"

cat >"$FAKE_BIN/git" <<'EOF'
#!/usr/bin/env bash
if [ "${1:-}" = "config" ] && [ "${2:-}" = "user.name" ]; then echo "X"; exit 0; fi
if [ "${1:-}" = "config" ] && [ "${2:-}" = "user.email" ]; then echo "x@x"; exit 0; fi
exec /usr/bin/git "$@"
EOF
chmod +x "$FAKE_BIN/git"

run_agent() {
  PATH="$FAKE_BIN:$PATH" bash "$AGENT" "$@" 2>&1
}

# =============================================================================
# Test 1: Basic 3-step pipeline
# =============================================================================
YAML1="$TMP_DIR/basic.yaml"
cat >"$YAML1" <<'YAML'
name: test-and-fix
steps:
  - name: run-tests
    type: claude
    repo: git@github.com:org/api.git
    prompt: Run all tests
    on_failure: fix-tests
  - name: fix-tests
    type: claude
    repo: git@github.com:org/api.git
    prompt: Fix failing tests
    retry: 2
  - name: create-pr
    type: claude
    repo: git@github.com:org/api.git
    prompt: Create a PR
    depends_on: fix-tests
YAML

parsed1=$(call_parse "$YAML1")
pname_b64=$(echo "$parsed1" | head -n1 | sed 's/^pipeline_name=//')
pname=$(echo "$pname_b64" | base64 -d 2>/dev/null || echo "")
assert_eq "pipeline name" "$pname" "test-and-fix"

step_lines1=$(echo "$parsed1" | tail -n +2)
step_count1=$(echo "$step_lines1" | grep -c .)
assert_eq "step count" "$step_count1" "3"

step1=$(echo "$step_lines1" | sed -n '1p')
assert_eq "step1 name"       "$(decode_field "$step1" name)"       "run-tests"
assert_eq "step1 type"       "$(decode_field "$step1" type)"       "claude"
assert_eq "step1 repo"       "$(decode_field "$step1" repo)"       "git@github.com:org/api.git"
assert_eq "step1 prompt"     "$(decode_field "$step1" prompt)"     "Run all tests"
assert_eq "step1 on_failure" "$(decode_field "$step1" on_failure)" "fix-tests"
assert_eq "step1 retry"      "$(decode_field "$step1" retry)"      ""

step2=$(echo "$step_lines1" | sed -n '2p')
assert_eq "step2 name"   "$(decode_field "$step2" name)"   "fix-tests"
assert_eq "step2 retry"  "$(decode_field "$step2" retry)"  "2"

step3=$(echo "$step_lines1" | sed -n '3p')
assert_eq "step3 name"       "$(decode_field "$step3" name)"       "create-pr"
assert_eq "step3 depends_on" "$(decode_field "$step3" depends_on)" "fix-tests"

# =============================================================================
# Test 2: Pipeline with ssh and reuse_auth
# =============================================================================
YAML2="$TMP_DIR/ssh.yaml"
cat >"$YAML2" <<'YAML'
name: ssh-pipeline
steps:
  - name: clone-and-test
    type: codex
    repo: git@github.com:org/secure.git
    ssh: true
    reuse_auth: true
    prompt: Run security scan
YAML

parsed2=$(call_parse "$YAML2")
step_ssh=$(echo "$parsed2" | tail -n +2 | head -n1)
assert_eq "ssh step name"       "$(decode_field "$step_ssh" name)"       "clone-and-test"
assert_eq "ssh step type"       "$(decode_field "$step_ssh" type)"       "codex"
assert_eq "ssh step ssh"        "$(decode_field "$step_ssh" ssh)"        "true"
assert_eq "ssh step reuse_auth" "$(decode_field "$step_ssh" reuse_auth)" "true"
assert_eq "ssh step prompt"     "$(decode_field "$step_ssh" prompt)"     "Run security scan"

# =============================================================================
# Test 3: Minimal pipeline (no optional fields)
# =============================================================================
YAML3="$TMP_DIR/minimal.yaml"
cat >"$YAML3" <<'YAML'
name: minimal
steps:
  - name: only-step
    type: claude
YAML

parsed3=$(call_parse "$YAML3")
step_min=$(echo "$parsed3" | tail -n +2 | head -n1)
assert_eq "minimal step name"       "$(decode_field "$step_min" name)"       "only-step"
assert_eq "minimal step depends_on" "$(decode_field "$step_min" depends_on)" ""
assert_eq "minimal step on_failure" "$(decode_field "$step_min" on_failure)" ""
assert_eq "minimal step retry"      "$(decode_field "$step_min" retry)"      ""

# =============================================================================
# Test 4: Multi-word prompt preserved through base64
# =============================================================================
YAML4="$TMP_DIR/multiword.yaml"
cat >"$YAML4" <<'YAML'
name: multi-word
steps:
  - name: analyze
    type: claude
    prompt: Analyze the codebase and find security issues
YAML

parsed4=$(call_parse "$YAML4")
step4=$(echo "$parsed4" | tail -n +2 | head -n1)
prompt4=$(decode_field "$step4" prompt)
assert_eq "multi-word prompt" "$prompt4" "Analyze the codebase and find security issues"

# =============================================================================
# Test 5: Dry-run output (CLI)
# =============================================================================
out_dry=$(run_agent pipeline "$YAML1" --dry-run 2>&1 || true)
assert_contains "dry-run shows step1"         "$out_dry" "run-tests"
assert_contains "dry-run shows step2"         "$out_dry" "fix-tests"
assert_contains "dry-run shows step3"         "$out_dry" "create-pr"
assert_contains "dry-run shows pipeline name" "$out_dry" "test-and-fix"
assert_contains "dry-run shows on_failure"    "$out_dry" "on_failure"
assert_contains "dry-run shows depends_on"    "$out_dry" "depends_on"
assert_contains "dry-run shows retry"         "$out_dry" "retry"
assert_contains "dry-run shows agent spawn"   "$out_dry" "agent spawn"

# Dry run must NOT actually launch Docker
assert_not_contains "dry-run no attach"  "$out_dry" "Attaching"
assert_not_contains "dry-run no docker"  "$out_dry" "docker run"

# =============================================================================
# Test 6: Dry-run with ssh/reuse_auth flags reflected in plan
# =============================================================================
out_dry2=$(run_agent pipeline "$YAML2" --dry-run 2>&1 || true)
assert_contains "dry-run ssh flag shown"        "$out_dry2" "--ssh"
assert_contains "dry-run reuse-auth flag shown" "$out_dry2" "--reuse-auth"

# =============================================================================
# Test 7: CLI error handling — missing file arg
# =============================================================================
out_nofile=$(run_agent pipeline 2>&1 || true)
assert_contains "missing file arg error" "$out_nofile" "Pipeline YAML file required"

# =============================================================================
# Test 8: CLI error handling — file not found
# =============================================================================
out_notfound=$(run_agent pipeline /nonexistent/file.yaml 2>&1 || true)
assert_contains "file not found error" "$out_notfound" "not found"

# =============================================================================
# Test 9: Pipeline --help
# =============================================================================
out_help=$(run_agent pipeline --help 2>&1 || true)
assert_contains "pipeline help usage"    "$out_help" "Usage: agent pipeline"
assert_contains "pipeline help dry-run"  "$out_help" "--dry-run"
assert_contains "pipeline help depends"  "$out_help" "depends_on"
assert_contains "pipeline help retry"    "$out_help" "retry"
assert_contains "pipeline help on_fail"  "$out_help" "on_failure"

# =============================================================================
# Test 10: cmd_help pipeline topic
# =============================================================================
out_help2=$(run_agent help pipeline 2>&1 || true)
assert_contains "help pipeline topic" "$out_help2" "Usage: agent pipeline"

# =============================================================================
# Test 11: General help mentions pipeline
# =============================================================================
out_general=$(run_agent help 2>&1 || true)
assert_contains "general help mentions pipeline" "$out_general" "pipeline"

# =============================================================================
# Test 12: Syntax check of bin/agent
# =============================================================================
if bash -n "$AGENT" 2>/dev/null; then
  ((++pass))
else
  echo "FAIL: bin/agent syntax check failed" >&2
  ((++fail))
fi

echo ""
echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
