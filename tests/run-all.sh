#!/usr/bin/env bash
# Run all tests.
set -euo pipefail

TESTS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PASS=0
FAIL=0
SKIP=0

run_test() {
  local test_file="$1"
  local name
  local output
  local status

  name="$(basename "$test_file")"
  printf "%-50s " "$name"

  set +e
  output="$(bash "$test_file" 2>&1)"
  status=$?
  set -e

  case "$status" in
    0)
      echo "PASS"
      PASS=$((PASS + 1))
      ;;
    77)
      echo "SKIP"
      SKIP=$((SKIP + 1))
      if [ -n "$output" ]; then
        echo "--- $name output ---"
        echo "$output"
        echo "--- end ---"
      fi
      ;;
    *)
      echo "FAIL"
      FAIL=$((FAIL + 1))
      echo "--- $name output ---"
      echo "$output"
      echo "--- end ---"
      ;;
  esac
}

for test_file in "$TESTS_DIR"/test-*.sh; do
  run_test "$test_file"
done

# Also run the existing security regression test
run_test "$TESTS_DIR/agent-cli-security.sh"

echo ""
echo "$((PASS + FAIL + SKIP)) tests, $PASS passed, $FAIL failed, $SKIP skipped"
[ "$FAIL" -eq 0 ]
