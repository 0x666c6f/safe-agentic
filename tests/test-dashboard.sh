#!/usr/bin/env bash
# Tests for the `agent dashboard` command.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TUI_BIN="$REPO_DIR/tui/agent-tui"
PASS=0
FAIL=0

pass() { PASS=$((PASS + 1)); printf '  \033[32mPASS\033[0m %s\n' "$1"; }
fail() { FAIL=$((FAIL + 1)); printf '  \033[31mFAIL\033[0m %s\n' "$1"; }

echo "=== test-dashboard ==="

# Skip if TUI binary not built
if [ ! -x "$TUI_BIN" ]; then
  echo "SKIP: TUI binary not built ($TUI_BIN). Run 'make -C tui build' first."
  exit 77
fi

# -----------------------------------------------------------------------
# Test 1: --help output mentions dashboard
# -----------------------------------------------------------------------
help_output=$("$TUI_BIN" --help 2>&1) || true
if echo "$help_output" | grep -qi "dashboard"; then
  pass "TUI --help mentions dashboard"
else
  fail "TUI --help does not mention dashboard"
fi

# -----------------------------------------------------------------------
# Test 2: agent help output mentions dashboard
# -----------------------------------------------------------------------
agent_help=$("$REPO_DIR/bin/agent" help 2>&1) || true
if echo "$agent_help" | grep -qi "dashboard"; then
  pass "agent help mentions dashboard"
else
  fail "agent help does not mention dashboard"
fi

# -----------------------------------------------------------------------
# Test 3: --help output mentions --bind flag
# -----------------------------------------------------------------------
if echo "$help_output" | grep -qi "\-\-bind"; then
  pass "TUI --help mentions --bind"
else
  fail "TUI --help does not mention --bind"
fi

echo
echo "Results: $PASS passed, $FAIL failed"
[ "$FAIL" -eq 0 ] || exit 1
