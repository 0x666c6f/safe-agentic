#!/usr/bin/env bash
# Test lifecycle script parsing from safe-agentic.json
set -euo pipefail

pass=0
fail=0

assert_eq() {
  local label="$1" expected="$2" actual="$3"
  if [ "$expected" = "$actual" ]; then
    ((++pass))
  else
    echo "FAIL: $label" >&2
    echo "  expected: $(printf '%q' "$expected")" >&2
    echo "  actual:   $(printf '%q' "$actual")" >&2
    ((++fail))
  fi
}

assert_empty() {
  local label="$1" actual="$2"
  assert_eq "$label" "" "$actual"
}

# Create temp workspace
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

WORKSPACE="$TMP_DIR/workspace"
mkdir -p "$WORKSPACE/myrepo"

# Helper: parse script from safe-agentic.json using the same python3 snippet
# as entrypoint.sh
parse_script() {
  local config_file="$1" script_name="$2"
  python3 -c "
import json, sys
try:
    with open('$config_file') as f:
        data = json.load(f)
    print(data.get('scripts', {}).get('$script_name', ''))
except:
    pass
" 2>/dev/null || true
}

# ---------------------------------------------------------------------------
# Test 1: setup script extracted correctly
# ---------------------------------------------------------------------------
cat >"$WORKSPACE/myrepo/safe-agentic.json" <<'EOF'
{
  "scripts": {
    "setup": "npm install && npm run build"
  }
}
EOF

result=$(parse_script "$WORKSPACE/myrepo/safe-agentic.json" "setup")
assert_eq "setup script extracted" "npm install && npm run build" "$result"

# ---------------------------------------------------------------------------
# Test 2: missing script key returns empty
# ---------------------------------------------------------------------------
cat >"$WORKSPACE/myrepo/safe-agentic.json" <<'EOF'
{
  "scripts": {
    "build": "make build"
  }
}
EOF

result=$(parse_script "$WORKSPACE/myrepo/safe-agentic.json" "setup")
assert_empty "missing setup key returns empty" "$result"

# ---------------------------------------------------------------------------
# Test 3: JSON without scripts key returns empty
# ---------------------------------------------------------------------------
cat >"$WORKSPACE/myrepo/safe-agentic.json" <<'EOF'
{
  "name": "my-project",
  "version": "1.0.0"
}
EOF

result=$(parse_script "$WORKSPACE/myrepo/safe-agentic.json" "setup")
assert_empty "no scripts key returns empty" "$result"

# ---------------------------------------------------------------------------
# Test 4: malformed JSON does not crash (returns empty)
# ---------------------------------------------------------------------------
printf 'not valid json {{{{' >"$WORKSPACE/myrepo/safe-agentic.json"

result=$(parse_script "$WORKSPACE/myrepo/safe-agentic.json" "setup")
assert_empty "malformed JSON returns empty" "$result"

# ---------------------------------------------------------------------------
# Test 5: missing file returns empty
# ---------------------------------------------------------------------------
result=$(parse_script "$WORKSPACE/myrepo/does-not-exist.json" "setup")
assert_empty "missing file returns empty" "$result"

# ---------------------------------------------------------------------------
# Test 6: entrypoint.sh contains run_lifecycle_script function
# ---------------------------------------------------------------------------
REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENTRYPOINT="$REPO_DIR/entrypoint.sh"

if grep -qF 'run_lifecycle_script' "$ENTRYPOINT"; then
  ((++pass))
else
  echo "FAIL: run_lifecycle_script function missing from entrypoint.sh" >&2
  ((++fail))
fi

# ---------------------------------------------------------------------------
# Test 7: entrypoint.sh calls run_lifecycle_script "setup"
# ---------------------------------------------------------------------------
if grep -qF 'run_lifecycle_script "setup"' "$ENTRYPOINT"; then
  ((++pass))
else
  echo "FAIL: run_lifecycle_script \"setup\" call missing from entrypoint.sh" >&2
  ((++fail))
fi

# ---------------------------------------------------------------------------
# Test 8: setup script with multiple commands runs to completion
# ---------------------------------------------------------------------------
cat >"$WORKSPACE/myrepo/safe-agentic.json" <<'EOF'
{
  "scripts": {
    "setup": "echo hello && echo world"
  }
}
EOF

result=$(parse_script "$WORKSPACE/myrepo/safe-agentic.json" "setup")
assert_eq "multi-command setup script extracted" "echo hello && echo world" "$result"

# ---------------------------------------------------------------------------
# Test 9: empty scripts object returns empty for setup
# ---------------------------------------------------------------------------
cat >"$WORKSPACE/myrepo/safe-agentic.json" <<'EOF'
{
  "scripts": {}
}
EOF

result=$(parse_script "$WORKSPACE/myrepo/safe-agentic.json" "setup")
assert_empty "empty scripts object returns empty" "$result"

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
