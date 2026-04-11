# safe-agentic v2 Improvements — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement 6 phases of improvements — UX simplification, async lifecycle events, state management, fleet communication, observability dashboard, and SDK foundations — as defined in `docs/design-v2-improvements.md`.

**Architecture:** All changes target the existing bash CLI (`bin/agent`) and shared library (`bin/agent-lib.sh`). New Go code goes into the existing `tui/` module (expanded to serve both TUI and web dashboard). Tests follow the existing fake-orb pattern in `tests/`.

**Tech Stack:** Bash (CLI), Go (TUI + dashboard), Python (embedded cost/YAML parsers), HTML + SSE (dashboard frontend)

---

## Task 1: Add `cmd_run()` — One-Command Quick Start

**Files:**
- Modify: `bin/agent:4349-4391` (dispatch case statement)
- Modify: `bin/agent:751` (insert new function before `cmd_spawn`)
- Modify: `bin/agent-lib.sh` (add `detect_ssh_url` helper)
- Create: `tests/test-run-command.sh`

- [ ] **Step 1: Write the test file**

Create `tests/test-run-command.sh`:

```bash
#!/usr/bin/env bash
# Tests for the 'agent run' command.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

AGENT_BIN="$REPO_DIR/bin/agent"
OUT_LOG="$TMP_DIR/out.log"
ERR_LOG="$TMP_DIR/err.log"
FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"

mkdir -p "$FAKE_BIN"

cat >"$FAKE_BIN/orb" <<'FAKEORB'
#!/usr/bin/env bash
set -euo pipefail
log_file="${TEST_ORB_LOG:?}"
cmd="${1:-}"; shift || true
case "$cmd" in
  list) echo "safe-agentic" ;;
  run)
    [ "${1:-}" = "-m" ] && shift 2
    printf '%s\n' "$*" >>"$log_file"
    # docker inspect — always return "no such container" for fresh spawns
    if [[ "${1:-}" = "docker" && "${2:-}" = "inspect" ]]; then
      echo "no-container"; exit 1
    fi
    # docker network ls — return empty
    if [[ "${1:-}" = "docker" && "${2:-}" = "network" ]]; then
      exit 0
    fi
    # docker volume ls — simulate auth volume exists
    if [[ "${1:-}" = "docker" && "${2:-}" = "volume" && "${3:-}" = "ls" ]]; then
      echo "agent-claude-auth"
      exit 0
    fi
    exit 0 ;;
  *) exit 0 ;;
esac
FAKEORB
chmod +x "$FAKE_BIN/orb"

pass=0
fail=0

run_dry() {
  local label="$1"; shift
  : >"$ORB_LOG"
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" \
     "$AGENT_BIN" "$@" --dry-run >"$OUT_LOG" 2>"$ERR_LOG"; then
    ((++pass))
  else
    echo "FAIL: $label: expected zero exit" >&2
    cat "$ERR_LOG" >&2
    ((++fail))
  fi
}

assert_log_contains() {
  local needle="$1" label="$2"
  if grep -qF "$needle" "$ORB_LOG" 2>/dev/null || grep -qF "$needle" "$OUT_LOG" 2>/dev/null; then
    ((++pass))
  else
    echo "FAIL: $label — expected to find: $needle" >&2
    ((++fail))
  fi
}

assert_err_contains() {
  local needle="$1" label="$2"
  if grep -qF "$needle" "$ERR_LOG" 2>/dev/null; then
    ((++pass))
  else
    echo "FAIL: $label — expected in stderr: $needle" >&2
    ((++fail))
  fi
}

run_fails() {
  local label="$1"; shift
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" \
     "$AGENT_BIN" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
    echo "FAIL: $label: expected non-zero exit" >&2
    ((++fail))
  else
    ((++pass))
  fi
}

# --- Tests ---

# 1. Basic run with SSH URL auto-detects SSH and sets prompt
run_dry "run-ssh-url" run git@github.com:org/repo.git "Fix the tests"
assert_err_contains "spawn" "run delegates to spawn"

# 2. Run with https URL does not enable SSH
run_dry "run-https-url" run https://github.com/org/repo.git "Fix the tests"

# 3. Run with --codex flag sets agent type
run_dry "run-codex" run --codex git@github.com:org/repo.git "Fix tests"

# 4. Run with no repo fails
run_fails "run-no-repo" run "Fix the tests"

# 5. Run with no prompt fails
run_fails "run-no-prompt" run git@github.com:org/repo.git

# 6. Run with multiple repos
run_dry "run-multi-repo" run git@github.com:org/a.git git@github.com:org/b.git "Fix shared types"

echo ""
echo "test-run-command: $pass passed, $fail failed"
[ "$fail" -eq 0 ] || exit 1
```

- [ ] **Step 2: Run the test to confirm it fails**

Run: `bash tests/test-run-command.sh`
Expected: FAIL — `Unknown command: run`

- [ ] **Step 3: Implement `cmd_run()` in `bin/agent`**

Insert before `cmd_spawn()` (around line 751):

```bash
cmd_run() {
  local agent_type="claude"
  local -a repos=()
  local prompt=""
  local -a extra_flags=()

  # Parse flags
  while [ $# -gt 0 ]; do
    case "$1" in
      --claude)  agent_type="claude"; shift ;;
      --codex)   agent_type="codex"; shift ;;
      --dry-run) extra_flags+=(--dry-run); shift ;;
      --ephemeral-auth) extra_flags+=(--no-reuse-auth); shift ;;
      --max-cost) extra_flags+=(--max-cost "$2"); shift 2 ;;
      --on-complete) extra_flags+=(--on-complete "$2"); shift 2 ;;
      --on-fail) extra_flags+=(--on-fail "$2"); shift 2 ;;
      --notify) extra_flags+=(--notify "$2"); shift 2 ;;
      --network) extra_flags+=(--network "$2"); shift 2 ;;
      --memory) extra_flags+=(--memory "$2"); shift 2 ;;
      --cpus) extra_flags+=(--cpus "$2"); shift 2 ;;
      --name) extra_flags+=(--name "$2"); shift 2 ;;
      --template) extra_flags+=(--template "$2"); shift 2 ;;
      --instructions) extra_flags+=(--instructions "$2"); shift 2 ;;
      --background) extra_flags+=(--background); shift ;;
      -*)
        die "Unknown flag for 'run': $1. Use 'agent spawn' for full control." ;;
      *)
        # Positional args: URLs then prompt (last arg)
        # Collect all remaining positional args
        local -a positional=()
        while [ $# -gt 0 ] && [[ "$1" != -* ]]; do
          positional+=("$1"); shift
        done
        # Last positional is the prompt, rest are repos
        if [ ${#positional[@]} -lt 2 ]; then
          die "Usage: agent run [flags] <repo-url> [repo-url...] \"prompt\""
        fi
        prompt="${positional[-1]}"
        unset 'positional[-1]'
        repos=("${positional[@]}")
        ;;
    esac
  done

  [ ${#repos[@]} -gt 0 ] || die "Usage: agent run [flags] <repo-url> [repo-url...] \"prompt\""
  [ -n "$prompt" ] || die "Usage: agent run [flags] <repo-url> [repo-url...] \"prompt\""

  # Auto-detect SSH
  local enable_ssh=false
  for url in "${repos[@]}"; do
    if [[ "$url" == git@* ]] || [[ "$url" == ssh://* ]]; then
      enable_ssh=true
      break
    fi
  done

  # Auto-detect git identity from host
  local identity_flag=()
  if [ -z "${SAFE_AGENTIC_DEFAULT_IDENTITY:-}" ]; then
    local git_name git_email
    git_name=$(git config --global user.name 2>/dev/null || echo "")
    git_email=$(git config --global user.email 2>/dev/null || echo "")
    if [ -n "$git_name" ] && [ -n "$git_email" ]; then
      identity_flag=(--identity "$git_name <$git_email>")
    fi
  fi

  # Build spawn args
  local -a spawn_args=("$agent_type")
  for url in "${repos[@]}"; do
    spawn_args+=(--repo "$url")
  done
  spawn_args+=(--prompt "$prompt")
  [ "$enable_ssh" = true ] && spawn_args+=(--ssh)
  spawn_args+=(--reuse-auth)
  [ ${#identity_flag[@]} -gt 0 ] && spawn_args+=("${identity_flag[@]}")
  spawn_args+=("${extra_flags[@]}")

  cmd_spawn "${spawn_args[@]}"
}
```

- [ ] **Step 4: Add `run` to the dispatch case statement**

In `bin/agent` at line ~4352, add `run)` to the case statement:

```bash
  run)        cmd_run "$@" ;;
```

Add it right after the `spawn)` line.

- [ ] **Step 5: Add `run` to `cmd_help()`**

Find the help text section in `cmd_help()` and add:

```
  run         Quick start — auto-detect settings from repo URL
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `bash tests/test-run-command.sh`
Expected: All tests PASS

- [ ] **Step 7: Run full test suite to check for regressions**

Run: `bash tests/run-all.sh`
Expected: All existing tests still pass

- [ ] **Step 8: Commit**

```bash
git add bin/agent tests/test-run-command.sh
git commit -m "feat: add 'agent run' one-command quick start

Smart defaults: auto-detect SSH from git@ URLs, auto-enable
reuse-auth, auto-detect git identity from host config.
Prompt is the last positional argument."
```

---

## Task 2: Auth Default Flip — Make `--reuse-auth` Default

**Files:**
- Modify: `bin/agent:57` (DEFAULT_REUSE_AUTH)
- Modify: `bin/agent:751-900` (cmd_spawn flag parsing — add `--ephemeral-auth` / `--no-reuse-auth`)
- Modify: `tests/test-docker-cmd.sh` (update expectations)
- Create: `tests/test-auth-defaults.sh`

- [ ] **Step 1: Write the test**

Create `tests/test-auth-defaults.sh`:

```bash
#!/usr/bin/env bash
# Tests for auth default behavior.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

AGENT_BIN="$REPO_DIR/bin/agent"
OUT_LOG="$TMP_DIR/out.log"
ERR_LOG="$TMP_DIR/err.log"
FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"

mkdir -p "$FAKE_BIN"
cat >"$FAKE_BIN/orb" <<'FAKEORB'
#!/usr/bin/env bash
set -euo pipefail
log_file="${TEST_ORB_LOG:?}"
cmd="${1:-}"; shift || true
case "$cmd" in
  list) echo "safe-agentic" ;;
  run)
    [ "${1:-}" = "-m" ] && shift 2
    printf '%s\n' "$*" >>"$log_file"
    if [[ "${1:-}" = "docker" && "${2:-}" = "inspect" ]]; then exit 1; fi
    if [[ "${1:-}" = "docker" && "${2:-}" = "network" ]]; then exit 0; fi
    exit 0 ;;
  *) exit 0 ;;
esac
FAKEORB
chmod +x "$FAKE_BIN/orb"

pass=0
fail=0

assert_log_contains() {
  local needle="$1" label="$2"
  if grep -qF "$needle" "$ORB_LOG" 2>/dev/null || grep -qF "$needle" "$OUT_LOG" 2>/dev/null; then
    ((++pass))
  else
    echo "FAIL: $label — expected: $needle" >&2
    ((++fail))
  fi
}

assert_log_not_contains() {
  local needle="$1" label="$2"
  if grep -qF "$needle" "$ORB_LOG" 2>/dev/null; then
    echo "FAIL: $label — should NOT contain: $needle" >&2
    ((++fail))
  else
    ((++pass))
  fi
}

# 1. Default spawn uses shared auth volume
: >"$ORB_LOG"
PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" \
  "$AGENT_BIN" spawn claude --repo https://github.com/org/r.git --dry-run >"$OUT_LOG" 2>"$ERR_LOG" || true
assert_log_contains "agent-claude-auth" "default spawn uses shared auth volume"

# 2. --ephemeral-auth disables shared auth
: >"$ORB_LOG"
PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" \
  "$AGENT_BIN" spawn claude --repo https://github.com/org/r.git --ephemeral-auth --dry-run >"$OUT_LOG" 2>"$ERR_LOG" || true
assert_log_not_contains "agent-claude-auth" "--ephemeral-auth disables shared auth"

# 3. --no-reuse-auth is alias for --ephemeral-auth
: >"$ORB_LOG"
PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" \
  "$AGENT_BIN" spawn claude --repo https://github.com/org/r.git --no-reuse-auth --dry-run >"$OUT_LOG" 2>"$ERR_LOG" || true
assert_log_not_contains "agent-claude-auth" "--no-reuse-auth disables shared auth"

echo ""
echo "test-auth-defaults: $pass passed, $fail failed"
[ "$fail" -eq 0 ] || exit 1
```

- [ ] **Step 2: Run the test to confirm it fails**

Run: `bash tests/test-auth-defaults.sh`
Expected: FAIL — default spawn does NOT use shared auth volume yet

- [ ] **Step 3: Change the default in `bin/agent`**

At line 57, change:

```bash
DEFAULT_REUSE_AUTH="${SAFE_AGENTIC_DEFAULT_REUSE_AUTH:-false}"
```

to:

```bash
DEFAULT_REUSE_AUTH="${SAFE_AGENTIC_DEFAULT_REUSE_AUTH:-true}"
```

- [ ] **Step 4: Add `--ephemeral-auth` and `--no-reuse-auth` flags to `cmd_spawn()`**

In the flag parsing loop inside `cmd_spawn()` (around line 779-899), add:

```bash
      --ephemeral-auth|--no-reuse-auth)
        reuse_auth=false
        shift
        ;;
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `bash tests/test-auth-defaults.sh`
Expected: All PASS

- [ ] **Step 6: Run full test suite, fix any regressions**

Run: `bash tests/run-all.sh`
Expected: May need to update `tests/test-docker-cmd.sh` if it asserts the old default. Fix any tests that now fail because auth volume appears by default.

- [ ] **Step 7: Commit**

```bash
git add bin/agent tests/test-auth-defaults.sh tests/test-docker-cmd.sh
git commit -m "feat: make --reuse-auth the default, add --ephemeral-auth opt-out

Auth volumes are now reused by default for convenience.
Use --ephemeral-auth for isolated one-off sessions."
```

---

## Task 3: Git Identity Auto-Detection

**Files:**
- Modify: `bin/agent:751-900` (cmd_spawn, before container creation)
- Modify: `bin/agent-lib.sh` (add `detect_git_identity` helper)
- Create: `tests/test-git-identity.sh`

- [ ] **Step 1: Write the test**

Create `tests/test-git-identity.sh`:

```bash
#!/usr/bin/env bash
# Tests for git identity auto-detection.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

AGENT_BIN="$REPO_DIR/bin/agent"
OUT_LOG="$TMP_DIR/out.log"
ERR_LOG="$TMP_DIR/err.log"
FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"

mkdir -p "$FAKE_BIN"
cat >"$FAKE_BIN/orb" <<'FAKEORB'
#!/usr/bin/env bash
set -euo pipefail
log_file="${TEST_ORB_LOG:?}"
cmd="${1:-}"; shift || true
case "$cmd" in
  list) echo "safe-agentic" ;;
  run)
    [ "${1:-}" = "-m" ] && shift 2
    printf '%s\n' "$*" >>"$log_file"
    if [[ "${1:-}" = "docker" && "${2:-}" = "inspect" ]]; then exit 1; fi
    if [[ "${1:-}" = "docker" && "${2:-}" = "network" ]]; then exit 0; fi
    exit 0 ;;
  *) exit 0 ;;
esac
FAKEORB
chmod +x "$FAKE_BIN/orb"

# Create a fake git config
FAKE_HOME="$TMP_DIR/home"
mkdir -p "$FAKE_HOME"
cat >"$FAKE_HOME/.gitconfig" <<'GIT'
[user]
    name = Test User
    email = test@example.com
GIT

pass=0
fail=0

assert_log_contains() {
  local needle="$1" label="$2"
  if grep -qF "$needle" "$ORB_LOG" 2>/dev/null || grep -qF "$needle" "$OUT_LOG" 2>/dev/null; then
    ((++pass))
  else
    echo "FAIL: $label — expected: $needle" >&2
    ((++fail))
  fi
}

# 1. Auto-detects git identity when no --identity flag
: >"$ORB_LOG"
HOME="$FAKE_HOME" PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" \
  "$AGENT_BIN" spawn claude --repo https://github.com/org/r.git --dry-run >"$OUT_LOG" 2>"$ERR_LOG" || true
assert_log_contains "GIT_AUTHOR_NAME=Test User" "auto-detect git name"
assert_log_contains "GIT_AUTHOR_EMAIL=test@example.com" "auto-detect git email"

# 2. Explicit --identity overrides auto-detection
: >"$ORB_LOG"
HOME="$FAKE_HOME" PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" \
  "$AGENT_BIN" spawn claude --repo https://github.com/org/r.git --identity "Other <other@ex.com>" --dry-run >"$OUT_LOG" 2>"$ERR_LOG" || true
assert_log_contains "GIT_AUTHOR_NAME=Other" "explicit identity name"

echo ""
echo "test-git-identity: $pass passed, $fail failed"
[ "$fail" -eq 0 ] || exit 1
```

- [ ] **Step 2: Run the test to confirm it fails**

Run: `bash tests/test-git-identity.sh`
Expected: FAIL — currently defaults to `Agent <agent@localhost>`

- [ ] **Step 3: Add `detect_git_identity()` to `bin/agent-lib.sh`**

```bash
# Auto-detect git identity from host's global config.
# Sets GIT_AUTHOR_NAME / GIT_AUTHOR_EMAIL if not already set.
detect_git_identity() {
  local git_name git_email
  git_name=$(git config --global user.name 2>/dev/null || echo "")
  git_email=$(git config --global user.email 2>/dev/null || echo "")
  if [ -n "$git_name" ] && [ -n "$git_email" ]; then
    echo "$git_name <$git_email>"
  else
    echo ""
  fi
}
```

- [ ] **Step 4: Use it in `cmd_spawn()` before identity is consumed**

In `cmd_spawn()`, after flag parsing but before `build_container_runtime()` is called (around line 901), add:

```bash
  # Auto-detect git identity if not explicitly set
  if [ -z "$GIT_AUTHOR_NAME" ] && [ -z "${identity:-}" ]; then
    local detected_identity
    detected_identity=$(detect_git_identity)
    if [ -n "$detected_identity" ]; then
      # Parse "Name <email>" format
      GIT_AUTHOR_NAME="${detected_identity%%<*}"
      GIT_AUTHOR_NAME="${GIT_AUTHOR_NAME% }"
      GIT_AUTHOR_EMAIL="${detected_identity#*<}"
      GIT_AUTHOR_EMAIL="${GIT_AUTHOR_EMAIL%>}"
      GIT_COMMITTER_NAME="$GIT_AUTHOR_NAME"
      GIT_COMMITTER_EMAIL="$GIT_AUTHOR_EMAIL"
    fi
  fi
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `bash tests/test-git-identity.sh`
Expected: All PASS

- [ ] **Step 6: Run full test suite**

Run: `bash tests/run-all.sh`
Expected: All pass

- [ ] **Step 7: Commit**

```bash
git add bin/agent bin/agent-lib.sh tests/test-git-identity.sh
git commit -m "feat: auto-detect git identity from host config

Falls back to Agent <agent@localhost> only when host has no
git user.name/user.email configured."
```

---

## Task 4: Event System Foundation

**Files:**
- Modify: `bin/agent-lib.sh` (add `emit_event()`, `start_event_watcher()`, `dispatch_event()`)
- Modify: `bin/agent:751-1084` (cmd_spawn — start watcher, emit spawned event)
- Modify: `bin/agent` (cmd_stop — emit completed/failed events)
- Create: `tests/test-events.sh`

- [ ] **Step 1: Write the test**

Create `tests/test-events.sh`:

```bash
#!/usr/bin/env bash
# Tests for the event system.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

# Source the library directly for unit testing
source "$REPO_DIR/bin/agent-lib.sh" 2>/dev/null || true

pass=0
fail=0

assert_eq() {
  local label="$1" got="$2" want="$3"
  if [ "$got" = "$want" ]; then
    ((++pass))
  else
    echo "FAIL: $label — got: '$got', want: '$want'" >&2
    ((++fail))
  fi
}

# Test event JSONL format
EVENT_FILE="$TMP_DIR/events.jsonl"

# 1. emit_event writes valid JSONL
emit_event "$EVENT_FILE" "agent.spawned" '{"name":"test","type":"claude"}'
line=$(tail -1 "$EVENT_FILE")
assert_eq "event has type field" "$(echo "$line" | python3 -c 'import sys,json; print(json.load(sys.stdin)["event"])')" "agent.spawned"
assert_eq "event has timestamp" "$(echo "$line" | python3 -c 'import sys,json; d=json.load(sys.stdin); print("ts" in d)')" "True"
assert_eq "event has data" "$(echo "$line" | python3 -c 'import sys,json; print(json.load(sys.stdin)["data"]["name"])')" "test"

# 2. Multiple events append
emit_event "$EVENT_FILE" "agent.completed" '{"name":"test","exit_code":0}'
assert_eq "two events in file" "$(wc -l < "$EVENT_FILE" | tr -d ' ')" "2"

# 3. dispatch_event runs command sink
SINK_OUT="$TMP_DIR/sink.out"
dispatch_event "command" "echo dispatched" "agent.completed" '{"name":"test"}' > "$SINK_OUT" 2>&1 || true
assert_eq "command sink executed" "$(cat "$SINK_OUT")" "dispatched"

echo ""
echo "test-events: $pass passed, $fail failed"
[ "$fail" -eq 0 ] || exit 1
```

- [ ] **Step 2: Run the test to confirm it fails**

Run: `bash tests/test-events.sh`
Expected: FAIL — `emit_event` function not found

- [ ] **Step 3: Implement event functions in `bin/agent-lib.sh`**

Add at the end of `bin/agent-lib.sh`:

```bash
# --- Event System ---

# Emit an event as a JSONL line to a file.
# Usage: emit_event <file> <event_type> <json_data>
emit_event() {
  local file="$1" event_type="$2" data="$3"
  local ts
  ts=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
  printf '{"ts":"%s","event":"%s","data":%s}\n' "$ts" "$event_type" "$data" >> "$file"
}

# Dispatch an event to a sink.
# Usage: dispatch_event <sink_type> <sink_target> <event_type> <json_data>
dispatch_event() {
  local sink_type="$1" sink_target="$2" event_type="$3" data="$4"
  case "$sink_type" in
    command)
      eval "$sink_target"
      ;;
    webhook)
      curl -s -X POST -H "Content-Type: application/json" -d "$data" "$sink_target" || true
      ;;
    file)
      emit_event "$sink_target" "$event_type" "$data"
      ;;
  esac
}

# Parse event sinks from config file.
# Returns lines of: sink_type|sink_target|event_pattern
# Usage: parse_event_sinks <config_dir>
parse_event_sinks() {
  local config_dir="${1:-${XDG_CONFIG_HOME:-$HOME/.config}/safe-agentic}"
  local sinks_file="$config_dir/events.yaml"
  [ -f "$sinks_file" ] || return 0

  python3 -c "
import sys, re
content = open('$sinks_file').read()
# Simple YAML parser for sink entries
current_type = current_events = current_target = ''
for line in content.split('\n'):
    line = line.strip()
    if line.startswith('type:'):
        current_type = line.split(':',1)[1].strip()
    elif line.startswith('events:'):
        current_events = line.split(':',1)[1].strip().strip('[]').replace(' ','')
    elif line.startswith('command:') or line.startswith('url:') or line.startswith('path:'):
        current_target = line.split(':',1)[1].strip().strip('\"').strip(\"'\")
    elif line.startswith('- type:'):
        if current_type and current_target:
            print(f'{current_type}|{current_target}|{current_events}')
        current_type = line.split(':',1)[1].strip()
        current_events = current_target = ''
if current_type and current_target:
    print(f'{current_type}|{current_target}|{current_events}')
" 2>/dev/null || true
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `bash tests/test-events.sh`
Expected: All PASS

- [ ] **Step 5: Wire events into `cmd_spawn()`**

In `cmd_spawn()`, after successful container start (around line 1066-1084), add:

```bash
  # Emit agent.spawned event
  local events_dir="${XDG_CONFIG_HOME:-$HOME/.config}/safe-agentic"
  local events_file="$events_dir/events.jsonl"
  mkdir -p "$events_dir"
  emit_event "$events_file" "agent.spawned" \
    "$(printf '{"name":"%s","type":"%s","repo":"%s"}' "$container_name" "$agent_type" "${repos[0]:-}")"
```

- [ ] **Step 6: Add `--on-complete` and `--on-fail` flags to `cmd_spawn()`**

In the flag parsing loop, add:

```bash
      --on-complete)
        on_complete_cmd="$2"
        shift 2
        ;;
      --on-fail)
        on_fail_cmd="$2"
        shift 2
        ;;
```

Store as labels alongside `--on-exit`:

```bash
  [ -n "$on_complete_cmd" ] && {
    local on_complete_b64
    on_complete_b64=$(printf '%s' "$on_complete_cmd" | base64)
    docker_cmd+=(-e "SAFE_AGENTIC_ON_COMPLETE_B64=$on_complete_b64")
    docker_cmd+=(--label "safe-agentic.on-complete-b64=$on_complete_b64")
  }
  [ -n "$on_fail_cmd" ] && {
    local on_fail_b64
    on_fail_b64=$(printf '%s' "$on_fail_cmd" | base64)
    docker_cmd+=(-e "SAFE_AGENTIC_ON_FAIL_B64=$on_fail_b64")
    docker_cmd+=(--label "safe-agentic.on-fail-b64=$on_fail_b64")
  }
```

- [ ] **Step 7: Run full test suite**

Run: `bash tests/run-all.sh`
Expected: All pass

- [ ] **Step 8: Commit**

```bash
git add bin/agent bin/agent-lib.sh tests/test-events.sh
git commit -m "feat: add event system with emit, dispatch, and sink support

Foundation for --on-complete, --on-fail, webhooks, and
budget enforcement. Events written as JSONL."
```

---

## Task 5: Budget Enforcement (`--max-cost` Kill Switch)

**Files:**
- Modify: `bin/agent-lib.sh` (add `start_budget_monitor()`, `compute_running_cost()`)
- Modify: `bin/agent:751-1084` (cmd_spawn — start monitor when --max-cost set)
- Create: `tests/test-budget-enforcement.sh`

- [ ] **Step 1: Write the test**

Create `tests/test-budget-enforcement.sh`:

```bash
#!/usr/bin/env bash
# Tests for budget enforcement.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

source "$REPO_DIR/bin/agent-lib.sh" 2>/dev/null || true

pass=0
fail=0

assert_eq() {
  local label="$1" got="$2" want="$3"
  if [ "$got" = "$want" ]; then ((++pass)); else echo "FAIL: $label — got '$got' want '$want'" >&2; ((++fail)); fi
}

# Test cost computation from JSONL
JSONL_FILE="$TMP_DIR/session.jsonl"
cat >"$JSONL_FILE" <<'JSONL'
{"message":{"usage":{"input_tokens":1000000,"output_tokens":100000},"model":"claude-opus-4-6"}}
JSONL

# 1. compute_running_cost returns correct cost for opus
cost=$(compute_running_cost "$JSONL_FILE")
# 1M input * $15/1M = $15.00, 100K output * $75/1M = $7.50, total = $22.50
assert_eq "opus cost calculation" "$cost" "22.50"

# 2. Zero tokens returns zero
echo '{"message":{"usage":{"input_tokens":0,"output_tokens":0},"model":"claude-opus-4-6"}}' > "$JSONL_FILE"
cost=$(compute_running_cost "$JSONL_FILE")
assert_eq "zero tokens = zero cost" "$cost" "0.00"

# 3. check_budget returns 1 when over budget
echo '{"message":{"usage":{"input_tokens":1000000,"output_tokens":100000},"model":"claude-opus-4-6"}}' > "$JSONL_FILE"
if check_budget "$JSONL_FILE" "5.00"; then
  echo "FAIL: should exceed budget" >&2; ((++fail))
else
  ((++pass))
fi

# 4. check_budget returns 0 when under budget
if check_budget "$JSONL_FILE" "50.00"; then
  ((++pass))
else
  echo "FAIL: should be under budget" >&2; ((++fail))
fi

echo ""
echo "test-budget-enforcement: $pass passed, $fail failed"
[ "$fail" -eq 0 ] || exit 1
```

- [ ] **Step 2: Run the test to confirm it fails**

Run: `bash tests/test-budget-enforcement.sh`
Expected: FAIL — `compute_running_cost` not found

- [ ] **Step 3: Implement budget functions in `bin/agent-lib.sh`**

```bash
# --- Budget Enforcement ---

# Compute running cost from a session JSONL file.
# Prints cost as decimal string (e.g., "22.50").
# Usage: compute_running_cost <jsonl_file>
compute_running_cost() {
  local jsonl_file="$1"
  [ -f "$jsonl_file" ] || { echo "0.00"; return 0; }

  python3 -c "
import json, sys

PRICES = {
    'claude-opus-4-6':   (15.0, 75.0),
    'claude-sonnet-4-6': (3.0, 15.0),
    'claude-haiku-4-5':  (0.80, 4.0),
    'gpt-5.4':           (2.50, 10.0),
    'o3':                (10.0, 40.0),
    'o4-mini':           (1.10, 4.40),
}
DEFAULT_PRICE = (3.0, 15.0)

total = 0.0
for line in open('$jsonl_file'):
    line = line.strip()
    if not line:
        continue
    try:
        obj = json.loads(line)
        msg = obj.get('message', obj)
        usage = msg.get('usage', {})
        model = msg.get('model', '')
        inp = usage.get('input_tokens', 0) + usage.get('cache_creation_input_tokens', 0)
        out = usage.get('output_tokens', 0)
        price = PRICES.get(model, DEFAULT_PRICE)
        total += (inp / 1_000_000) * price[0] + (out / 1_000_000) * price[1]
    except (json.JSONDecodeError, KeyError):
        continue

print(f'{total:.2f}')
" 2>/dev/null || echo "0.00"
}

# Check if running cost exceeds budget.
# Returns 0 if under budget, 1 if over.
# Usage: check_budget <jsonl_file> <max_cost>
check_budget() {
  local jsonl_file="$1" max_cost="$2"
  local current_cost
  current_cost=$(compute_running_cost "$jsonl_file")

  python3 -c "
import sys
current = float('$current_cost')
budget = float('$max_cost')
sys.exit(0 if current <= budget else 1)
" 2>/dev/null
}

# Start a background budget monitor for a container.
# Polls session JSONL every 10s and kills agent if budget exceeded.
# Usage: start_budget_monitor <container_name> <max_cost> <events_file>
start_budget_monitor() {
  local container_name="$1" max_cost="$2" events_file="$3"

  (
    while true; do
      sleep 10

      # Check if container is still running
      local state
      state=$(vm_exec docker inspect --format '{{.State.Status}}' "$container_name" 2>/dev/null || echo "gone")
      [ "$state" = "running" ] || break

      # Get session JSONL from container
      local tmp_jsonl
      tmp_jsonl=$(mktemp)
      vm_exec docker exec "$container_name" \
        find /home/agent/.claude /home/agent/.codex -name '*.jsonl' -exec cat {} + 2>/dev/null > "$tmp_jsonl" || true

      if ! check_budget "$tmp_jsonl" "$max_cost"; then
        local current_cost
        current_cost=$(compute_running_cost "$tmp_jsonl")
        warn "Budget exceeded for $container_name: \$$current_cost > \$$max_cost — stopping agent"
        emit_event "$events_file" "agent.budget_exceeded" \
          "$(printf '{"name":"%s","estimated_cost":%s,"budget":%s}' "$container_name" "$current_cost" "$max_cost")"
        vm_exec docker exec "$container_name" kill -TERM 1 2>/dev/null || true
        sleep 5
        vm_exec docker stop "$container_name" 2>/dev/null || true
        break
      fi

      rm -f "$tmp_jsonl"
    done
  ) &
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `bash tests/test-budget-enforcement.sh`
Expected: All PASS

- [ ] **Step 5: Wire monitor into `cmd_spawn()`**

In `cmd_spawn()`, after container starts successfully (around line 1070), add:

```bash
  # Start budget monitor if --max-cost is set
  if [ -n "$max_cost" ]; then
    local events_dir="${XDG_CONFIG_HOME:-$HOME/.config}/safe-agentic"
    local events_file="$events_dir/events.jsonl"
    mkdir -p "$events_dir"
    start_budget_monitor "$container_name" "$max_cost" "$events_file"
  fi
```

- [ ] **Step 6: Run full test suite**

Run: `bash tests/run-all.sh`
Expected: All pass

- [ ] **Step 7: Commit**

```bash
git add bin/agent bin/agent-lib.sh tests/test-budget-enforcement.sh
git commit -m "feat: enforce --max-cost budget with kill switch

Background monitor polls session JSONL every 10s.
Kills agent process and emits agent.budget_exceeded event
when estimated cost exceeds the budget."
```

---

## Task 6: Notification Integration (`--notify`)

**Files:**
- Modify: `bin/agent:751-900` (cmd_spawn — add --notify flag)
- Modify: `bin/agent-lib.sh` (add `send_notification()`)
- Create: `tests/test-notify.sh`

- [ ] **Step 1: Write the test**

Create `tests/test-notify.sh`:

```bash
#!/usr/bin/env bash
# Tests for notification system.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

source "$REPO_DIR/bin/agent-lib.sh" 2>/dev/null || true

pass=0
fail=0

assert_eq() {
  local label="$1" got="$2" want="$3"
  if [ "$got" = "$want" ]; then ((++pass)); else echo "FAIL: $label — got '$got' want '$want'" >&2; ((++fail)); fi
}

# 1. send_notification with command type executes the command
NOTIFY_OUT="$TMP_DIR/notify.out"
send_notification "command" "echo notified > $NOTIFY_OUT" "test-agent" "completed" ""
assert_eq "command notification" "$(cat "$NOTIFY_OUT" 2>/dev/null | tr -d '\n')" "notified"

# 2. send_notification with terminal type (just verifies no crash on macOS)
send_notification "terminal" "" "test-agent" "completed" "" 2>/dev/null || true
((++pass))  # If we get here without crashing, it's a pass

# 3. parse_notify_targets splits comma-separated targets
targets=$(parse_notify_targets "terminal,slack,command:my-script")
assert_eq "parse 3 targets" "$(echo "$targets" | wc -l | tr -d ' ')" "3"

echo ""
echo "test-notify: $pass passed, $fail failed"
[ "$fail" -eq 0 ] || exit 1
```

- [ ] **Step 2: Run test to confirm it fails**

Run: `bash tests/test-notify.sh`
Expected: FAIL — `send_notification` not found

- [ ] **Step 3: Implement notification functions in `bin/agent-lib.sh`**

```bash
# --- Notifications ---

# Send a notification via a specific sink type.
# Usage: send_notification <type> <target> <agent_name> <status> <details>
send_notification() {
  local ntype="$1" target="$2" agent_name="$3" status="$4" details="${5:-}"

  case "$ntype" in
    terminal)
      # macOS notification center
      if command -v osascript &>/dev/null; then
        osascript -e "display notification \"Agent $agent_name: $status\" with title \"safe-agentic\"" 2>/dev/null || true
      elif command -v notify-send &>/dev/null; then
        notify-send "safe-agentic" "Agent $agent_name: $status" 2>/dev/null || true
      fi
      ;;
    slack)
      local webhook_url="${target:-$(config_get notify_slack_webhook 2>/dev/null || echo "")}"
      [ -n "$webhook_url" ] && {
        curl -s -X POST -H "Content-Type: application/json" \
          -d "{\"text\":\"Agent $agent_name: $status. $details\"}" \
          "$webhook_url" 2>/dev/null || true
      }
      ;;
    command:*|command)
      local cmd="${target:-${ntype#command:}}"
      eval "$cmd" 2>/dev/null || true
      ;;
    *)
      warn "Unknown notification type: $ntype"
      ;;
  esac
}

# Parse comma-separated notify targets into lines.
# Usage: parse_notify_targets "terminal,slack,command:my-script"
parse_notify_targets() {
  local input="$1"
  echo "$input" | tr ',' '\n'
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `bash tests/test-notify.sh`
Expected: All PASS

- [ ] **Step 5: Add `--notify` flag to `cmd_spawn()`**

In the flag parsing loop:

```bash
      --notify)
        notify_targets="$2"
        shift 2
        ;;
```

After container exits (in the on-exit handler area), dispatch notifications:

```bash
  if [ -n "${notify_targets:-}" ]; then
    local target
    while IFS= read -r target; do
      [ -n "$target" ] || continue
      send_notification "$target" "" "$container_name" "completed" ""
    done < <(parse_notify_targets "$notify_targets")
  fi
```

- [ ] **Step 6: Run full test suite**

Run: `bash tests/run-all.sh`
Expected: All pass

- [ ] **Step 7: Commit**

```bash
git add bin/agent bin/agent-lib.sh tests/test-notify.sh
git commit -m "feat: add --notify flag for agent completion notifications

Supports terminal (macOS/Linux native), slack (webhook),
and command:script targets. Comma-separated for multiple."
```

---

## Task 7: Container State Snapshots & Fork

**Files:**
- Modify: `bin/agent:2515-2630` (cmd_checkpoint — extend create/list/revert, add fork)
- Create: `tests/test-checkpoint-snapshots.sh`

- [ ] **Step 1: Write the test**

Create `tests/test-checkpoint-snapshots.sh`:

```bash
#!/usr/bin/env bash
# Tests for container snapshot checkpoints.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

AGENT_BIN="$REPO_DIR/bin/agent"
OUT_LOG="$TMP_DIR/out.log"
ERR_LOG="$TMP_DIR/err.log"
FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"
COMMITTED_IMAGE=""

mkdir -p "$FAKE_BIN"
cat >"$FAKE_BIN/orb" <<'FAKEORB'
#!/usr/bin/env bash
set -euo pipefail
log_file="${TEST_ORB_LOG:?}"
cmd="${1:-}"; shift || true
case "$cmd" in
  list) echo "safe-agentic" ;;
  run)
    [ "${1:-}" = "-m" ] && shift 2
    printf '%s\n' "$*" >>"$log_file"
    # docker inspect State.Status
    if [[ "$*" == *"State.Status"* ]]; then echo "running"; exit 0; fi
    # docker inspect agent-type
    if [[ "$*" == *"agent-type"* ]]; then echo "claude"; exit 0; fi
    # docker commit
    if [[ "${1:-}" = "docker" && "${2:-}" = "commit" ]]; then
      echo "sha256:abc123"; exit 0
    fi
    # docker exec — tmux list-panes
    if [[ "$*" == *"tmux list-panes"* ]]; then echo "42"; exit 0; fi
    # docker exec — git stash create
    if [[ "$*" == *"stash create"* ]]; then echo "abc1234"; exit 0; fi
    # docker exec — git update-ref
    if [[ "$*" == *"update-ref"* ]]; then exit 0; fi
    # docker exec — cat/tee to checkpoints.jsonl
    if [[ "$*" == *"checkpoints.jsonl"* ]]; then exit 0; fi
    # docker exec — kill
    if [[ "$*" == *"kill -"* ]]; then exit 0; fi
    exit 0 ;;
  *) exit 0 ;;
esac
FAKEORB
chmod +x "$FAKE_BIN/orb"

pass=0
fail=0

run_ok() {
  local label="$1"; shift
  : >"$ORB_LOG"
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
    ((++pass))
  else
    echo "FAIL: $label" >&2; cat "$ERR_LOG" >&2; ((++fail))
  fi
}

assert_log_contains() {
  local needle="$1" label="$2"
  if grep -qF "$needle" "$ORB_LOG" 2>/dev/null; then
    ((++pass))
  else
    echo "FAIL: $label — expected: $needle" >&2; ((++fail))
  fi
}

# 1. checkpoint save calls docker commit
run_ok "checkpoint-save" "$AGENT_BIN" checkpoint create test-container "my-label"
assert_log_contains "docker commit" "docker commit called"

# 2. checkpoint fork calls docker run with snapshot image
run_ok "checkpoint-fork" "$AGENT_BIN" checkpoint fork test-container new-fork "my-label"
assert_log_contains "docker run" "docker run called for fork"

echo ""
echo "test-checkpoint-snapshots: $pass passed, $fail failed"
[ "$fail" -eq 0 ] || exit 1
```

- [ ] **Step 2: Run test to confirm it fails**

Run: `bash tests/test-checkpoint-snapshots.sh`
Expected: FAIL — `fork` subcommand not recognized

- [ ] **Step 3: Extend `cmd_checkpoint()` with Docker commit and fork**

In `bin/agent` at `cmd_checkpoint()` (line 2515), modify the `create` subcommand and add `fork`:

For `create` — after the existing git stash, add docker commit:

```bash
    # Docker commit for full container state snapshot
    local snapshot_tag="safe-agentic-checkpoint:${name}-${timestamp}"
    info "Creating container snapshot: $snapshot_tag"

    # Pause agent process
    local agent_pid
    agent_pid=$(vm_exec docker exec "$name" tmux list-panes -t agent -F '#{pane_pid}' 2>/dev/null || echo "")
    [ -n "$agent_pid" ] && vm_exec docker exec "$name" kill -STOP "$agent_pid" 2>/dev/null || true

    # Commit container filesystem
    vm_exec docker commit "$name" "$snapshot_tag"

    # Store snapshot metadata
    vm_exec docker exec "$name" bash -c "
      echo '{\"timestamp\":\"$timestamp\",\"label\":\"$label\",\"image\":\"$snapshot_tag\",\"git_ref\":\"$sha\"}' \
        >> /workspace/.safe-agentic/checkpoints.jsonl
    " 2>/dev/null || true

    # Resume agent process
    [ -n "$agent_pid" ] && vm_exec docker exec "$name" kill -CONT "$agent_pid" 2>/dev/null || true
```

Add `fork` subcommand to the case dispatch:

```bash
    fork)
      local name="${2:?Usage: agent checkpoint fork <container> <new-name> [label]}"
      local new_name="${3:?Usage: agent checkpoint fork <container> <new-name> [label]}"
      local label="${4:-fork-$(date +%s)}"

      validate_name_component "$new_name" "fork name"

      # Get latest snapshot
      local snapshot_tag
      snapshot_tag=$(vm_exec docker exec "$name" bash -c "
        tail -1 /workspace/.safe-agentic/checkpoints.jsonl 2>/dev/null | python3 -c 'import sys,json; print(json.load(sys.stdin)[\"image\"])' 2>/dev/null
      " || echo "")
      [ -n "$snapshot_tag" ] || die "No checkpoint found for $name. Run 'agent checkpoint create' first."

      info "Forking $name as $new_name from $snapshot_tag..."

      # Get original container labels to reconstruct config
      local agent_type
      agent_type=$(vm_exec docker inspect --format '{{index .Config.Labels "safe-agentic.agent-type"}}' "$name" 2>/dev/null || echo "claude")

      # Create new network and run from snapshot
      local fork_network="${new_name}-net"
      create_managed_network "$fork_network"
      vm_exec docker run -d \
        --name "$new_name" \
        --network "$fork_network" \
        --label "safe-agentic.agent-type=$agent_type" \
        --label "safe-agentic.forked-from=$name" \
        "$snapshot_tag" \
        sleep infinity

      info "Fork created: $new_name (use 'agent attach $new_name' to connect)"
      ;;
```

- [ ] **Step 4: Update `cmd_checkpoint()` subcommand dispatch**

At line 2517, add `fork` to the case:

```bash
  local subcmd="${1:?Usage: agent checkpoint <create|list|revert|fork> ...}"
  shift
  case "$subcmd" in
    create) ... ;;
    list)   ... ;;
    revert) ... ;;
    fork)   ... ;;
    *)      die "Unknown checkpoint subcommand: $subcmd" ;;
  esac
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `bash tests/test-checkpoint-snapshots.sh`
Expected: All PASS

- [ ] **Step 6: Run full test suite**

Run: `bash tests/run-all.sh`
Expected: All pass

- [ ] **Step 7: Commit**

```bash
git add bin/agent tests/test-checkpoint-snapshots.sh
git commit -m "feat: add container snapshots and fork to checkpoint system

checkpoint create now does docker commit alongside git stash.
checkpoint fork creates a new container from the latest snapshot."
```

---

## Task 8: Session Event Log

**Files:**
- Modify: `entrypoint.sh:88-168` (add session event log writer alongside agent launch)
- Modify: `bin/agent-session.sh` (wrap agent launch to tee session events)
- Create: `tests/test-session-events.sh`

- [ ] **Step 1: Write the test**

Create `tests/test-session-events.sh`:

```bash
#!/usr/bin/env bash
# Tests for session event log format.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

pass=0
fail=0

assert_eq() {
  local label="$1" got="$2" want="$3"
  if [ "$got" = "$want" ]; then ((++pass)); else echo "FAIL: $label — got '$got' want '$want'" >&2; ((++fail)); fi
}

# Test the session event writer script
SESSION_DIR="$TMP_DIR/.safe-agentic"
mkdir -p "$SESSION_DIR"

# Simulate writing a session start event
cat >"$TMP_DIR/write-session-event.sh" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
session_file="${1:?}"
event_type="${2:?}"
data="${3:-{}}"
ts=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
printf '{"ts":"%s","type":"%s","data":%s}\n' "$ts" "$event_type" "$data" >> "$session_file"
SCRIPT
chmod +x "$TMP_DIR/write-session-event.sh"

EVENT_FILE="$SESSION_DIR/session-events.jsonl"

# 1. Write session.start event
bash "$TMP_DIR/write-session-event.sh" "$EVENT_FILE" "session.start" '{"agent":"claude","model":"opus-4-6"}'
assert_eq "event file created" "$(test -f "$EVENT_FILE" && echo yes)" "yes"

# 2. Verify JSONL format
line=$(head -1 "$EVENT_FILE")
assert_eq "has type field" "$(echo "$line" | python3 -c 'import sys,json; print(json.load(sys.stdin)["type"])')" "session.start"
assert_eq "has agent data" "$(echo "$line" | python3 -c 'import sys,json; print(json.load(sys.stdin)["data"]["agent"])')" "claude"

# 3. Multiple events append
bash "$TMP_DIR/write-session-event.sh" "$EVENT_FILE" "session.end" '{"exit_code":0}'
assert_eq "two events" "$(wc -l < "$EVENT_FILE" | tr -d ' ')" "2"

echo ""
echo "test-session-events: $pass passed, $fail failed"
[ "$fail" -eq 0 ] || exit 1
```

- [ ] **Step 2: Run test to verify it passes (this tests the format, not integration)**

Run: `bash tests/test-session-events.sh`
Expected: PASS (the script is self-contained)

- [ ] **Step 3: Add session event writer to `bin/agent-session.sh`**

At the start of the agent session (after the tmux setup), add:

```bash
# Session event log
SESSION_EVENTS="/workspace/.safe-agentic/session-events.jsonl"
mkdir -p "$(dirname "$SESSION_EVENTS")"

write_session_event() {
  local event_type="$1" data="${2:-{}}"
  local ts
  ts=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
  printf '{"ts":"%s","type":"%s","data":%s}\n' "$ts" "$event_type" "$data" >> "$SESSION_EVENTS"
}

write_session_event "session.start" \
  "$(printf '{"agent":"%s","repos":"%s"}' "${AGENT_TYPE:-unknown}" "${REPOS:-}")"
```

At the end (after agent exits), add:

```bash
write_session_event "session.end" \
  "$(printf '{"exit_code":%d}' "${agent_exit_code:-0}")"
```

- [ ] **Step 4: Run full test suite**

Run: `bash tests/run-all.sh`
Expected: All pass

- [ ] **Step 5: Commit**

```bash
git add bin/agent-session.sh entrypoint.sh tests/test-session-events.sh
git commit -m "feat: add session event log (JSONL) for structured history

Writes session.start and session.end events to
/workspace/.safe-agentic/session-events.jsonl."
```

---

## Task 9: Fleet Shared Task List

**Files:**
- Modify: `bin/agent:3402-3586` (cmd_fleet — create shared volume, mount in containers)
- Modify: `config/security-preamble.md` (add `{{FLEET_TASKS}}` placeholder)
- Modify: `entrypoint.sh` (handle fleet task injection)
- Create: `tests/test-fleet-tasks.sh`

- [ ] **Step 1: Write the test**

Create `tests/test-fleet-tasks.sh`:

```bash
#!/usr/bin/env bash
# Tests for fleet shared task list feature.
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
  local label="$1" haystack="$2" needle="$3"
  if [[ "$haystack" == *"$needle"* ]]; then ((++pass)); else echo "FAIL: $label — expected: $needle" >&2; ((++fail)); fi
}

assert_not_contains() {
  local label="$1" haystack="$2" needle="$3"
  if [[ "$haystack" == *"$needle"* ]]; then echo "FAIL: $label — should NOT contain: $needle" >&2; ((++fail)); else ((++pass)); fi
}

# 1. Fleet with shared_tasks: true creates shared volume in dry-run
cat >"$MANIFEST" <<'YAML'
name: test-fleet
shared_tasks: true
agents:
  - name: backend
    type: claude
    repo: https://github.com/org/api.git
    prompt: "Fix backend"
  - name: frontend
    type: claude
    repo: https://github.com/org/web.git
    prompt: "Fix frontend"
YAML

output=$("$AGENT_BIN" fleet "$MANIFEST" --dry-run 2>&1 || true)
assert_contains "shared volume in dry-run" "$output" "fleet-"
assert_contains "fleet mount" "$output" "/fleet"

# 2. Fleet without shared_tasks does NOT create shared volume
cat >"$MANIFEST" <<'YAML'
name: test-fleet-no-tasks
agents:
  - name: solo
    type: claude
    repo: https://github.com/org/r.git
    prompt: "Work alone"
YAML

output=$("$AGENT_BIN" fleet "$MANIFEST" --dry-run 2>&1 || true)
assert_not_contains "no shared volume" "$output" "/fleet"

echo ""
echo "test-fleet-tasks: $pass passed, $fail failed"
[ "$fail" -eq 0 ] || exit 1
```

- [ ] **Step 2: Run test to confirm it fails**

Run: `bash tests/test-fleet-tasks.sh`
Expected: FAIL — `shared_tasks` not recognized

- [ ] **Step 3: Extend the fleet YAML parser**

In `cmd_fleet()`, within the embedded Python YAML parser (lines 3434-3542), add support for the `shared_tasks` top-level field:

In the Python parser output section, add:

```python
# Parse shared_tasks from top-level
shared_tasks = 'false'
for line in content.split('\n'):
    stripped = line.strip()
    if stripped.startswith('shared_tasks:'):
        val = stripped.split(':',1)[1].strip().lower()
        shared_tasks = 'true' if val in ('true', 'yes', '1') else 'false'
        break

print(f'SHARED_TASKS={shared_tasks}')
```

- [ ] **Step 4: Handle shared volume creation in `cmd_fleet()`**

After parsing the YAML output, check for `SHARED_TASKS=true` and create a volume:

```bash
  # Check for shared tasks
  local shared_tasks=false
  local fleet_volume=""
  if echo "$parsed" | grep -qF "SHARED_TASKS=true"; then
    shared_tasks=true
    fleet_volume="fleet-$(echo "$manifest_file" | md5sum | cut -c1-8)-$(date +%s)"
    if [ "$dry_run" = false ]; then
      vm_exec docker volume create "$fleet_volume" >/dev/null
      info "Created shared task volume: $fleet_volume"
    else
      info "[dry-run] Would create shared volume: $fleet_volume"
    fi
  fi

  # When spawning, add volume mount if shared_tasks is enabled
  if [ "$shared_tasks" = true ]; then
    cmd_line="$cmd_line -v $fleet_volume:/fleet"
  fi
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `bash tests/test-fleet-tasks.sh`
Expected: All PASS

- [ ] **Step 6: Add `fleet status` subcommand**

Add a `fleet status <manifest>` option that reads `/fleet/tasks.jsonl` from a running fleet container:

```bash
  # In cmd_fleet, before the YAML parsing:
  if [ "${1:-}" = "status" ]; then
    shift
    local status_manifest="${1:?Usage: agent fleet status <manifest>}"
    # Find running containers with fleet label
    local containers
    containers=$(vm_exec docker ps --filter "label=safe-agentic.fleet" --format '{{.Names}}' 2>/dev/null || echo "")
    if [ -z "$containers" ]; then
      info "No fleet containers running."
      return 0
    fi
    # Read tasks from first container's /fleet/tasks.jsonl
    local first_container
    first_container=$(echo "$containers" | head -1)
    vm_exec docker exec "$first_container" cat /fleet/tasks.jsonl 2>/dev/null || info "No tasks found."
    return 0
  fi
```

- [ ] **Step 7: Run full test suite**

Run: `bash tests/run-all.sh`
Expected: All pass

- [ ] **Step 8: Commit**

```bash
git add bin/agent config/security-preamble.md tests/test-fleet-tasks.sh
git commit -m "feat: add shared task list for fleet agents

Fleet manifests with shared_tasks: true create a Docker volume
mounted at /fleet/ in all containers. Enables agent-to-agent
coordination via tasks.jsonl."
```

---

## Task 10: Pipeline Conditional Branching

**Files:**
- Modify: `bin/agent:3668-3913` (cmd_pipeline — add `outputs`, `when` fields)
- Modify: `bin/agent:3595-3666` (parse_pipeline_yaml — parse new fields)
- Create: `tests/test-pipeline-conditions.sh`

- [ ] **Step 1: Write the test**

Create `tests/test-pipeline-conditions.sh`:

```bash
#!/usr/bin/env bash
# Tests for pipeline conditional branching.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

AGENT_BIN="$REPO_DIR/bin/agent"
PIPELINE="$TMP_DIR/pipeline.yaml"
OUT_LOG="$TMP_DIR/out.log"
ERR_LOG="$TMP_DIR/err.log"

pass=0
fail=0

assert_contains() {
  local label="$1" haystack="$2" needle="$3"
  if [[ "$haystack" == *"$needle"* ]]; then ((++pass)); else echo "FAIL: $label — expected: $needle" >&2; ((++fail)); fi
}

# 1. Pipeline with 'when' field appears in dry-run output
cat >"$PIPELINE" <<'YAML'
name: conditional-test
steps:
  - name: check
    type: claude
    repo: https://github.com/org/r.git
    prompt: "Run tests"
    outputs:
      result: "echo pass"
  - name: on-pass
    type: claude
    repo: https://github.com/org/r.git
    prompt: "Deploy"
    depends_on: check
    when: "pass"
  - name: on-fail
    type: claude
    repo: https://github.com/org/r.git
    prompt: "Fix tests"
    depends_on: check
    when: "fail"
YAML

output=$("$AGENT_BIN" pipeline "$PIPELINE" --dry-run 2>&1 || true)
assert_contains "step check in output" "$output" "check"
assert_contains "step on-pass in output" "$output" "on-pass"
assert_contains "step on-fail in output" "$output" "on-fail"
assert_contains "when field shown" "$output" "when"

echo ""
echo "test-pipeline-conditions: $pass passed, $fail failed"
[ "$fail" -eq 0 ] || exit 1
```

- [ ] **Step 2: Run test to confirm it fails**

Run: `bash tests/test-pipeline-conditions.sh`
Expected: FAIL — `when` and `outputs` fields not parsed

- [ ] **Step 3: Extend `parse_pipeline_yaml()` to handle `outputs` and `when`**

In the Python YAML parser inside `parse_pipeline_yaml()`, add parsing for `outputs` and `when` fields:

```python
# Inside the step field parser, add:
elif key == 'when':
    step['when'] = val
elif key == 'outputs':
    step['outputs'] = val
```

And in the output line, include:

```python
# Add to the output format
when_b64 = b64(step.get('when', ''))
outputs_b64 = b64(step.get('outputs', ''))
# Append: when=<b64> outputs=<b64>
```

- [ ] **Step 4: Extend `pipeline_run_step()` with condition evaluation**

In `pipeline_run_step()`, after dependency checks, add:

```bash
    # Check 'when' condition
    local when_condition
    when_condition=$(echo "$step_line" | grep -oP 'when=\S+' | cut -d= -f2 | base64 -d 2>/dev/null || echo "")
    if [ -n "$when_condition" ]; then
      local dep_name
      dep_name=$(echo "$step_line" | grep -oP 'depends_on=\S+' | cut -d= -f2 | base64 -d 2>/dev/null || echo "")
      local dep_output="${step_outputs[$dep_name]:-}"
      if [ "$dep_output" != "$when_condition" ]; then
        info "Skipping step '$step_name': condition '$when_condition' not met (got '$dep_output')"
        return 0
      fi
    fi
```

After step completes, evaluate outputs:

```bash
    # Evaluate outputs
    local outputs_expr
    outputs_expr=$(echo "$step_line" | grep -oP 'outputs=\S+' | cut -d= -f2 | base64 -d 2>/dev/null || echo "")
    if [ -n "$outputs_expr" ]; then
      local output_val
      output_val=$(vm_exec docker exec "pipeline-${step_name}" bash -c "$outputs_expr" 2>/dev/null || echo "")
      step_outputs["$step_name"]="$output_val"
    fi
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `bash tests/test-pipeline-conditions.sh`
Expected: All PASS

- [ ] **Step 6: Run full test suite**

Run: `bash tests/run-all.sh`
Expected: All pass

- [ ] **Step 7: Commit**

```bash
git add bin/agent tests/test-pipeline-conditions.sh
git commit -m "feat: add conditional branching to pipeline steps

Steps can define 'outputs' (shell command to evaluate) and
'when' (condition to match against dependency output).
Skips steps whose conditions are not met."
```

---

## Task 11: Web Dashboard — Go HTTP Server

**Files:**
- Create: `tui/dashboard.go` (HTTP server)
- Create: `tui/dashboard_handlers.go` (route handlers)
- Create: `tui/dashboard_templates.go` (HTML templates)
- Modify: `tui/main.go` (add `--dashboard` flag)
- Modify: `bin/agent:4383-4387` (add `dashboard` command dispatch)
- Create: `tests/test-dashboard.sh`

- [ ] **Step 1: Write a smoke test**

Create `tests/test-dashboard.sh`:

```bash
#!/usr/bin/env bash
# Smoke test for web dashboard binary.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TUI_BIN="$REPO_DIR/tui/agent-tui"

pass=0
fail=0

# 1. Binary exists and has --dashboard flag in help
if [ -x "$TUI_BIN" ]; then
  help_output=$("$TUI_BIN" --help 2>&1 || true)
  if [[ "$help_output" == *"dashboard"* ]]; then
    ((++pass))
  else
    echo "FAIL: --dashboard not in help output" >&2
    ((++fail))
  fi
else
  echo "SKIP: TUI binary not built" >&2
  exit 77
fi

echo ""
echo "test-dashboard: $pass passed, $fail failed"
[ "$fail" -eq 0 ] || exit 1
```

- [ ] **Step 2: Create `tui/dashboard.go`**

```go
package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

type Dashboard struct {
	bind    string
	poller  *Poller
	tmpl    *template.Template
}

func NewDashboard(bind string) *Dashboard {
	d := &Dashboard{
		bind:   bind,
		poller: NewPoller(),
	}
	d.tmpl = template.Must(template.New("").Parse(dashboardHTML))
	return d
}

func (d *Dashboard) Start() error {
	d.poller.Start()

	mux := http.NewServeMux()
	mux.HandleFunc("/", d.handleIndex)
	mux.HandleFunc("/agents/", d.handleAgentDetail)
	mux.HandleFunc("/events", d.handleSSE)
	mux.HandleFunc("/api/agents", d.handleAPIAgents)
	mux.HandleFunc("/api/agents/stop/", d.handleAPIStop)

	log.Printf("Dashboard running at http://%s", d.bind)
	return http.ListenAndServe(d.bind, mux)
}

func (d *Dashboard) handleIndex(w http.ResponseWriter, r *http.Request) {
	agents := d.poller.GetAgents()
	d.tmpl.ExecuteTemplate(w, "index", agents)
}

func (d *Dashboard) handleAgentDetail(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/agents/")
	name = strings.TrimSuffix(name, "/")

	if strings.HasSuffix(r.URL.Path, "/logs") {
		d.handleAgentLogs(w, r, strings.TrimSuffix(name, "/logs"))
		return
	}

	agents := d.poller.GetAgents()
	for _, a := range agents {
		if a.Name == name {
			d.tmpl.ExecuteTemplate(w, "detail", a)
			return
		}
	}
	http.NotFound(w, r)
}

func (d *Dashboard) handleAgentLogs(w http.ResponseWriter, r *http.Request, name string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	for {
		out, err := exec.Command("orb", "run", "-m", "safe-agentic",
			"docker", "exec", name,
			"tmux", "capture-pane", "-t", "agent", "-p", "-S", "-30").Output()
		if err != nil {
			fmt.Fprintf(w, "data: [agent stopped]\n\n")
			flusher.Flush()
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", strings.ReplaceAll(string(out), "\n", "\ndata: "))
		flusher.Flush()
		time.Sleep(2 * time.Second)
	}
}

func (d *Dashboard) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)

	for {
		agents := d.poller.GetAgents()
		data, _ := json.Marshal(agents)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
		time.Sleep(2 * time.Second)
	}
}

func (d *Dashboard) handleAPIAgents(w http.ResponseWriter, r *http.Request) {
	agents := d.poller.GetAgents()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(agents)
}

func (d *Dashboard) handleAPIStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/agents/stop/")
	exec.Command("orb", "run", "-m", "safe-agentic", "docker", "stop", name).Run()
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"stopped","name":"%s"}`, name)
}
```

- [ ] **Step 3: Create `tui/dashboard_templates.go`**

```go
package main

const dashboardHTML = `
{{define "index"}}<!DOCTYPE html>
<html><head>
<title>safe-agentic dashboard</title>
<meta charset="utf-8">
<style>
  body { font-family: -apple-system, system-ui, sans-serif; margin: 2rem; background: #0d1117; color: #c9d1d9; }
  h1 { color: #58a6ff; font-size: 1.4rem; }
  table { border-collapse: collapse; width: 100%; margin-top: 1rem; }
  th, td { padding: 0.6rem 1rem; text-align: left; border-bottom: 1px solid #21262d; }
  th { color: #8b949e; font-weight: 500; font-size: 0.85rem; text-transform: uppercase; }
  tr:hover { background: #161b22; }
  a { color: #58a6ff; text-decoration: none; }
  .status-running { color: #3fb950; } .status-running::before { content: "● "; }
  .status-exited { color: #f85149; } .status-exited::before { content: "✗ "; }
  .status-idle { color: #d29922; } .status-idle::before { content: "◐ "; }
  .btn { padding: 0.3rem 0.8rem; border: 1px solid #30363d; border-radius: 6px; background: #21262d; color: #c9d1d9; cursor: pointer; font-size: 0.8rem; }
  .btn:hover { background: #30363d; }
  .btn-danger { border-color: #f85149; color: #f85149; }
  .refresh { color: #8b949e; font-size: 0.8rem; margin-top: 0.5rem; }
</style>
</head><body>
<h1>safe-agentic dashboard</h1>
<table>
<tr><th>Name</th><th>Status</th><th>Type</th><th>CPU</th><th>Memory</th><th>Actions</th></tr>
{{range .}}
<tr>
  <td><a href="/agents/{{.Name}}">{{.Name}}</a></td>
  <td class="status-{{.Status}}">{{.Status}}</td>
  <td>{{.AgentType}}</td>
  <td>{{.CPUPerc}}</td>
  <td>{{.MemUsage}}</td>
  <td>
    {{if eq .Status "running"}}<button class="btn btn-danger" onclick="stopAgent('{{.Name}}')">Stop</button>{{end}}
  </td>
</tr>
{{else}}
<tr><td colspan="6" style="color:#8b949e;text-align:center;padding:2rem;">No agents running</td></tr>
{{end}}
</table>
<p class="refresh">Auto-refreshes every 5s</p>
<script>
function stopAgent(name) {
  if (confirm('Stop agent ' + name + '?')) {
    fetch('/api/agents/stop/' + name, {method: 'POST'}).then(() => location.reload());
  }
}
setTimeout(() => location.reload(), 5000);
</script>
</body></html>{{end}}

{{define "detail"}}<!DOCTYPE html>
<html><head>
<title>{{.Name}} — safe-agentic</title>
<meta charset="utf-8">
<style>
  body { font-family: -apple-system, system-ui, sans-serif; margin: 2rem; background: #0d1117; color: #c9d1d9; }
  h1 { color: #58a6ff; font-size: 1.4rem; }
  a { color: #58a6ff; text-decoration: none; }
  pre { background: #161b22; padding: 1rem; border-radius: 6px; overflow-x: auto; font-size: 0.85rem; line-height: 1.5; }
  .meta { color: #8b949e; margin-bottom: 1rem; }
  .meta span { margin-right: 2rem; }
</style>
</head><body>
<a href="/">&larr; Back</a>
<h1>{{.Name}}</h1>
<div class="meta">
  <span>Type: {{.AgentType}}</span>
  <span>Status: {{.Status}}</span>
  <span>CPU: {{.CPUPerc}}</span>
  <span>Memory: {{.MemUsage}}</span>
</div>
<h2>Live Output</h2>
<pre id="logs">Loading...</pre>
<script>
const pre = document.getElementById('logs');
const es = new EventSource('/agents/{{.Name}}/logs');
es.onmessage = function(e) { pre.textContent = e.data; };
es.onerror = function() { pre.textContent += '\n[Connection lost]'; };
</script>
</body></html>{{end}}
`
```

- [ ] **Step 4: Add dashboard flag to `tui/main.go`**

In `main()`, add a flag check before TUI initialization:

```go
func main() {
	// Check for dashboard mode
	for _, arg := range os.Args[1:] {
		if arg == "--dashboard" || arg == "dashboard" {
			bind := "localhost:8420"
			for i, a := range os.Args[1:] {
				if a == "--bind" && i+2 < len(os.Args) {
					bind = os.Args[i+2]
				}
			}
			d := NewDashboard(bind)
			log.Fatal(d.Start())
		}
		if arg == "--help" || arg == "-h" {
			fmt.Println("Usage: agent-tui [--dashboard [--bind host:port]]")
			fmt.Println("  --dashboard    Start web dashboard instead of TUI")
			fmt.Println("  --bind         Bind address (default: localhost:8420)")
			os.Exit(0)
		}
	}

	// ... existing TUI code ...
```

- [ ] **Step 5: Add `dashboard` to dispatch in `bin/agent`**

At line ~4383, add before the `tui)` case:

```bash
  dashboard)
    tui_bin="$REPO_DIR/tui/agent-tui"
    [ -x "$tui_bin" ] || die "TUI not built. Run 'make -C $REPO_DIR/tui build' first."
    exec "$tui_bin" --dashboard "$@"
    ;;
```

- [ ] **Step 6: Build and test**

Run: `cd /Users/florian/perso/safe-agentic/tui && go build -o agent-tui . && cd -`
Run: `bash tests/test-dashboard.sh`
Expected: PASS

- [ ] **Step 7: Run full test suite**

Run: `bash tests/run-all.sh`
Expected: All pass

- [ ] **Step 8: Commit**

```bash
git add tui/dashboard.go tui/dashboard_handlers.go tui/dashboard_templates.go tui/main.go bin/agent tests/test-dashboard.sh
git commit -m "feat: add web dashboard for agent fleet management

Lightweight Go HTTP server at localhost:8420 with agent grid,
live log streaming via SSE, and stop actions. Reuses TUI's
Docker polling infrastructure."
```

---

## Task 12: Session Replay Command

**Files:**
- Modify: `bin/agent` (add `cmd_replay()` and dispatch)
- Create: `tests/test-replay.sh`

- [ ] **Step 1: Write the test**

Create `tests/test-replay.sh`:

```bash
#!/usr/bin/env bash
# Tests for session replay command.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

AGENT_BIN="$REPO_DIR/bin/agent"
OUT_LOG="$TMP_DIR/out.log"
ERR_LOG="$TMP_DIR/err.log"
FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"

mkdir -p "$FAKE_BIN"

# Prepare fake session events
SESSION_EVENTS='{"ts":"2026-04-10T14:30:00Z","type":"session.start","data":{"agent":"claude","model":"opus-4-6"}}
{"ts":"2026-04-10T14:30:05Z","type":"tool.call","data":{"tool":"Read","file":"src/auth.py","tokens_in":150}}
{"ts":"2026-04-10T14:30:10Z","type":"tool.call","data":{"tool":"Edit","file":"src/auth.py","tokens_in":200,"tokens_out":500}}
{"ts":"2026-04-10T14:35:00Z","type":"session.end","data":{"exit_code":0}}'

cat >"$FAKE_BIN/orb" <<FAKEORB
#!/usr/bin/env bash
set -euo pipefail
log_file="\${TEST_ORB_LOG:?}"
cmd="\${1:-}"; shift || true
case "\$cmd" in
  list) echo "safe-agentic" ;;
  run)
    [ "\${1:-}" = "-m" ] && shift 2
    printf '%s\n' "\$*" >>"\$log_file"
    if [[ "\${1:-}" = "docker" && "\${2:-}" = "inspect" && "\$*" == *"State.Status"* ]]; then echo "exited"; exit 0; fi
    if [[ "\${1:-}" = "docker" && "\${2:-}" = "inspect" && "\$*" == *"agent-type"* ]]; then echo "claude"; exit 0; fi
    if [[ "\$*" == *"session-events.jsonl"* ]]; then
      printf '%s\n' '$SESSION_EVENTS'
      exit 0
    fi
    exit 0 ;;
  *) exit 0 ;;
esac
FAKEORB
chmod +x "$FAKE_BIN/orb"

pass=0
fail=0

# 1. Replay shows session events
PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" \
  "$AGENT_BIN" replay test-container >"$OUT_LOG" 2>"$ERR_LOG" || true
output=$(cat "$OUT_LOG")

if [[ "$output" == *"session.start"* ]] || [[ "$output" == *"Session started"* ]]; then
  ((++pass))
else
  echo "FAIL: replay should show session start" >&2
  ((++fail))
fi

if [[ "$output" == *"Read"* ]] || [[ "$output" == *"tool.call"* ]]; then
  ((++pass))
else
  echo "FAIL: replay should show tool calls" >&2
  ((++fail))
fi

echo ""
echo "test-replay: $pass passed, $fail failed"
[ "$fail" -eq 0 ] || exit 1
```

- [ ] **Step 2: Run test to confirm it fails**

Run: `bash tests/test-replay.sh`
Expected: FAIL — `Unknown command: replay`

- [ ] **Step 3: Implement `cmd_replay()` in `bin/agent`**

```bash
cmd_replay() {
  local name=""
  local tools_only=false
  local cost_timeline=false

  while [ $# -gt 0 ]; do
    case "$1" in
      --latest) name=$(resolve_latest_container); shift ;;
      --tools-only) tools_only=true; shift ;;
      --cost-timeline) cost_timeline=true; shift ;;
      -*) die "Unknown flag: $1" ;;
      *) name="$1"; shift ;;
    esac
  done

  [ -n "$name" ] || die "Usage: agent replay <container|--latest> [--tools-only] [--cost-timeline]"

  # Read session events from container
  local events
  events=$(vm_exec docker exec "$name" cat /workspace/.safe-agentic/session-events.jsonl 2>/dev/null || \
           vm_exec docker cp "$name:/workspace/.safe-agentic/session-events.jsonl" /dev/stdout 2>/dev/null || echo "")

  [ -n "$events" ] || die "No session events found for $name. Session event logging may not be enabled."

  # Format and display
  python3 -c "
import json, sys

tools_only = $( [ "$tools_only" = true ] && echo "True" || echo "False" )
cost_timeline = $( [ "$cost_timeline" = true ] && echo "True" || echo "False" )

PRICES = {
    'claude-opus-4-6':   (15.0, 75.0),
    'claude-sonnet-4-6': (3.0, 15.0),
    'claude-haiku-4-5':  (0.80, 4.0),
}
running_cost = 0.0
total_in = 0
total_out = 0

for line in '''$events'''.strip().split('\n'):
    if not line.strip():
        continue
    try:
        evt = json.loads(line)
    except json.JSONDecodeError:
        continue

    ts = evt.get('ts', '?')
    etype = evt.get('type', '?')
    data = evt.get('data', {})

    # Track costs
    tin = data.get('tokens_in', 0)
    tout = data.get('tokens_out', 0)
    total_in += tin
    total_out += tout
    model = data.get('model', 'claude-sonnet-4-6')
    price = PRICES.get(model, (3.0, 15.0))
    running_cost += (tin / 1_000_000) * price[0] + (tout / 1_000_000) * price[1]

    if tools_only and etype != 'tool.call':
        continue

    # Format output
    time_part = ts.split('T')[1].rstrip('Z') if 'T' in ts else ts

    if etype == 'session.start':
        agent = data.get('agent', '?')
        model = data.get('model', '?')
        print(f'[{time_part}] Session started ({agent} {model})')
    elif etype == 'tool.call':
        tool = data.get('tool', '?')
        file = data.get('file', '')
        tokens = f'({tin + tout} tokens)' if tin + tout > 0 else ''
        print(f'[{time_part}] {tool} {file} {tokens}')
    elif etype == 'git.commit':
        sha = data.get('sha', '?')[:7]
        msg = data.get('message', '')
        print(f'[{time_part}] Git commit: {sha} \"{msg}\"')
    elif etype == 'agent.message':
        content = data.get('content', '')[:80]
        print(f'[{time_part}] Agent: {content}...')
    elif etype == 'session.end':
        code = data.get('exit_code', '?')
        print(f'[{time_part}] Session ended (exit {code})')
    else:
        print(f'[{time_part}] {etype}: {json.dumps(data)[:80]}')

    if cost_timeline and running_cost > 0:
        print(f'           Running cost: \${running_cost:.4f}')

print(f'')
print(f'Total: {total_in:,} in / {total_out:,} out tokens | Cost: ~\${running_cost:.2f}')
" 2>/dev/null || die "Failed to parse session events"
}
```

- [ ] **Step 4: Add `replay` to dispatch**

```bash
  replay)     cmd_replay "$@" ;;
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `bash tests/test-replay.sh`
Expected: All PASS

- [ ] **Step 6: Run full test suite**

Run: `bash tests/run-all.sh`
Expected: All pass

- [ ] **Step 7: Commit**

```bash
git add bin/agent tests/test-replay.sh
git commit -m "feat: add session replay command

Replays agent sessions from session-events.jsonl with
timestamps, tool calls, and running cost. Supports
--tools-only and --cost-timeline filters."
```

---

## Task 13: Cost Dashboard Enhancements

**Files:**
- Modify: `bin/agent:2899-3010` (cmd_cost — add `--fleet`, `--history`, `--budget-report` flags)
- Modify: `bin/agent-lib.sh` (emit cost events to audit log on agent completion)
- Create: `tests/test-cost-dashboard.sh`

- [ ] **Step 1: Write the test**

Create `tests/test-cost-dashboard.sh`:

```bash
#!/usr/bin/env bash
# Tests for enhanced cost command.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

# Create fake audit log with cost entries
AUDIT_DIR="$TMP_DIR/config/safe-agentic"
mkdir -p "$AUDIT_DIR"
cat >"$AUDIT_DIR/audit.jsonl" <<'AUDIT'
{"timestamp":"2026-04-10T14:00:00Z","action":"cost","container":"agent-a","cost":1.23,"tokens":{"in":5000,"out":2000},"model":"opus-4-6","duration_s":300}
{"timestamp":"2026-04-10T15:00:00Z","action":"cost","container":"agent-b","cost":0.45,"tokens":{"in":2000,"out":500},"model":"sonnet-4-6","duration_s":120}
{"timestamp":"2026-04-09T10:00:00Z","action":"cost","container":"agent-c","cost":2.10,"tokens":{"in":10000,"out":3000},"model":"opus-4-6","duration_s":600}
AUDIT

pass=0
fail=0

assert_eq() {
  local label="$1" got="$2" want="$3"
  if [ "$got" = "$want" ]; then ((++pass)); else echo "FAIL: $label — got '$got' want '$want'" >&2; ((++fail)); fi
}

# Test history parsing directly
total=$(python3 -c "
import json
total = 0.0
for line in open('$AUDIT_DIR/audit.jsonl'):
    obj = json.loads(line)
    if obj.get('action') == 'cost':
        total += obj.get('cost', 0)
print(f'{total:.2f}')
")
assert_eq "total cost from audit" "$total" "3.78"

# Test filtering by date (last 1 day)
today_total=$(python3 -c "
import json
from datetime import datetime, timedelta
cutoff = datetime.utcnow() - timedelta(days=1)
total = 0.0
for line in open('$AUDIT_DIR/audit.jsonl'):
    obj = json.loads(line)
    if obj.get('action') == 'cost':
        ts = datetime.fromisoformat(obj['timestamp'].rstrip('Z'))
        if ts >= cutoff:
            total += obj.get('cost', 0)
print(f'{total:.2f}')
")
assert_eq "last 1 day cost" "$today_total" "1.68"

echo ""
echo "test-cost-dashboard: $pass passed, $fail failed"
[ "$fail" -eq 0 ] || exit 1
```

- [ ] **Step 2: Run the test to verify it passes (unit test on audit parsing)**

Run: `bash tests/test-cost-dashboard.sh`
Expected: PASS

- [ ] **Step 3: Extend `cmd_cost()` with `--history` flag**

In `cmd_cost()` flag parsing, add:

```bash
      --history)
        local period="$2"
        shift 2
        local audit_file="${XDG_CONFIG_HOME:-$HOME/.config}/safe-agentic/audit.jsonl"
        [ -f "$audit_file" ] || die "No audit log found at $audit_file"

        python3 -c "
import json, sys
from datetime import datetime, timedelta

period = '$period'
days = int(period.rstrip('d')) if period.endswith('d') else 7
cutoff = datetime.utcnow() - timedelta(days=days)

by_container = {}
total = 0.0
for line in open('$audit_file'):
    try:
        obj = json.loads(line)
        if obj.get('action') != 'cost':
            continue
        ts = datetime.fromisoformat(obj['timestamp'].rstrip('Z'))
        if ts < cutoff:
            continue
        name = obj.get('container', '?')
        cost = obj.get('cost', 0)
        by_container[name] = by_container.get(name, 0) + cost
        total += cost
    except (json.JSONDecodeError, KeyError, ValueError):
        continue

print(f'Cost history (last {days} days):')
print(f'')
for name, cost in sorted(by_container.items(), key=lambda x: -x[1]):
    print(f'  {name:40s} \${cost:.2f}')
print(f'')
print(f'  {\"Total\":40s} \${total:.2f}')
"
        return 0
        ;;
```

- [ ] **Step 4: Add cost event emission to `cmd_stop()`**

When an agent is stopped, append a cost entry to the audit log:

```bash
  # In cmd_stop, after container is stopped but before removal:
  local cost_estimate
  cost_estimate=$(cmd_cost_silent "$name" 2>/dev/null || echo "0.00")
  if [ "$cost_estimate" != "0.00" ]; then
    local audit_file="${XDG_CONFIG_HOME:-$HOME/.config}/safe-agentic/audit.jsonl"
    printf '{"timestamp":"%s","action":"cost","container":"%s","cost":%s}\n' \
      "$(date -u +"%Y-%m-%dT%H:%M:%SZ")" "$name" "$cost_estimate" >> "$audit_file"
  fi
```

- [ ] **Step 5: Run full test suite**

Run: `bash tests/run-all.sh`
Expected: All pass

- [ ] **Step 6: Commit**

```bash
git add bin/agent tests/test-cost-dashboard.sh
git commit -m "feat: add cost history and fleet-level cost tracking

agent cost --history 7d shows per-agent spend from audit log.
Cost events automatically recorded when agents are stopped."
```

---

## Task 14: Update Help Text and Documentation

**Files:**
- Modify: `bin/agent` (cmd_help — add new commands)
- Modify: `CLAUDE.md` (add new commands to reference)

- [ ] **Step 1: Update `cmd_help()` with new commands**

Add to the help output:

```
Quick start:
  run         Smart defaults — auto-SSH, auto-auth, auto-identity

Lifecycle:
  spawn       Full-control agent launch (all flags)
  ...

Observability:
  replay      Replay agent session from event log
  dashboard   Start web dashboard (localhost:8420)

Notifications:
  --on-complete "cmd"   Run command when agent succeeds
  --on-fail "cmd"       Run command when agent fails
  --notify targets      Send notifications (terminal,slack,command:script)
  --max-cost N.NN       Kill agent if estimated cost exceeds budget

Checkpoints:
  checkpoint create     Save git + container state
  checkpoint fork       Create new agent from checkpoint
```

- [ ] **Step 2: Update `CLAUDE.md` commands section**

Add `run`, `replay`, `dashboard` to the commands table. Add `--on-complete`, `--on-fail`, `--notify`, `--ephemeral-auth` to the spawn flags.

- [ ] **Step 3: Run `bash tests/test-syntax.sh`**

Expected: All scripts pass syntax check

- [ ] **Step 4: Run full test suite**

Run: `bash tests/run-all.sh`
Expected: All pass

- [ ] **Step 5: Commit**

```bash
git add bin/agent CLAUDE.md
git commit -m "docs: update help text and CLAUDE.md for v2 commands

Add run, replay, dashboard commands. Document --on-complete,
--on-fail, --notify, --ephemeral-auth, checkpoint fork."
```

---

## Task 15: Integration Test — End-to-End Workflow

**Files:**
- Create: `tests/test-v2-integration.sh`

- [ ] **Step 1: Write an integration test covering the full v2 workflow**

```bash
#!/usr/bin/env bash
# Integration test for v2 features using fake orb.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

AGENT_BIN="$REPO_DIR/bin/agent"
OUT_LOG="$TMP_DIR/out.log"
ERR_LOG="$TMP_DIR/err.log"

pass=0
fail=0

check() {
  local label="$1" result="$2"
  if [ "$result" = "ok" ]; then ((++pass)); else echo "FAIL: $label" >&2; ((++fail)); fi
}

# 1. agent run --dry-run works
"$AGENT_BIN" run https://github.com/org/r.git "Test prompt" --dry-run >"$OUT_LOG" 2>"$ERR_LOG" && check "run-dry" "ok" || check "run-dry" "fail"

# 2. agent help includes 'run'
"$AGENT_BIN" help 2>&1 | grep -qF "run" && check "help-run" "ok" || check "help-run" "fail"

# 3. agent help includes 'replay'
"$AGENT_BIN" help 2>&1 | grep -qF "replay" && check "help-replay" "ok" || check "help-replay" "fail"

# 4. agent help includes 'dashboard'
"$AGENT_BIN" help 2>&1 | grep -qF "dashboard" && check "help-dashboard" "ok" || check "help-dashboard" "fail"

# 5. --ephemeral-auth is recognized
"$AGENT_BIN" spawn claude --repo https://github.com/org/r.git --ephemeral-auth --dry-run >"$OUT_LOG" 2>"$ERR_LOG" && check "ephemeral-auth" "ok" || check "ephemeral-auth" "fail"

# 6. --on-complete is recognized
"$AGENT_BIN" spawn claude --repo https://github.com/org/r.git --on-complete "echo done" --dry-run >"$OUT_LOG" 2>"$ERR_LOG" && check "on-complete" "ok" || check "on-complete" "fail"

# 7. --notify is recognized
"$AGENT_BIN" spawn claude --repo https://github.com/org/r.git --notify terminal --dry-run >"$OUT_LOG" 2>"$ERR_LOG" && check "notify" "ok" || check "notify" "fail"

echo ""
echo "test-v2-integration: $pass passed, $fail failed"
[ "$fail" -eq 0 ] || exit 1
```

- [ ] **Step 2: Run the integration test**

Run: `bash tests/test-v2-integration.sh`
Expected: All PASS

- [ ] **Step 3: Add to `tests/run-all.sh` if not auto-discovered**

The test runner glob should pick it up automatically. Verify: `bash tests/run-all.sh`

- [ ] **Step 4: Commit**

```bash
git add tests/test-v2-integration.sh
git commit -m "test: add v2 integration test covering all new commands

Verifies run, replay, dashboard, --ephemeral-auth,
--on-complete, --on-fail, --notify are recognized."
```

---

## Summary

| Task | Feature | Files Changed | New Tests |
|------|---------|--------------|-----------|
| 1 | `cmd_run()` quick start | bin/agent, bin/agent-lib.sh | test-run-command.sh |
| 2 | Auth default flip | bin/agent | test-auth-defaults.sh |
| 3 | Git identity auto-detect | bin/agent, bin/agent-lib.sh | test-git-identity.sh |
| 4 | Event system | bin/agent, bin/agent-lib.sh | test-events.sh |
| 5 | Budget enforcement | bin/agent, bin/agent-lib.sh | test-budget-enforcement.sh |
| 6 | Notifications | bin/agent, bin/agent-lib.sh | test-notify.sh |
| 7 | Container snapshots & fork | bin/agent | test-checkpoint-snapshots.sh |
| 8 | Session event log | entrypoint.sh, bin/agent-session.sh | test-session-events.sh |
| 9 | Fleet shared tasks | bin/agent, config/security-preamble.md | test-fleet-tasks.sh |
| 10 | Pipeline conditions | bin/agent | test-pipeline-conditions.sh |
| 11 | Web dashboard | tui/*.go, bin/agent | test-dashboard.sh |
| 12 | Session replay | bin/agent | test-replay.sh |
| 13 | Cost dashboard | bin/agent, bin/agent-lib.sh | test-cost-dashboard.sh |
| 14 | Help & docs | bin/agent, CLAUDE.md | — |
| 15 | Integration test | — | test-v2-integration.sh |

**Execution order:** Tasks 1-3 (Phase 1, no deps) → 4-6 (Phase 2, event foundation) → 7-8 (Phase 3, state) → 9-10 (Phase 4, fleet) → 11-13 (Phase 5, observability) → 14-15 (docs & integration)

**Total new test files:** 12
**Estimated commits:** 15
