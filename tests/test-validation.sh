#!/usr/bin/env bash
# Unit tests for validation functions in agent-lib.sh.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Provide stubs expected by agent-lib.sh (normally defined in bin/agent).
# die() must exit (not return) to match production behavior.
# Tests that expect failure run the function in a subshell so exit doesn't kill the test.
IMAGE_NAME="safe-agentic"
die() { exit 1; }
info() { :; }
warn() { :; }
vm_exec() { return 1; }
export -f die info warn vm_exec
export IMAGE_NAME

# Source the library
source "$REPO_DIR/bin/agent-lib.sh"
export -f validate_name_component validate_pids_limit validate_network_name bridge_name_for_network network_name_for_container

pass=0
fail=0

# --- validate_name_component ---

assert_name_ok() {
  local input="$1"
  if (validate_name_component "$input" "test") 2>/dev/null; then
    ((++pass))
  else
    echo "FAIL: validate_name_component '$input' should pass" >&2
    ((++fail))
  fi
}

assert_name_fail() {
  local input="$1"
  local label="${2:-}"
  if (validate_name_component "$input" "test") 2>/dev/null; then
    echo "FAIL: validate_name_component '$input' should fail${label:+ ($label)}" >&2
    ((++fail))
  else
    ((++pass))
  fi
}

# Valid names
assert_name_ok "review"
assert_name_ok "my-task"
assert_name_ok "task_01"
assert_name_ok "Task.v2"
assert_name_ok "A"
assert_name_ok "a1b2c3"
assert_name_ok "0-start-with-digit"

# Invalid names
assert_name_fail ""               "empty"
assert_name_fail "-leading-dash"  "leading dash"
assert_name_fail ".leading-dot"   "leading dot"
assert_name_fail "_leading-under" "leading underscore"
assert_name_fail "has space"      "space"
assert_name_fail "semi;colon"     "semicolon"
assert_name_fail 'dollar$sign'    "dollar sign"
assert_name_fail "back\slash"     "backslash"
assert_name_fail "pipe|char"      "pipe"
assert_name_fail "amp&ersand"     "ampersand"

# --- validate_network_name ---

assert_net_ok() {
  local input="$1"
  if (validate_network_name "$input") 2>/dev/null; then
    ((++pass))
  else
    echo "FAIL: validate_network_name '$input' should pass" >&2
    ((++fail))
  fi
}

assert_net_fail() {
  local input="$1"
  local label="${2:-}"
  if (validate_network_name "$input") 2>/dev/null; then
    echo "FAIL: validate_network_name '$input' should fail${label:+ ($label)}" >&2
    ((++fail))
  else
    ((++pass))
  fi
}

# Valid network names
assert_net_ok "my-network"
assert_net_ok "agent-isolated"
assert_net_ok "none"
assert_net_ok "Net.v2"
assert_net_ok "net_01"

# Blocked network modes (security-critical)
assert_net_fail "host"           "host network"
assert_net_fail "bridge"         "default bridge"
assert_net_fail "container:foo"  "container mode"
assert_net_fail "container:abc"  "container mode variant"

# Invalid characters
assert_net_fail ""               "empty"
assert_net_fail "has space"      "space"
assert_net_fail "semi;colon"     "semicolon"
assert_net_fail "-leading-dash"  "leading dash"

# --- validate_pids_limit ---

assert_pids_ok() {
  local input="$1"
  if (validate_pids_limit "$input") 2>/dev/null; then
    ((++pass))
  else
    echo "FAIL: validate_pids_limit '$input' should pass" >&2
    ((++fail))
  fi
}

assert_pids_fail() {
  local input="$1"
  local label="${2:-}"
  if (validate_pids_limit "$input") 2>/dev/null; then
    echo "FAIL: validate_pids_limit '$input' should fail${label:+ ($label)}" >&2
    ((++fail))
  else
    ((++pass))
  fi
}

assert_pids_ok "64"
assert_pids_ok "512"
assert_pids_fail "63" "below floor"
assert_pids_fail "abc" "non-numeric"
assert_pids_fail "64.5" "non-integer"

# --- network_name_for_container ---

result=$(network_name_for_container "agent-claude-review")
if [ "$result" = "agent-claude-review-net" ]; then
  ((++pass))
else
  echo "FAIL: network_name_for_container 'agent-claude-review' → '$result'" >&2
  ((++fail))
fi

# --- bridge_name_for_network ---

bridge_name="$(bridge_name_for_network "agent-claude-review-net")"
if [[ "$bridge_name" == sa* ]] && [ "${#bridge_name}" -le 14 ]; then
  ((++pass))
else
  echo "FAIL: bridge_name_for_network produced '$bridge_name'" >&2
  ((++fail))
fi

echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
