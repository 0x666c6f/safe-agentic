#!/usr/bin/env bash
# Tests for pipeline conditional branching: when/outputs fields parsing and dry-run display.
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
# Helper: extract the Python YAML parser from bin/agent
# =============================================================================
PYPARSER="$TMP_DIR/parse_pipeline.py"
python3 - "$AGENT" "$PYPARSER" <<'EXTRACT'
import sys, re

agent_path = sys.argv[1]
out_path   = sys.argv[2]

with open(agent_path) as f:
    content = f.read()

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
# Test 1: Pipeline with when and outputs fields parses correctly
# =============================================================================
YAML_COND="$TMP_DIR/conditional.yaml"
cat >"$YAML_COND" <<'YAML'
name: conditional-pipeline
steps:
  - name: check-tests
    type: claude
    repo: git@github.com:org/api.git
    prompt: Run tests and report pass or fail
    outputs: cat /tmp/test-result.txt
  - name: deploy
    type: claude
    repo: git@github.com:org/api.git
    prompt: Deploy to staging
    depends_on: check-tests
    when: pass
  - name: fix-tests
    type: claude
    repo: git@github.com:org/api.git
    prompt: Fix failing tests
    depends_on: check-tests
    when: fail
YAML

parsed=$(call_parse "$YAML_COND")
pname_b64=$(echo "$parsed" | head -n1 | sed 's/^pipeline_name=//')
pname=$(echo "$pname_b64" | base64 -d 2>/dev/null || echo "")
assert_eq "cond pipeline name" "$pname" "conditional-pipeline"

step_lines=$(echo "$parsed" | tail -n +2)
step_count=$(echo "$step_lines" | grep -c .)
assert_eq "cond step count" "$step_count" "3"

step1=$(echo "$step_lines" | sed -n '1p')
assert_eq "step1 name"    "$(decode_field "$step1" name)"    "check-tests"
assert_eq "step1 outputs" "$(decode_field "$step1" outputs)" "cat /tmp/test-result.txt"
assert_eq "step1 when"    "$(decode_field "$step1" when)"    ""

step2=$(echo "$step_lines" | sed -n '2p')
assert_eq "step2 name"       "$(decode_field "$step2" name)"       "deploy"
assert_eq "step2 when"       "$(decode_field "$step2" when)"       "pass"
assert_eq "step2 depends_on" "$(decode_field "$step2" depends_on)" "check-tests"
assert_eq "step2 outputs"    "$(decode_field "$step2" outputs)"    ""

step3=$(echo "$step_lines" | sed -n '3p')
assert_eq "step3 name"       "$(decode_field "$step3" name)"       "fix-tests"
assert_eq "step3 when"       "$(decode_field "$step3" when)"       "fail"
assert_eq "step3 depends_on" "$(decode_field "$step3" depends_on)" "check-tests"

# =============================================================================
# Test 2: Dry-run shows when conditions for conditional steps
# =============================================================================
out_dry=$(run_agent pipeline "$YAML_COND" --dry-run 2>&1 || true)
assert_contains "dry-run shows when pass"     "$out_dry" "when:       pass"
assert_contains "dry-run shows when fail"     "$out_dry" "when:       fail"
assert_contains "dry-run shows outputs"       "$out_dry" "outputs:    cat /tmp/test-result.txt"
assert_contains "dry-run shows pipeline name" "$out_dry" "conditional-pipeline"
assert_contains "dry-run shows step1"         "$out_dry" "check-tests"
assert_contains "dry-run shows step2"         "$out_dry" "deploy"
assert_contains "dry-run shows step3"         "$out_dry" "fix-tests"

# =============================================================================
# Test 3: Pipeline YAML with outputs field is included in parsed output
# =============================================================================
YAML_OUT="$TMP_DIR/outputs-only.yaml"
cat >"$YAML_OUT" <<'YAML'
name: outputs-test
steps:
  - name: analyze
    type: claude
    repo: https://github.com/org/repo.git
    prompt: Analyze code quality
    outputs: echo quality-score
YAML

parsed_out=$(call_parse "$YAML_OUT")
step_out=$(echo "$parsed_out" | tail -n +2 | head -n1)
assert_eq "outputs-only name"    "$(decode_field "$step_out" name)"    "analyze"
assert_eq "outputs-only outputs" "$(decode_field "$step_out" outputs)" "echo quality-score"
assert_eq "outputs-only when"    "$(decode_field "$step_out" when)"    ""

# Verify the raw parsed line actually contains an outputs= token
assert_contains "parsed line has outputs token" "$step_out" "outputs="

# =============================================================================
# Test 4: Steps without when don't show condition text in dry-run
# =============================================================================
YAML_NOCOND="$TMP_DIR/no-condition.yaml"
cat >"$YAML_NOCOND" <<'YAML'
name: no-conditions
steps:
  - name: build
    type: claude
    repo: https://github.com/org/repo.git
    prompt: Build the project
  - name: test
    type: claude
    repo: https://github.com/org/repo.git
    prompt: Run tests
    depends_on: build
YAML

out_nocond=$(run_agent pipeline "$YAML_NOCOND" --dry-run 2>&1 || true)
assert_not_contains "no-condition dry-run no when" "$out_nocond" "when:"
assert_not_contains "no-condition dry-run no outputs" "$out_nocond" "outputs:"
assert_contains "no-condition dry-run shows build" "$out_nocond" "build"
assert_contains "no-condition dry-run shows test"  "$out_nocond" "test"
assert_contains "no-condition dry-run shows depends_on" "$out_nocond" "depends_on: build"

# =============================================================================
# Test 5: Syntax check of bin/agent
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
