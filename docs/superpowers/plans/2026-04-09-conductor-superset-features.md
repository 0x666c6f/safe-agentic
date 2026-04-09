# Conductor & Superset Feature Port — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the developer UX gap with Conductor.build while maintaining safe-agentic's security moat. Add diff viewing, checkpoints, lifecycle scripts, todos, PR workflow, code review, plan mode, audit logging, cost tracking, fleet manifests, and agent pipelines across 3 phases.

**Architecture:** Features split across the bash CLI (`bin/agent`), shared library (`bin/agent-lib.sh`), entrypoint (`entrypoint.sh`), and Go TUI (`tui/`). New CLI commands are `cmd_*` functions in `bin/agent` dispatched via the case statement at line 1629. New TUI actions are methods on `Actions` in `tui/actions.go` with keybindings wired in `tui/app.go:handleInput`. Persistent data stored at `~/.config/safe-agentic/` on the host and `/workspace/.safe-agentic/` inside containers.

**Tech Stack:** Bash (CLI), Go + tview/tcell (TUI), JSONL (audit/metrics), JSON (config), Docker labels (metadata)

---

## File Map

### Phase 1 — Immediate (developer workflow)

| File | Action | Responsibility |
|------|--------|---------------|
| `bin/agent` | Modify | Add `cmd_diff`, `cmd_checkpoint`, `cmd_todo`, `cmd_pr` functions + case dispatch |
| `bin/agent-lib.sh` | Modify | Add `checkpoint_ref_prefix`, `create_checkpoint`, `list_checkpoints`, `revert_checkpoint` helpers |
| `tests/test-diff.sh` | Create | Tests for `agent diff` command |
| `tests/test-checkpoint.sh` | Create | Tests for `agent checkpoint` command |
| `tests/test-todo.sh` | Create | Tests for `agent todo` command |
| `tests/test-pr.sh` | Create | Tests for `agent pr` command |
| `tests/test-lifecycle-scripts.sh` | Create | Tests for `safe-agentic.json` lifecycle scripts |
| `entrypoint.sh` | Modify | Run setup script from `safe-agentic.json` after clone |
| `tui/actions.go` | Modify | Add `Diff()`, `Checkpoint()`, `Todo()`, `PR()` methods |
| `tui/app.go` | Modify | Wire `f`, `x`, `t`, `g` keybindings |
| `tui/footer.go` | Modify | Add new shortcut hints |

### Phase 2 — Competitive parity

| File | Action | Responsibility |
|------|--------|---------------|
| `bin/agent` | Modify | Add `cmd_review`, `cmd_audit` functions |
| `bin/agent-lib.sh` | Modify | Add `audit_log`, `audit_dir` helpers |
| `tests/test-review.sh` | Create | Tests for `agent review` |
| `tests/test-audit.sh` | Create | Tests for `agent audit` |
| `tests/test-cost.sh` | Create | Tests for `agent cost` |
| `tui/actions.go` | Modify | Add `Review()`, `Audit()`, `Cost()` methods |
| `tui/app.go` | Modify | Wire keybindings for review/audit |

### Phase 3 — Differentiation

| File | Action | Responsibility |
|------|--------|---------------|
| `bin/agent` | Modify | Add `cmd_fleet`, `cmd_pipeline` functions |
| `bin/agent-lib.sh` | Modify | Add `parse_fleet_manifest`, `parse_pipeline` helpers |
| `tests/test-fleet.sh` | Create | Tests for fleet manifests |
| `tests/test-pipeline.sh` | Create | Tests for agent pipelines |

---

## Phase 1: Developer Workflow Features

### Task 1: `agent diff` — Show Git Diff from Agent's Working Tree

**Files:**
- Modify: `bin/agent` (add `cmd_diff` function + dispatch)
- Create: `tests/test-diff.sh`
- Modify: `tui/actions.go` (add `Diff()` method)
- Modify: `tui/app.go` (wire `f` keybinding)
- Modify: `tui/footer.go` (add `f/Diff` shortcut)

- [ ] **Step 1: Write the test for `agent diff`**

Create `tests/test-diff.sh`:

```bash
#!/usr/bin/env bash
# Tests for agent diff command.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"
ERR_LOG="$TMP_DIR/error.log"
OUT_LOG="$TMP_DIR/out.log"
VERIFY_STATE="$TMP_DIR/verify-state"
mkdir -p "$FAKE_BIN"

# Fake orb that captures docker exec commands
cat >"$FAKE_BIN/orb" <<'ORBEOF'
#!/usr/bin/env bash
set -euo pipefail
log_file="${TEST_ORB_LOG:?}"
verify_state="${TEST_VERIFY_STATE:?}"
cmd="${1:-}"; shift || true
case "$cmd" in
  list) echo "safe-agentic" ;;
  run)
    [ "${1:-}" = "-m" ] && shift 2
    printf '%s\n' "$*" >>"$log_file"
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ]; then
      case "${4:-}" in
        *State.Status*) echo "running" ;;
        *agent-type*)   echo "claude" ;;
        *) echo "" ;;
      esac
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "ps" ]; then
      echo "agent-claude-test-1234"
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "exec" ]; then
      # Simulate git diff output
      if [[ "$*" == *"git diff"* ]]; then
        echo "diff --git a/main.py b/main.py"
        echo "--- a/main.py"
        echo "+++ b/main.py"
        echo "@@ -1,3 +1,4 @@"
        echo "+# new line"
        echo " existing line"
        exit 0
      fi
      # Simulate git diff --stat output
      if [[ "$*" == *"git diff --stat"* ]]; then
        echo " main.py | 1 +"
        echo " 1 file changed, 1 insertion(+)"
        exit 0
      fi
      exit 0
    fi
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-lc" ] && [[ "${3:-}" == *safe-agentic-hardening-verify* ]]; then
      : >"$verify_state"
      exit 0
    fi
    exit 0 ;;
  push|start|stop|create|ssh) ;;
  *) echo "unexpected orb command: $cmd" >&2; exit 1 ;;
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

pass=0
fail=0

run_ok() {
  local label="$1"; shift
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
    ((++pass))
  else
    echo "FAIL: $label: expected zero exit" >&2
    ((++fail))
  fi
}

run_fails() {
  local label="$1"; shift
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
    echo "FAIL: $label: expected non-zero exit" >&2
    ((++fail))
  else
    ((++pass))
  fi
}

assert_output_contains() {
  local needle="$1" label="$2"
  if grep -q "$needle" "$OUT_LOG" 2>/dev/null || grep -q "$needle" "$ERR_LOG" 2>/dev/null; then
    ((++pass))
  else
    echo "FAIL: $label: output should contain '$needle'" >&2
    ((++fail))
  fi
}

assert_log_contains() {
  local needle="$1" label="$2"
  if grep -q "$needle" "$ORB_LOG" 2>/dev/null; then
    ((++pass))
  else
    echo "FAIL: $label: orb log should contain '$needle'" >&2
    ((++fail))
  fi
}

# --- Tests ---

run_ok "diff help" bash "$REPO_DIR/bin/agent" diff --help
assert_output_contains "agent diff" "diff help mentions command"

run_fails "diff no args" bash "$REPO_DIR/bin/agent" diff
assert_output_contains "agent help diff" "diff usage pointer"

: >"$ORB_LOG"
run_ok "diff with name" bash "$REPO_DIR/bin/agent" diff agent-claude-test-1234
assert_log_contains "git diff" "diff runs git diff in container"

: >"$ORB_LOG"
run_ok "diff with stat" bash "$REPO_DIR/bin/agent" diff --stat agent-claude-test-1234
assert_log_contains "git diff --stat" "diff --stat runs git diff --stat"

echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bash tests/test-diff.sh`
Expected: FAIL — `Unknown command: diff`

- [ ] **Step 3: Add `cmd_diff` and help to `bin/agent`**

Add the help function after `print_help_peek` (around line 196):

```bash
print_help_diff() {
  cat <<EOF
Usage: agent diff <name>|--latest [--stat]

Show git diff from an agent's working tree.

Options:
  --stat    Show diffstat summary only

Examples:
  agent diff api-refactor
  agent diff --latest
  agent diff --latest --stat
EOF
}
```

Add the command function after `cmd_peek` (around line 1555):

```bash
cmd_diff() {
  local topic="diff"
  local name=""
  local latest=false
  local stat_only=false

  while [ $# -gt 0 ]; do
    case "$1" in
      --latest)
        latest=true
        shift
        ;;
      --stat)
        stat_only=true
        shift
        ;;
      -h|--help)
        print_help_diff
        return 0
        ;;
      *)
        if [ -z "$name" ] && ! $latest; then
          name="$1"
        else
          die_with_help "$topic" "Unexpected argument '$1'."
        fi
        shift
        ;;
    esac
  done

  require_vm
  name=$(resolve_target_container "$topic" "$name" "$latest")

  local state
  state=$(vm_exec docker inspect --format '{{.State.Status}}' "$name" 2>/dev/null || echo "unknown")
  [ "$state" = "running" ] || die "Container $name is not running (state: $state)."

  if $stat_only; then
    vm_exec docker exec "$name" bash -c 'cd /workspace/* 2>/dev/null && git diff --stat || git -C /workspace diff --stat' 2>/dev/null
  else
    vm_exec docker exec "$name" bash -c 'cd /workspace/* 2>/dev/null && git diff || git -C /workspace diff' 2>/dev/null
  fi
}
```

Add to the dispatch case statement (around line 1644):

```bash
  diff)       cmd_diff "$@" ;;
```

Add `diff` to the help dispatch in `cmd_help` and to `print_help_general`.

- [ ] **Step 4: Run test to verify it passes**

Run: `bash tests/test-diff.sh`
Expected: All pass

- [ ] **Step 5: Add diff view to TUI**

In `tui/actions.go`, add after the `YAMLView` method:

```go
// Diff shows git diff from the agent's working tree in an overlay.
func (ac *Actions) Diff() {
	agent := ac.selectedOrWarn()
	if agent == nil {
		return
	}
	if !agent.Running {
		ac.app.footer.ShowStatus("Agent not running", true)
		return
	}
	data, err := execOrbLong("docker", "exec", agent.Name, "bash", "-c",
		"cd /workspace/* 2>/dev/null && git diff || git -C /workspace diff")
	if err != nil {
		ac.app.footer.ShowStatus("Failed to get diff", true)
		return
	}
	content := string(data)
	if strings.TrimSpace(content) == "" {
		ac.app.footer.ShowStatus("No changes", false)
		return
	}
	ShowOverlay(ac.app, "diff", fmt.Sprintf("Diff: %s", agent.Name), content)
}
```

In `tui/app.go`, add in the `handleInput` rune switch (after the `'p'` case):

```go
		case 'f':
			ac.actions.Diff()
			return nil
```

In `tui/footer.go`, add to `allShortcuts` slice:

```go
	{"f", "Diff"},
```

- [ ] **Step 6: Run all tests to ensure nothing broke**

Run: `bash tests/run-all.sh`
Expected: All pass

- [ ] **Step 7: Commit**

```bash
git add bin/agent tests/test-diff.sh tui/actions.go tui/app.go tui/footer.go
git commit -m "feat: add agent diff command and TUI diff viewer

Shows git diff from agent's working tree. Available as 'agent diff <name>'
CLI command and 'f' keybinding in TUI."
```

---

### Task 2: `agent checkpoint` — Snapshot & Revert Agent Working Tree

**Files:**
- Modify: `bin/agent` (add `cmd_checkpoint` function + dispatch)
- Create: `tests/test-checkpoint.sh`
- Modify: `tui/actions.go` (add `Checkpoint()` method)
- Modify: `tui/app.go` (wire `x` keybinding)
- Modify: `tui/footer.go` (add shortcut)

- [ ] **Step 1: Write the test for `agent checkpoint`**

Create `tests/test-checkpoint.sh`:

```bash
#!/usr/bin/env bash
# Tests for agent checkpoint command.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"
ERR_LOG="$TMP_DIR/error.log"
OUT_LOG="$TMP_DIR/out.log"
VERIFY_STATE="$TMP_DIR/verify-state"
mkdir -p "$FAKE_BIN"

# Track checkpoint operations
CHECKPOINT_OPS="$TMP_DIR/checkpoint-ops.log"

cat >"$FAKE_BIN/orb" <<'ORBEOF'
#!/usr/bin/env bash
set -euo pipefail
log_file="${TEST_ORB_LOG:?}"
verify_state="${TEST_VERIFY_STATE:?}"
checkpoint_ops="${TEST_CHECKPOINT_OPS:?}"
cmd="${1:-}"; shift || true
case "$cmd" in
  list) echo "safe-agentic" ;;
  run)
    [ "${1:-}" = "-m" ] && shift 2
    printf '%s\n' "$*" >>"$log_file"
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ]; then
      case "${4:-}" in
        *State.Status*) echo "running" ;;
        *agent-type*)   echo "claude" ;;
        *) echo "" ;;
      esac
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "ps" ]; then
      echo "agent-claude-test-1234"
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "exec" ]; then
      if [[ "$*" == *"git stash create"* ]]; then
        echo "abc123def456"
        echo "create" >>"$checkpoint_ops"
        exit 0
      fi
      if [[ "$*" == *"safe-agentic/checkpoints"* ]] && [[ "$*" == *"update-ref"* ]]; then
        echo "save-ref" >>"$checkpoint_ops"
        exit 0
      fi
      if [[ "$*" == *"for-each-ref"* ]] && [[ "$*" == *"safe-agentic/checkpoints"* ]]; then
        printf 'abc123 1712678400 checkpoint-1\nabc124 1712678500 checkpoint-2\n'
        echo "list" >>"$checkpoint_ops"
        exit 0
      fi
      if [[ "$*" == *"git stash apply"* ]]; then
        echo "revert" >>"$checkpoint_ops"
        exit 0
      fi
      exit 0
    fi
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-lc" ] && [[ "${3:-}" == *safe-agentic-hardening-verify* ]]; then
      : >"$verify_state"
      exit 0
    fi
    exit 0 ;;
  push|start|stop|create|ssh) ;;
  *) echo "unexpected orb command: $cmd" >&2; exit 1 ;;
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

pass=0
fail=0

run_ok() {
  local label="$1"; shift
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" TEST_CHECKPOINT_OPS="$CHECKPOINT_OPS" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
    ((++pass))
  else
    echo "FAIL: $label: expected zero exit" >&2
    ((++fail))
  fi
}

run_fails() {
  local label="$1"; shift
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" TEST_CHECKPOINT_OPS="$CHECKPOINT_OPS" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
    echo "FAIL: $label: expected non-zero exit" >&2
    ((++fail))
  else
    ((++pass))
  fi
}

assert_output_contains() {
  local needle="$1" label="$2"
  if grep -q "$needle" "$OUT_LOG" 2>/dev/null || grep -q "$needle" "$ERR_LOG" 2>/dev/null; then
    ((++pass))
  else
    echo "FAIL: $label: output should contain '$needle'" >&2
    ((++fail))
  fi
}

# --- Tests ---

run_ok "checkpoint help" bash "$REPO_DIR/bin/agent" checkpoint --help
assert_output_contains "agent checkpoint" "checkpoint help mentions command"

run_fails "checkpoint no args" bash "$REPO_DIR/bin/agent" checkpoint
assert_output_contains "agent help checkpoint" "checkpoint usage pointer"

: >"$ORB_LOG" >"$CHECKPOINT_OPS"
run_ok "checkpoint create" bash "$REPO_DIR/bin/agent" checkpoint create agent-claude-test-1234
assert_output_contains "Checkpoint created" "create shows confirmation"

: >"$ORB_LOG" >"$CHECKPOINT_OPS"
run_ok "checkpoint list" bash "$REPO_DIR/bin/agent" checkpoint list agent-claude-test-1234
assert_output_contains "checkpoint-1" "list shows checkpoints"

echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bash tests/test-checkpoint.sh`
Expected: FAIL — `Unknown command: checkpoint`

- [ ] **Step 3: Add `cmd_checkpoint` to `bin/agent`**

Add help function:

```bash
print_help_checkpoint() {
  cat <<EOF
Usage: agent checkpoint <create|list|revert> <name>|--latest [label]

Manage working tree snapshots inside an agent's container.

Subcommands:
  create [label]   Create a checkpoint (uses git stash + refs)
  list             List all checkpoints
  revert <ref>     Revert to a checkpoint

Examples:
  agent checkpoint create api-refactor "before refactor"
  agent checkpoint list api-refactor
  agent checkpoint revert api-refactor checkpoint-1
  agent checkpoint create --latest
EOF
}
```

Add the command function:

```bash
cmd_checkpoint() {
  local topic="checkpoint"
  local subcmd=""
  local name=""
  local latest=false
  local label=""
  local checkpoint_ref=""

  [ $# -gt 0 ] || die_with_help "$topic" "Subcommand required (create|list|revert)."

  subcmd="$1"; shift

  case "$subcmd" in
    -h|--help) print_help_checkpoint; return 0 ;;
    create|list|revert) ;;
    *) die_with_help "$topic" "Unknown subcommand '$subcmd'." ;;
  esac

  while [ $# -gt 0 ]; do
    case "$1" in
      --latest)
        latest=true
        shift
        ;;
      -h|--help)
        print_help_checkpoint
        return 0
        ;;
      *)
        if [ -z "$name" ] && ! $latest; then
          name="$1"
        elif [ "$subcmd" = "create" ] && [ -z "$label" ]; then
          label="$1"
        elif [ "$subcmd" = "revert" ] && [ -z "$checkpoint_ref" ]; then
          checkpoint_ref="$1"
        else
          die_with_help "$topic" "Unexpected argument '$1'."
        fi
        shift
        ;;
    esac
  done

  require_vm
  name=$(resolve_target_container "$topic" "$name" "$latest")

  local state
  state=$(vm_exec docker inspect --format '{{.State.Status}}' "$name" 2>/dev/null || echo "unknown")
  [ "$state" = "running" ] || die "Container $name is not running (state: $state)."

  case "$subcmd" in
    create)
      [ -z "$label" ] && label="checkpoint-$(date +%s)"
      local stash_sha
      stash_sha=$(vm_exec docker exec "$name" bash -c \
        'cd /workspace/* 2>/dev/null || cd /workspace; git add -A && git stash create' 2>/dev/null)
      if [ -z "$stash_sha" ]; then
        ok "No changes to checkpoint."
        return 0
      fi
      vm_exec docker exec "$name" bash -c \
        "cd /workspace/* 2>/dev/null || cd /workspace; git update-ref refs/safe-agentic/checkpoints/$label $stash_sha" 2>/dev/null
      ok "Checkpoint created: $label ($stash_sha)"
      ;;
    list)
      vm_exec docker exec "$name" bash -c \
        'cd /workspace/* 2>/dev/null || cd /workspace; git for-each-ref --format="%(objectname:short) %(creatordate:unix) %(refname:lstrip=3)" refs/safe-agentic/checkpoints/' 2>/dev/null \
        | while IFS=' ' read -r sha ts ref; do
            local_ts=$(date -r "$ts" "+%Y-%m-%d %H:%M:%S" 2>/dev/null || date -d "@$ts" "+%Y-%m-%d %H:%M:%S" 2>/dev/null || echo "$ts")
            printf "  %-20s %s  %s\n" "$ref" "$local_ts" "$sha"
          done
      ;;
    revert)
      [ -n "$checkpoint_ref" ] || die_with_help "$topic" "Checkpoint ref required for revert."
      vm_exec docker exec "$name" bash -c \
        "cd /workspace/* 2>/dev/null || cd /workspace; git checkout . && git clean -fd && git stash apply refs/safe-agentic/checkpoints/$checkpoint_ref" 2>/dev/null
      ok "Reverted to checkpoint: $checkpoint_ref"
      ;;
  esac
}
```

Add to dispatch:

```bash
  checkpoint) cmd_checkpoint "$@" ;;
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bash tests/test-checkpoint.sh`
Expected: All pass

- [ ] **Step 5: Add checkpoint to TUI**

In `tui/actions.go`, add:

```go
// Checkpoint creates a checkpoint of the selected agent's working tree.
func (ac *Actions) Checkpoint() {
	agent := ac.selectedOrWarn()
	if agent == nil {
		return
	}
	if !agent.Running {
		ac.app.footer.ShowStatus("Agent not running", true)
		return
	}
	name := agent.Name
	label := fmt.Sprintf("checkpoint-%d", time.Now().Unix())
	ac.app.footer.ShowStatus("Creating checkpoint...", false)
	go func() {
		stashSha, err := execOrb("docker", "exec", name, "bash", "-c",
			"cd /workspace/* 2>/dev/null || cd /workspace; git add -A && git stash create")
		if err != nil || strings.TrimSpace(string(stashSha)) == "" {
			ac.app.tapp.QueueUpdateDraw(func() {
				ac.app.footer.ShowStatus("No changes to checkpoint", false)
			})
			return
		}
		sha := strings.TrimSpace(string(stashSha))
		_, err = execOrb("docker", "exec", name, "bash", "-c",
			fmt.Sprintf("cd /workspace/* 2>/dev/null || cd /workspace; git update-ref refs/safe-agentic/checkpoints/%s %s", label, sha))
		ac.app.tapp.QueueUpdateDraw(func() {
			if err != nil {
				ac.app.footer.ShowStatus("Checkpoint failed", true)
			} else {
				ac.app.footer.ShowStatus(fmt.Sprintf("Checkpoint: %s (%s)", label, sha[:7]), false)
			}
		})
	}()
}
```

In `tui/app.go`, add keybinding:

```go
		case 'x':
			ac.actions.Checkpoint()
			return nil
```

In `tui/footer.go`, add shortcut:

```go
	{"x", "Checkpoint"},
```

- [ ] **Step 6: Run all tests**

Run: `bash tests/run-all.sh`
Expected: All pass

- [ ] **Step 7: Commit**

```bash
git add bin/agent tests/test-checkpoint.sh tui/actions.go tui/app.go tui/footer.go
git commit -m "feat: add agent checkpoint for working tree snapshots

Supports create/list/revert via git stash refs. Available as CLI
command and 'x' keybinding in TUI."
```

---

### Task 3: Lifecycle Scripts (`safe-agentic.json`)

**Files:**
- Modify: `entrypoint.sh` (run setup script after clone)
- Create: `tests/test-lifecycle-scripts.sh`

- [ ] **Step 1: Write test for lifecycle script execution**

Create `tests/test-lifecycle-scripts.sh`:

```bash
#!/usr/bin/env bash
# Tests for safe-agentic.json lifecycle script support in entrypoint.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

pass=0
fail=0

assert_eq() {
  local actual="$1" expected="$2" label="$3"
  if [ "$actual" = "$expected" ]; then
    ((++pass))
  else
    echo "FAIL: $label: expected '$expected', got '$actual'" >&2
    ((++fail))
  fi
}

# Test: extract_lifecycle_script parses setup from JSON
WORKSPACE="$TMP_DIR/workspace/test-repo"
mkdir -p "$WORKSPACE"

cat >"$WORKSPACE/safe-agentic.json" <<'EOF'
{
  "scripts": {
    "setup": "echo setup-ran > /tmp/test-setup-marker"
  }
}
EOF

# Source the function we'll add to entrypoint.sh
# We test the parsing logic in isolation
parse_result=$(python3 -c "
import json, sys
with open('$WORKSPACE/safe-agentic.json') as f:
    data = json.load(f)
print(data.get('scripts', {}).get('setup', ''))
" 2>/dev/null || echo "")

assert_eq "$parse_result" "echo setup-ran > /tmp/test-setup-marker" "parse setup script from JSON"

# Test: missing file produces empty result
missing_result=$(python3 -c "
import json, sys
try:
    with open('$TMP_DIR/nonexistent/safe-agentic.json') as f:
        data = json.load(f)
    print(data.get('scripts', {}).get('setup', ''))
except:
    print('')
" 2>/dev/null || echo "")

assert_eq "$missing_result" "" "missing JSON file returns empty"

# Test: JSON without scripts key
cat >"$TMP_DIR/no-scripts.json" <<'EOF'
{"name": "test"}
EOF

no_scripts_result=$(python3 -c "
import json, sys
with open('$TMP_DIR/no-scripts.json') as f:
    data = json.load(f)
print(data.get('scripts', {}).get('setup', ''))
" 2>/dev/null || echo "")

assert_eq "$no_scripts_result" "" "JSON without scripts returns empty"

echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bash tests/test-lifecycle-scripts.sh`
Expected: Should pass (tests parsing logic). If python3 unavailable, test skips.

- [ ] **Step 3: Add lifecycle script runner to `entrypoint.sh`**

Add after the repo clone section (after line 183), before the agent launch section:

```bash
# ---------------------------------------------------------------------------
# Run setup script from safe-agentic.json (if present)
# ---------------------------------------------------------------------------
run_lifecycle_script() {
  local script_name="$1"
  local config_file=""

  # Find safe-agentic.json in any cloned repo
  for dir in /workspace/*/; do
    if [ -f "${dir}safe-agentic.json" ]; then
      config_file="${dir}safe-agentic.json"
      break
    fi
  done

  [ -n "$config_file" ] || return 0

  local script_cmd
  script_cmd=$(python3 -c "
import json, sys
try:
    with open('$config_file') as f:
        data = json.load(f)
    print(data.get('scripts', {}).get('$script_name', ''))
except:
    pass
" 2>/dev/null || true)

  [ -n "$script_cmd" ] || return 0

  echo "[entrypoint] Running $script_name script from safe-agentic.json..."
  (cd "$(dirname "$config_file")" && bash -c "$script_cmd") || {
    echo "[entrypoint] WARNING: $script_name script failed (exit $?)" >&2
  }
}

run_lifecycle_script "setup"
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bash tests/test-lifecycle-scripts.sh`
Expected: All pass

- [ ] **Step 5: Run full test suite**

Run: `bash tests/run-all.sh`
Expected: All pass (entrypoint tests may need adjustment for the new function)

- [ ] **Step 6: Commit**

```bash
git add entrypoint.sh tests/test-lifecycle-scripts.sh
git commit -m "feat: add safe-agentic.json lifecycle scripts

Entrypoint runs 'setup' script from safe-agentic.json after repo clone.
Supports 'setup' key under 'scripts' object."
```

---

### Task 4: `agent todo` — Track Merge Requirements

**Files:**
- Modify: `bin/agent` (add `cmd_todo` function + dispatch)
- Create: `tests/test-todo.sh`
- Modify: `tui/actions.go` (add `Todo()` method)
- Modify: `tui/app.go` (wire `t` keybinding)

- [ ] **Step 1: Write the test for `agent todo`**

Create `tests/test-todo.sh`:

```bash
#!/usr/bin/env bash
# Tests for agent todo command.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"
ERR_LOG="$TMP_DIR/error.log"
OUT_LOG="$TMP_DIR/out.log"
VERIFY_STATE="$TMP_DIR/verify-state"
TODO_FILE="$TMP_DIR/todos.json"
mkdir -p "$FAKE_BIN"

cat >"$FAKE_BIN/orb" <<'ORBEOF'
#!/usr/bin/env bash
set -euo pipefail
log_file="${TEST_ORB_LOG:?}"
verify_state="${TEST_VERIFY_STATE:?}"
todo_file="${TEST_TODO_FILE:?}"
cmd="${1:-}"; shift || true
case "$cmd" in
  list) echo "safe-agentic" ;;
  run)
    [ "${1:-}" = "-m" ] && shift 2
    printf '%s\n' "$*" >>"$log_file"
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ]; then
      case "${4:-}" in
        *State.Status*) echo "running" ;;
        *agent-type*)   echo "claude" ;;
        *) echo "" ;;
      esac
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "ps" ]; then
      echo "agent-claude-test-1234"
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "exec" ]; then
      # Simulate todo file operations
      if [[ "$*" == *"cat /workspace/.safe-agentic/todos.json"* ]]; then
        cat "$todo_file" 2>/dev/null || echo "[]"
        exit 0
      fi
      if [[ "$*" == *"tee /workspace/.safe-agentic/todos.json"* ]]; then
        cat > "$todo_file"
        exit 0
      fi
      if [[ "$*" == *"mkdir -p"* ]]; then
        exit 0
      fi
      exit 0
    fi
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-lc" ] && [[ "${3:-}" == *safe-agentic-hardening-verify* ]]; then
      : >"$verify_state"
      exit 0
    fi
    exit 0 ;;
  push|start|stop|create|ssh) ;;
  *) echo "unexpected orb command: $cmd" >&2; exit 1 ;;
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

pass=0
fail=0

run_ok() {
  local label="$1"; shift
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" TEST_TODO_FILE="$TODO_FILE" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
    ((++pass))
  else
    echo "FAIL: $label: expected zero exit" >&2
    ((++fail))
  fi
}

run_fails() {
  local label="$1"; shift
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" TEST_TODO_FILE="$TODO_FILE" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
    echo "FAIL: $label: expected non-zero exit" >&2
    ((++fail))
  else
    ((++pass))
  fi
}

assert_output_contains() {
  local needle="$1" label="$2"
  if grep -q "$needle" "$OUT_LOG" 2>/dev/null || grep -q "$needle" "$ERR_LOG" 2>/dev/null; then
    ((++pass))
  else
    echo "FAIL: $label: output should contain '$needle'" >&2
    ((++fail))
  fi
}

# --- Tests ---

run_ok "todo help" bash "$REPO_DIR/bin/agent" todo --help
assert_output_contains "agent todo" "todo help mentions command"

run_fails "todo no args" bash "$REPO_DIR/bin/agent" todo
assert_output_contains "agent help todo" "todo usage pointer"

echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bash tests/test-todo.sh`
Expected: FAIL — `Unknown command: todo`

- [ ] **Step 3: Add `cmd_todo` to `bin/agent`**

Add help function:

```bash
print_help_todo() {
  cat <<EOF
Usage: agent todo <add|list|check|uncheck> <name>|--latest [text|index]

Track merge requirements for an agent. Stored in container at
/workspace/.safe-agentic/todos.json.

Subcommands:
  add <text>       Add a todo item
  list             List all todos
  check <index>    Mark todo as done (1-based)
  uncheck <index>  Mark todo as not done

Examples:
  agent todo add api-refactor "Run tests"
  agent todo add api-refactor "Update docs"
  agent todo list api-refactor
  agent todo check api-refactor 1
  agent todo list --latest
EOF
}
```

Add command function:

```bash
cmd_todo() {
  local topic="todo"
  local subcmd=""
  local name=""
  local latest=false
  local text=""
  local index=""

  [ $# -gt 0 ] || die_with_help "$topic" "Subcommand required (add|list|check|uncheck)."

  subcmd="$1"; shift

  case "$subcmd" in
    -h|--help) print_help_todo; return 0 ;;
    add|list|check|uncheck) ;;
    *) die_with_help "$topic" "Unknown subcommand '$subcmd'." ;;
  esac

  while [ $# -gt 0 ]; do
    case "$1" in
      --latest)
        latest=true
        shift
        ;;
      -h|--help)
        print_help_todo
        return 0
        ;;
      *)
        if [ -z "$name" ] && ! $latest; then
          name="$1"
        elif [ "$subcmd" = "add" ] && [ -z "$text" ]; then
          text="$1"
        elif ([ "$subcmd" = "check" ] || [ "$subcmd" = "uncheck" ]) && [ -z "$index" ]; then
          index="$1"
        else
          die_with_help "$topic" "Unexpected argument '$1'."
        fi
        shift
        ;;
    esac
  done

  require_vm
  name=$(resolve_target_container "$topic" "$name" "$latest")

  local state
  state=$(vm_exec docker inspect --format '{{.State.Status}}' "$name" 2>/dev/null || echo "unknown")

  # Ensure state dir exists
  if [ "$state" = "running" ]; then
    vm_exec docker exec "$name" mkdir -p /workspace/.safe-agentic 2>/dev/null || true
  fi

  local todos_path="/workspace/.safe-agentic/todos.json"

  case "$subcmd" in
    add)
      [ -n "$text" ] || die_with_help "$topic" "Todo text required."
      local current
      current=$(vm_exec docker exec "$name" cat "$todos_path" 2>/dev/null || echo "[]")
      local updated
      updated=$(printf '%s' "$current" | python3 -c "
import json, sys
todos = json.load(sys.stdin)
todos.append({'text': '''$text''', 'done': False})
json.dump(todos, sys.stdout)
")
      printf '%s' "$updated" | vm_exec docker exec -i "$name" tee "$todos_path" >/dev/null
      ok "Added todo: $text"
      ;;
    list)
      local current
      current=$(vm_exec docker exec "$name" cat "$todos_path" 2>/dev/null || echo "[]")
      printf '%s' "$current" | python3 -c "
import json, sys
todos = json.load(sys.stdin)
if not todos:
    print('  (no todos)')
    sys.exit(0)
for i, t in enumerate(todos, 1):
    mark = 'x' if t.get('done') else ' '
    print(f'  [{mark}] {i}. {t[\"text\"]}')
done_count = sum(1 for t in todos if t.get('done'))
print(f'\n  {done_count}/{len(todos)} complete')
"
      ;;
    check|uncheck)
      [ -n "$index" ] || die_with_help "$topic" "Todo index required."
      local done_val="True"
      [ "$subcmd" = "uncheck" ] && done_val="False"
      local current
      current=$(vm_exec docker exec "$name" cat "$todos_path" 2>/dev/null || echo "[]")
      local updated
      updated=$(printf '%s' "$current" | python3 -c "
import json, sys
todos = json.load(sys.stdin)
idx = int('$index') - 1
if idx < 0 or idx >= len(todos):
    print(json.dumps(todos))
    sys.exit(1)
todos[idx]['done'] = $done_val
json.dump(todos, sys.stdout)
")
      printf '%s' "$updated" | vm_exec docker exec -i "$name" tee "$todos_path" >/dev/null
      ok "Todo $index ${subcmd}ed"
      ;;
  esac
}
```

Add to dispatch:

```bash
  todo)       cmd_todo "$@" ;;
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bash tests/test-todo.sh`
Expected: All pass

- [ ] **Step 5: Add todo list to TUI**

In `tui/actions.go`, add:

```go
// Todo shows the agent's todo list in an overlay.
func (ac *Actions) Todo() {
	agent := ac.selectedOrWarn()
	if agent == nil {
		return
	}
	data, err := execOrb("docker", "exec", agent.Name, "bash", "-c",
		`python3 -c "
import json, sys
try:
    with open('/workspace/.safe-agentic/todos.json') as f:
        todos = json.load(f)
except: todos = []
if not todos:
    print('(no todos)')
    sys.exit(0)
for i, t in enumerate(todos, 1):
    mark = 'x' if t.get('done') else ' '
    print(f'[{mark}] {i}. {t[\"text\"]}')
done_count = sum(1 for t in todos if t.get('done'))
print(f'\n{done_count}/{len(todos)} complete')
"`)
	if err != nil {
		ac.app.footer.ShowStatus("No todos found", false)
		return
	}
	ShowOverlay(ac.app, "todos", fmt.Sprintf("Todos: %s", agent.Name), string(data))
}
```

In `tui/app.go`, add keybinding:

```go
		case 't':
			ac.actions.Todo()
			return nil
```

In `tui/footer.go`, add shortcut:

```go
	{"t", "Todos"},
```

- [ ] **Step 6: Run all tests**

Run: `bash tests/run-all.sh`
Expected: All pass

- [ ] **Step 7: Commit**

```bash
git add bin/agent tests/test-todo.sh tui/actions.go tui/app.go tui/footer.go
git commit -m "feat: add agent todo for tracking merge requirements

JSON-based todo list stored in container. Supports add/list/check/uncheck.
Available as CLI command and 't' keybinding in TUI."
```

---

## Phase 2: Competitive Parity

### Task 5: `agent pr` — Create PR from Agent's Branch

**Files:**
- Modify: `bin/agent` (add `cmd_pr` function + dispatch)
- Create: `tests/test-pr.sh`
- Modify: `tui/actions.go` (add `CreatePR()` method)
- Modify: `tui/app.go` (wire `g` keybinding)

- [ ] **Step 1: Write the test**

Create `tests/test-pr.sh`:

```bash
#!/usr/bin/env bash
# Tests for agent pr command.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"
ERR_LOG="$TMP_DIR/error.log"
OUT_LOG="$TMP_DIR/out.log"
VERIFY_STATE="$TMP_DIR/verify-state"
mkdir -p "$FAKE_BIN"

cat >"$FAKE_BIN/orb" <<'ORBEOF'
#!/usr/bin/env bash
set -euo pipefail
log_file="${TEST_ORB_LOG:?}"
verify_state="${TEST_VERIFY_STATE:?}"
cmd="${1:-}"; shift || true
case "$cmd" in
  list) echo "safe-agentic" ;;
  run)
    [ "${1:-}" = "-m" ] && shift 2
    printf '%s\n' "$*" >>"$log_file"
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ]; then
      case "${4:-}" in
        *State.Status*) echo "running" ;;
        *agent-type*)   echo "claude" ;;
        *ssh*)          echo "yes" ;;
        *) echo "" ;;
      esac
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "ps" ]; then
      echo "agent-claude-test-1234"
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "exec" ]; then
      if [[ "$*" == *"git push"* ]]; then
        echo "pushed"
        exit 0
      fi
      if [[ "$*" == *"gh pr create"* ]]; then
        echo "https://github.com/test/repo/pull/1"
        exit 0
      fi
      if [[ "$*" == *"git rev-parse"* ]]; then
        echo "feature-branch"
        exit 0
      fi
      exit 0
    fi
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-lc" ] && [[ "${3:-}" == *safe-agentic-hardening-verify* ]]; then
      : >"$verify_state"
      exit 0
    fi
    exit 0 ;;
  push|start|stop|create|ssh) ;;
  *) echo "unexpected orb command: $cmd" >&2; exit 1 ;;
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

pass=0
fail=0

run_ok() {
  local label="$1"; shift
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
    ((++pass))
  else
    echo "FAIL: $label: expected zero exit" >&2
    ((++fail))
  fi
}

run_fails() {
  local label="$1"; shift
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
    echo "FAIL: $label: expected non-zero exit" >&2
    ((++fail))
  else
    ((++pass))
  fi
}

assert_output_contains() {
  local needle="$1" label="$2"
  if grep -q "$needle" "$OUT_LOG" 2>/dev/null || grep -q "$needle" "$ERR_LOG" 2>/dev/null; then
    ((++pass))
  else
    echo "FAIL: $label: output should contain '$needle'" >&2
    ((++fail))
  fi
}

# --- Tests ---

run_ok "pr help" bash "$REPO_DIR/bin/agent" pr --help
assert_output_contains "agent pr" "pr help mentions command"

run_fails "pr no args" bash "$REPO_DIR/bin/agent" pr
assert_output_contains "agent help pr" "pr usage pointer"

echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bash tests/test-pr.sh`
Expected: FAIL — `Unknown command: pr`

- [ ] **Step 3: Add `cmd_pr` to `bin/agent`**

```bash
print_help_pr() {
  cat <<EOF
Usage: agent pr <name>|--latest [--title TITLE] [--base BRANCH]

Create a GitHub pull request from the agent's current branch.
Requires SSH enabled (for push) and gh CLI authenticated inside the container.

Options:
  --title TITLE    PR title (default: branch name)
  --base BRANCH    Base branch (default: main)

Examples:
  agent pr api-refactor
  agent pr --latest --title "feat: add caching" --base dev
EOF
}

cmd_pr() {
  local topic="pr"
  local name=""
  local latest=false
  local title=""
  local base="main"

  while [ $# -gt 0 ]; do
    case "$1" in
      --latest)
        latest=true
        shift
        ;;
      --title)
        require_option_value "$topic" "--title" "${2:-}"
        title="$2"
        shift 2
        ;;
      --base)
        require_option_value "$topic" "--base" "${2:-}"
        base="$2"
        shift 2
        ;;
      -h|--help)
        print_help_pr
        return 0
        ;;
      *)
        if [ -z "$name" ] && ! $latest; then
          name="$1"
        else
          die_with_help "$topic" "Unexpected argument '$1'."
        fi
        shift
        ;;
    esac
  done

  require_vm
  name=$(resolve_target_container "$topic" "$name" "$latest")

  local state
  state=$(vm_exec docker inspect --format '{{.State.Status}}' "$name" 2>/dev/null || echo "unknown")
  [ "$state" = "running" ] || die "Container $name is not running (state: $state)."

  # Check SSH is enabled (needed for push)
  local ssh_label
  ssh_label=$(vm_exec docker inspect --format '{{index .Config.Labels "safe-agentic.ssh"}}' "$name" 2>/dev/null || echo "")
  [ "$ssh_label" = "yes" ] || die "SSH not enabled on $name. PR creation requires --ssh for git push."

  # Check for uncommitted todos
  local todos
  todos=$(vm_exec docker exec "$name" cat /workspace/.safe-agentic/todos.json 2>/dev/null || echo "[]")
  local incomplete
  incomplete=$(printf '%s' "$todos" | python3 -c "
import json, sys
todos = json.load(sys.stdin)
incomplete = [t for t in todos if not t.get('done')]
print(len(incomplete))
" 2>/dev/null || echo "0")

  if [ "$incomplete" != "0" ] && [ "$incomplete" != "" ]; then
    warn "$incomplete incomplete todo(s). Complete them first or use --force."
    vm_exec docker exec "$name" bash -c "python3 -c \"
import json, sys
with open('/workspace/.safe-agentic/todos.json') as f:
    todos = json.load(f)
for i, t in enumerate(todos, 1):
    mark = 'x' if t.get('done') else ' '
    print(f'  [{mark}] {i}. {t[\\\"text\\\"]}')
\"" 2>/dev/null || true
    return 1
  fi

  # Get branch name
  local branch
  branch=$(vm_exec docker exec "$name" bash -c 'cd /workspace/* 2>/dev/null || cd /workspace; git rev-parse --abbrev-ref HEAD' 2>/dev/null)
  [ -n "$branch" ] || die "Could not determine branch name."
  [ "$branch" != "main" ] && [ "$branch" != "master" ] || die "Cannot create PR from $branch."

  [ -z "$title" ] && title="$branch"

  info "Committing and pushing changes on branch $branch..."
  vm_exec docker exec "$name" bash -c "cd /workspace/* 2>/dev/null || cd /workspace; git add -A && git diff --cached --quiet || git commit -m 'agent work: $title'" 2>/dev/null || true
  vm_exec docker exec "$name" bash -c "cd /workspace/* 2>/dev/null || cd /workspace; git push -u origin $branch" 2>/dev/null || die "Push failed. Check SSH and remote config."

  info "Creating PR: $title (base: $base)..."
  local pr_url
  pr_url=$(vm_exec docker exec "$name" bash -c "cd /workspace/* 2>/dev/null || cd /workspace; gh pr create --title '$title' --body 'Created by safe-agentic agent: $name' --base '$base'" 2>/dev/null)

  if [ -n "$pr_url" ]; then
    ok "PR created: $pr_url"
  else
    die "PR creation failed."
  fi
}
```

Add to dispatch:

```bash
  pr)         cmd_pr "$@" ;;
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bash tests/test-pr.sh`
Expected: All pass

- [ ] **Step 5: Add PR creation to TUI**

In `tui/actions.go`, add:

```go
// CreatePR creates a GitHub PR from the agent's current branch.
func (ac *Actions) CreatePR() {
	agent := ac.selectedOrWarn()
	if agent == nil {
		return
	}
	if !agent.Running {
		ac.app.footer.ShowStatus("Agent not running", true)
		return
	}
	if agent.SSH != "yes" {
		ac.app.footer.ShowStatus("SSH required for PR creation", true)
		return
	}
	name := agent.Name
	ac.app.footer.ShowConfirm("Create PR from "+name+"?", func(yes bool) {
		if !yes {
			return
		}
		ac.app.footer.ShowStatus("Creating PR...", false)
		go func() {
			cmd := exec.Command("agent", "pr", name)
			out, err := cmd.CombinedOutput()
			ac.app.tapp.QueueUpdateDraw(func() {
				output := strings.TrimSpace(string(out))
				if err != nil {
					ac.app.footer.ShowStatus("PR failed: "+output, true)
				} else {
					ac.app.footer.ShowStatus(output, false)
				}
			})
		}()
	})
}
```

In `tui/app.go`, add keybinding:

```go
		case 'g':
			ac.actions.CreatePR()
			return nil
```

In `tui/footer.go`, add shortcut:

```go
	{"g", "PR"},
```

- [ ] **Step 6: Commit**

```bash
git add bin/agent tests/test-pr.sh tui/actions.go tui/app.go tui/footer.go
git commit -m "feat: add agent pr for creating GitHub PRs from agents

Pushes agent's branch and creates PR via gh CLI inside container.
Blocks if incomplete todos exist. Available as 'g' in TUI."
```

---

### Task 6: `agent review` — AI Code Review of Agent's Changes

**Files:**
- Modify: `bin/agent` (add `cmd_review` function + dispatch)
- Create: `tests/test-review.sh`

- [ ] **Step 1: Write the test**

Create `tests/test-review.sh`:

```bash
#!/usr/bin/env bash
# Tests for agent review command.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"
ERR_LOG="$TMP_DIR/error.log"
OUT_LOG="$TMP_DIR/out.log"
VERIFY_STATE="$TMP_DIR/verify-state"
mkdir -p "$FAKE_BIN"

cat >"$FAKE_BIN/orb" <<'ORBEOF'
#!/usr/bin/env bash
set -euo pipefail
log_file="${TEST_ORB_LOG:?}"
verify_state="${TEST_VERIFY_STATE:?}"
cmd="${1:-}"; shift || true
case "$cmd" in
  list) echo "safe-agentic" ;;
  run)
    [ "${1:-}" = "-m" ] && shift 2
    printf '%s\n' "$*" >>"$log_file"
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ]; then
      case "${4:-}" in
        *State.Status*) echo "running" ;;
        *agent-type*)   echo "claude" ;;
        *) echo "" ;;
      esac
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "ps" ]; then
      echo "agent-claude-test-1234"; exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "exec" ]; then
      if [[ "$*" == *"codex review"* ]] || [[ "$*" == *"claude"*"review"* ]]; then
        echo "Review complete: 0 critical, 1 medium"
        exit 0
      fi
      if [[ "$*" == *"git diff"* ]]; then
        echo "diff --git a/main.py b/main.py"
        exit 0
      fi
      exit 0
    fi
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-lc" ] && [[ "${3:-}" == *safe-agentic-hardening-verify* ]]; then
      : >"$verify_state"; exit 0
    fi
    exit 0 ;;
  push|start|stop|create|ssh) ;;
  *) echo "unexpected orb command: $cmd" >&2; exit 1 ;;
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

pass=0
fail=0

run_ok() {
  local label="$1"; shift
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
    ((++pass))
  else
    echo "FAIL: $label: expected zero exit" >&2; ((++fail))
  fi
}

run_fails() {
  local label="$1"; shift
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
    echo "FAIL: $label: expected non-zero exit" >&2; ((++fail))
  else
    ((++pass))
  fi
}

assert_output_contains() {
  local needle="$1" label="$2"
  if grep -q "$needle" "$OUT_LOG" 2>/dev/null || grep -q "$needle" "$ERR_LOG" 2>/dev/null; then
    ((++pass))
  else
    echo "FAIL: $label: output should contain '$needle'" >&2; ((++fail))
  fi
}

# --- Tests ---

run_ok "review help" bash "$REPO_DIR/bin/agent" review --help
assert_output_contains "agent review" "review help mentions command"

run_fails "review no args" bash "$REPO_DIR/bin/agent" review
assert_output_contains "agent help review" "review usage pointer"

echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bash tests/test-review.sh`
Expected: FAIL — `Unknown command: review`

- [ ] **Step 3: Add `cmd_review` to `bin/agent`**

```bash
print_help_review() {
  cat <<EOF
Usage: agent review <name>|--latest [--base BRANCH]

Run an AI code review of the agent's uncommitted changes (or diff against base).
Uses codex review if available, otherwise outputs diff for external review.

Options:
  --base BRANCH    Compare against branch (default: shows uncommitted changes)

Examples:
  agent review api-refactor
  agent review --latest --base main
EOF
}

cmd_review() {
  local topic="review"
  local name=""
  local latest=false
  local base=""

  while [ $# -gt 0 ]; do
    case "$1" in
      --latest)
        latest=true
        shift
        ;;
      --base)
        require_option_value "$topic" "--base" "${2:-}"
        base="$2"
        shift 2
        ;;
      -h|--help)
        print_help_review
        return 0
        ;;
      *)
        if [ -z "$name" ] && ! $latest; then
          name="$1"
        else
          die_with_help "$topic" "Unexpected argument '$1'."
        fi
        shift
        ;;
    esac
  done

  require_vm
  name=$(resolve_target_container "$topic" "$name" "$latest")

  local state
  state=$(vm_exec docker inspect --format '{{.State.Status}}' "$name" 2>/dev/null || echo "unknown")
  [ "$state" = "running" ] || die "Container $name is not running (state: $state)."

  # Check if codex is available in the container
  local has_codex
  has_codex=$(vm_exec docker exec "$name" command -v codex 2>/dev/null && echo "yes" || echo "no")

  if [ "$has_codex" = "yes" ] && [ -n "$base" ]; then
    info "Running codex review (base: $base)..."
    vm_exec docker exec "$name" bash -c "cd /workspace/* 2>/dev/null || cd /workspace; codex review --base $base" 2>/dev/null
  elif [ "$has_codex" = "yes" ]; then
    info "Running codex review (uncommitted changes)..."
    vm_exec docker exec "$name" bash -c "cd /workspace/* 2>/dev/null || cd /workspace; codex review --uncommitted" 2>/dev/null
  else
    info "codex not available, showing raw diff..."
    if [ -n "$base" ]; then
      vm_exec docker exec "$name" bash -c "cd /workspace/* 2>/dev/null || cd /workspace; git diff $base" 2>/dev/null
    else
      vm_exec docker exec "$name" bash -c "cd /workspace/* 2>/dev/null || cd /workspace; git diff" 2>/dev/null
    fi
  fi
}
```

Add to dispatch:

```bash
  review)     cmd_review "$@" ;;
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bash tests/test-review.sh`
Expected: All pass

- [ ] **Step 5: Commit**

```bash
git add bin/agent tests/test-review.sh
git commit -m "feat: add agent review for AI code review of agent changes

Runs codex review inside the container if available, falls back to
raw git diff. Supports --base for branch comparison."
```

---

### Task 7: `agent audit` — Append-Only Operation Log

**Files:**
- Modify: `bin/agent-lib.sh` (add `audit_log` function)
- Modify: `bin/agent` (instrument spawn/stop/attach with audit calls + add `cmd_audit`)
- Create: `tests/test-audit.sh`

- [ ] **Step 1: Write the test**

Create `tests/test-audit.sh`:

```bash
#!/usr/bin/env bash
# Tests for audit log infrastructure.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

pass=0
fail=0

assert_eq() {
  local actual="$1" expected="$2" label="$3"
  if [ "$actual" = "$expected" ]; then
    ((++pass))
  else
    echo "FAIL: $label: expected '$expected', got '$actual'" >&2
    ((++fail))
  fi
}

assert_contains() {
  local haystack="$1" needle="$2" label="$3"
  if [[ "$haystack" == *"$needle"* ]]; then
    ((++pass))
  else
    echo "FAIL: $label: should contain '$needle'" >&2
    ((++fail))
  fi
}

# Test: audit_log function writes JSONL
export SAFE_AGENTIC_AUDIT_LOG="$TMP_DIR/audit.jsonl"

# Source the lib to get the function
source "$REPO_DIR/bin/agent-lib.sh"

# Test that audit_log writes valid JSONL
audit_log "spawn" "agent-claude-test" "type=claude repo=test ssh=yes"
audit_log "stop" "agent-claude-test" ""
audit_log "attach" "agent-claude-test" ""

# Verify file exists and has 3 lines
line_count=$(wc -l < "$SAFE_AGENTIC_AUDIT_LOG" | tr -d ' ')
assert_eq "$line_count" "3" "audit log has 3 entries"

# Verify JSON structure
first_line=$(head -1 "$SAFE_AGENTIC_AUDIT_LOG")
assert_contains "$first_line" '"action":"spawn"' "first entry has action=spawn"
assert_contains "$first_line" '"container":"agent-claude-test"' "first entry has container name"
assert_contains "$first_line" '"timestamp"' "first entry has timestamp"
assert_contains "$first_line" '"details":"type=claude repo=test ssh=yes"' "first entry has details"

echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bash tests/test-audit.sh`
Expected: FAIL — `audit_log` function not found

- [ ] **Step 3: Add `audit_log` to `bin/agent-lib.sh`**

Add at the end of `bin/agent-lib.sh`:

```bash
# ---------------------------------------------------------------------------
# Audit logging — append-only JSONL
# ---------------------------------------------------------------------------
AUDIT_LOG_FILE="${SAFE_AGENTIC_AUDIT_LOG:-${DEFAULTS_DIR}/audit.jsonl}"

audit_log() {
  local action="$1"
  local container="${2:-}"
  local details="${3:-}"
  local timestamp
  timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

  mkdir -p "$(dirname "$AUDIT_LOG_FILE")" 2>/dev/null || return 0

  printf '{"timestamp":"%s","action":"%s","container":"%s","details":"%s"}\n' \
    "$timestamp" "$action" "$container" "$details" \
    >>"$AUDIT_LOG_FILE" 2>/dev/null || true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bash tests/test-audit.sh`
Expected: All pass

- [ ] **Step 5: Add `cmd_audit` to `bin/agent`**

```bash
print_help_audit() {
  cat <<EOF
Usage: agent audit [--lines N]

Show the audit log of agent operations.

Options:
  --lines N    Show last N entries (default: 50)

Examples:
  agent audit
  agent audit --lines 100
EOF
}

cmd_audit() {
  local lines=50

  while [ $# -gt 0 ]; do
    case "$1" in
      --lines)
        require_option_value "audit" "--lines" "${2:-}"
        lines="$2"
        shift 2
        ;;
      -h|--help)
        print_help_audit
        return 0
        ;;
      *)
        die_with_help "audit" "Unexpected argument '$1'."
        ;;
    esac
  done

  local log_file="${SAFE_AGENTIC_AUDIT_LOG:-${DEFAULTS_DIR}/audit.jsonl}"
  if [ ! -f "$log_file" ]; then
    info "No audit log yet."
    return 0
  fi

  tail -"$lines" "$log_file" | python3 -c "
import json, sys
for line in sys.stdin:
    line = line.strip()
    if not line: continue
    try:
        e = json.loads(line)
        ts = e.get('timestamp', '?')[:19]
        action = e.get('action', '?')
        container = e.get('container', '')
        details = e.get('details', '')
        print(f'  {ts}  {action:<10}  {container:<30}  {details}')
    except: pass
" 2>/dev/null
}
```

Add to dispatch:

```bash
  audit)      cmd_audit "$@" ;;
```

- [ ] **Step 6: Instrument spawn, stop, and attach with audit calls**

In `cmd_spawn`, add after the `run_container_detached` / `run_container` calls:

```bash
  audit_log "spawn" "$container_name" "type=$agent_type ssh=$enable_ssh auth=$([ "$reuse_auth" = true ] && echo shared || echo ephemeral) network=$network_mode_label"
```

In `cmd_stop`, add after the `vm_exec docker stop` call:

```bash
  audit_log "stop" "$name" ""
```

In `cmd_attach`, add at the start of the attach logic:

```bash
  audit_log "attach" "$name" ""
```

- [ ] **Step 7: Run all tests**

Run: `bash tests/run-all.sh`
Expected: All pass

- [ ] **Step 8: Commit**

```bash
git add bin/agent bin/agent-lib.sh tests/test-audit.sh
git commit -m "feat: add audit log for all agent operations

Append-only JSONL log at ~/.config/safe-agentic/audit.jsonl.
Logs spawn/stop/attach with timestamp, container name, and details.
'agent audit' displays formatted log entries."
```

---

### Task 8: `agent cost` — Session Cost Estimation

**Files:**
- Modify: `bin/agent` (add `cmd_cost` function + dispatch)
- Create: `tests/test-cost.sh`

- [ ] **Step 1: Write the test**

Create `tests/test-cost.sh`:

```bash
#!/usr/bin/env bash
# Tests for agent cost command.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"
ERR_LOG="$TMP_DIR/error.log"
OUT_LOG="$TMP_DIR/out.log"
VERIFY_STATE="$TMP_DIR/verify-state"
mkdir -p "$FAKE_BIN"

cat >"$FAKE_BIN/orb" <<'ORBEOF'
#!/usr/bin/env bash
set -euo pipefail
log_file="${TEST_ORB_LOG:?}"
verify_state="${TEST_VERIFY_STATE:?}"
cmd="${1:-}"; shift || true
case "$cmd" in
  list) echo "safe-agentic" ;;
  run)
    [ "${1:-}" = "-m" ] && shift 2
    printf '%s\n' "$*" >>"$log_file"
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "inspect" ] && [ "${3:-}" = "--format" ]; then
      case "${4:-}" in
        *State.Status*) echo "running" ;;
        *agent-type*)   echo "claude" ;;
        *) echo "" ;;
      esac
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "ps" ]; then
      echo "agent-claude-test-1234"; exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "exec" ]; then
      exit 0
    fi
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-lc" ] && [[ "${3:-}" == *safe-agentic-hardening-verify* ]]; then
      : >"$verify_state"; exit 0
    fi
    exit 0 ;;
  push|start|stop|create|ssh) ;;
  *) echo "unexpected orb command: $cmd" >&2; exit 1 ;;
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

pass=0
fail=0

run_ok() {
  local label="$1"; shift
  if PATH="$FAKE_BIN:$PATH" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
    ((++pass))
  else
    echo "FAIL: $label: expected zero exit" >&2; ((++fail))
  fi
}

assert_output_contains() {
  local needle="$1" label="$2"
  if grep -q "$needle" "$OUT_LOG" 2>/dev/null || grep -q "$needle" "$ERR_LOG" 2>/dev/null; then
    ((++pass))
  else
    echo "FAIL: $label: output should contain '$needle'" >&2; ((++fail))
  fi
}

# --- Tests ---

run_ok "cost help" bash "$REPO_DIR/bin/agent" cost --help
assert_output_contains "agent cost" "cost help mentions command"

echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bash tests/test-cost.sh`
Expected: FAIL — `Unknown command: cost`

- [ ] **Step 3: Add `cmd_cost` to `bin/agent`**

```bash
print_help_cost() {
  cat <<EOF
Usage: agent cost <name>|--latest
       agent cost --all

Estimate API cost by parsing session JSONL files for token usage.

Examples:
  agent cost api-refactor
  agent cost --latest
EOF
}

cmd_cost() {
  local topic="cost"
  local name=""
  local latest=false

  while [ $# -gt 0 ]; do
    case "$1" in
      --latest)
        latest=true
        shift
        ;;
      -h|--help)
        print_help_cost
        return 0
        ;;
      *)
        if [ -z "$name" ] && ! $latest; then
          name="$1"
        else
          die_with_help "$topic" "Unexpected argument '$1'."
        fi
        shift
        ;;
    esac
  done

  require_vm
  name=$(resolve_target_container "$topic" "$name" "$latest")

  local agent_type config_dir
  agent_type=$(vm_exec docker inspect --format '{{index .Config.Labels "safe-agentic.agent-type"}}' "$name" 2>/dev/null || echo "")
  case "$agent_type" in
    codex) config_dir="/home/agent/.codex" ;;
    claude) config_dir="/home/agent/.claude" ;;
    *) config_dir="/home/agent/.codex" ;;
  esac

  info "Analyzing sessions from $name ($agent_type)..."

  vm_exec docker exec "$name" bash -c "
    find $config_dir/sessions/ -name '*.jsonl' ! -name '._*' -type f 2>/dev/null | while read -r f; do
      cat \"\$f\"
    done
  " 2>/dev/null | python3 -c "
import json, sys

total_input = 0
total_output = 0
model_usage = {}

for line in sys.stdin:
    line = line.strip()
    if not line: continue
    try:
        entry = json.loads(line)
        usage = entry.get('usage', {})
        if not usage:
            payload = entry.get('payload', {})
            if isinstance(payload, dict):
                usage = payload.get('usage', {})
        if usage:
            inp = usage.get('input_tokens', 0) or 0
            out = usage.get('output_tokens', 0) or 0
            total_input += inp
            total_output += out
            model = entry.get('model', payload.get('model', 'unknown') if isinstance(payload, dict) else 'unknown')
            if model not in model_usage:
                model_usage[model] = {'input': 0, 'output': 0}
            model_usage[model]['input'] += inp
            model_usage[model]['output'] += out
    except:
        pass

# Cost estimates (per 1M tokens, approximate)
COSTS = {
    'claude-opus-4-6': {'input': 15.0, 'output': 75.0},
    'claude-sonnet-4-6': {'input': 3.0, 'output': 15.0},
    'claude-haiku-4-5': {'input': 0.80, 'output': 4.0},
    'gpt-5.2': {'input': 5.0, 'output': 15.0},
    'default': {'input': 3.0, 'output': 15.0},
}

total_cost = 0.0
print(f'  Total tokens: {total_input + total_output:,}')
print(f'    Input:  {total_input:,}')
print(f'    Output: {total_output:,}')
print()
for model, u in sorted(model_usage.items()):
    rates = COSTS.get(model, COSTS['default'])
    cost = (u['input'] / 1_000_000 * rates['input']) + (u['output'] / 1_000_000 * rates['output'])
    total_cost += cost
    print(f'  {model}: {u[\"input\"]+u[\"output\"]:,} tokens (~\${cost:.2f})')
print(f'\n  Estimated total: ~\${total_cost:.2f}')
" 2>/dev/null
}
```

Add to dispatch:

```bash
  cost)       cmd_cost "$@" ;;
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bash tests/test-cost.sh`
Expected: All pass

- [ ] **Step 5: Commit**

```bash
git add bin/agent tests/test-cost.sh
git commit -m "feat: add agent cost for API spend estimation

Parses session JSONL for token usage, estimates cost per model.
Shows per-model breakdown and total estimated spend."
```

---

## Phase 3: Differentiation

### Task 9: `agent fleet` — Spawn from Manifest

**Files:**
- Modify: `bin/agent` (add `cmd_fleet` function + dispatch)
- Create: `tests/test-fleet.sh`

- [ ] **Step 1: Write the test**

Create `tests/test-fleet.sh`:

```bash
#!/usr/bin/env bash
# Tests for agent fleet command.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

pass=0
fail=0

# Test: validate fleet manifest parsing
cat >"$TMP_DIR/fleet.yaml" <<'EOF'
agents:
  - name: api-worker
    type: claude
    repo: git@github.com:org/api.git
    ssh: true
    reuse_auth: true
  - name: frontend-worker
    type: codex
    repo: https://github.com/org/frontend.git
EOF

# Parse with python3 (yaml via json conversion or direct)
count=$(python3 -c "
import json, sys
# Simple YAML-like parser for the subset we support
agents = []
current = None
with open('$TMP_DIR/fleet.yaml') as f:
    for line in f:
        line = line.rstrip()
        if line.strip().startswith('- name:'):
            if current: agents.append(current)
            current = {'name': line.split(':', 1)[1].strip()}
        elif current and ':' in line and line.startswith('    '):
            k, v = line.strip().split(':', 1)
            v = v.strip()
            if v in ('true', 'True'): v = True
            elif v in ('false', 'False'): v = False
            current[k.strip()] = v
if current: agents.append(current)
print(len(agents))
" 2>/dev/null)

if [ "$count" = "2" ]; then
  ((++pass))
else
  echo "FAIL: fleet manifest should parse 2 agents, got '$count'" >&2
  ((++fail))
fi

echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
```

- [ ] **Step 2: Run test to verify it passes**

Run: `bash tests/test-fleet.sh`
Expected: Pass (tests parsing only)

- [ ] **Step 3: Add `cmd_fleet` to `bin/agent`**

```bash
print_help_fleet() {
  cat <<EOF
Usage: agent fleet <manifest.yaml> [--dry-run]

Spawn multiple agents from a YAML manifest file.

Manifest format:
  agents:
    - name: api-worker
      type: claude
      repo: git@github.com:org/api.git
      ssh: true
      reuse_auth: true
      prompt: "Fix the failing tests"
    - name: frontend-worker
      type: codex
      repo: https://github.com/org/frontend.git

Options:
  --dry-run    Show what would be spawned without running

Examples:
  agent fleet fleet.yaml
  agent fleet fleet.yaml --dry-run
EOF
}

cmd_fleet() {
  local topic="fleet"
  local manifest=""
  local dry_run=false

  while [ $# -gt 0 ]; do
    case "$1" in
      --dry-run)
        dry_run=true
        shift
        ;;
      -h|--help)
        print_help_fleet
        return 0
        ;;
      *)
        if [ -z "$manifest" ]; then
          manifest="$1"
        else
          die_with_help "$topic" "Unexpected argument '$1'."
        fi
        shift
        ;;
    esac
  done

  [ -n "$manifest" ] || die_with_help "$topic" "Manifest file required."
  [ -f "$manifest" ] || die "Manifest not found: $manifest"

  info "Parsing fleet manifest: $manifest"

  local agents_json
  agents_json=$(python3 -c "
import json, sys

agents = []
current = None
with open('$manifest') as f:
    for line in f:
        line = line.rstrip()
        if line.strip().startswith('- name:'):
            if current: agents.append(current)
            current = {'name': line.split(':', 1)[1].strip()}
        elif current and ':' in line and line.startswith('    '):
            k, v = line.strip().split(':', 1)
            v = v.strip()
            if v in ('true', 'True'): v = True
            elif v in ('false', 'False'): v = False
            current[k.strip()] = v
if current: agents.append(current)
json.dump(agents, sys.stdout)
" 2>/dev/null) || die "Failed to parse manifest."

  local count
  count=$(printf '%s' "$agents_json" | python3 -c "import json,sys; print(len(json.load(sys.stdin)))")
  info "Found $count agent(s) in manifest"

  printf '%s' "$agents_json" | python3 -c "
import json, sys, subprocess

agents = json.load(sys.stdin)
dry_run = '$dry_run' == 'true'

for a in agents:
    args = ['agent', 'spawn', a.get('type', 'claude')]
    if a.get('repo'): args += ['--repo', a['repo']]
    if a.get('name'): args += ['--name', a['name']]
    if a.get('ssh'): args += ['--ssh']
    if a.get('reuse_auth'): args += ['--reuse-auth']
    if a.get('reuse_gh_auth'): args += ['--reuse-gh-auth']
    if a.get('docker'): args += ['--docker']
    if a.get('prompt'): args += ['--prompt', a['prompt']]
    if a.get('aws'): args += ['--aws', a['aws']]
    if a.get('network'): args += ['--network', a['network']]
    if a.get('memory'): args += ['--memory', a['memory']]
    if a.get('cpus'): args += ['--cpus', str(a['cpus'])]
    if dry_run:
        args += ['--dry-run']
    print(f'  > {\" \".join(args)}')
    if not dry_run:
        subprocess.Popen(args)
" 2>/dev/null

  ok "Fleet $( $dry_run && echo 'dry run' || echo 'spawn' ) complete ($count agents)"
}
```

Add to dispatch:

```bash
  fleet)      cmd_fleet "$@" ;;
```

- [ ] **Step 4: Run all tests**

Run: `bash tests/run-all.sh`
Expected: All pass

- [ ] **Step 5: Commit**

```bash
git add bin/agent tests/test-fleet.sh
git commit -m "feat: add agent fleet for spawning from YAML manifests

Parse a fleet.yaml manifest and spawn multiple agents in parallel.
Supports --dry-run to preview. All spawn flags supported per agent."
```

---

### Task 10: `agent pipeline` — Multi-Agent Workflow Orchestration

**Files:**
- Modify: `bin/agent` (add `cmd_pipeline` function + dispatch)
- Create: `tests/test-pipeline.sh`

- [ ] **Step 1: Write the test**

Create `tests/test-pipeline.sh`:

```bash
#!/usr/bin/env bash
# Tests for agent pipeline command.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

pass=0
fail=0

# Test: validate pipeline manifest parsing
cat >"$TMP_DIR/pipeline.yaml" <<'EOF'
name: test-and-fix
steps:
  - name: run-tests
    type: claude
    repo: git@github.com:org/api.git
    prompt: "Run all tests and report results"
    on_failure: fix-tests
  - name: fix-tests
    type: claude
    repo: git@github.com:org/api.git
    prompt: "Fix the failing tests"
    retry: 2
  - name: create-pr
    type: claude
    repo: git@github.com:org/api.git
    prompt: "Create a PR with the fixes"
    depends_on: fix-tests
EOF

count=$(python3 -c "
agents = []
current = None
with open('$TMP_DIR/pipeline.yaml') as f:
    for line in f:
        line = line.rstrip()
        if line.strip().startswith('- name:'):
            if current: agents.append(current)
            current = {'name': line.split(':', 1)[1].strip()}
        elif current and ':' in line and (line.startswith('    ') or line.startswith('      ')):
            k, v = line.strip().split(':', 1)
            v = v.strip()
            current[k.strip()] = v
if current: agents.append(current)
print(len(agents))
" 2>/dev/null)

if [ "$count" = "3" ]; then
  ((++pass))
else
  echo "FAIL: pipeline should parse 3 steps, got '$count'" >&2
  ((++fail))
fi

echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
```

- [ ] **Step 2: Run test to verify it passes**

Run: `bash tests/test-pipeline.sh`
Expected: Pass

- [ ] **Step 3: Add `cmd_pipeline` to `bin/agent`**

```bash
print_help_pipeline() {
  cat <<EOF
Usage: agent pipeline <pipeline.yaml> [--dry-run]

Run a multi-step agent pipeline from a YAML definition.
Steps execute sequentially. Failed steps trigger on_failure handlers.
Steps with retry will be re-attempted with the specified count.

Pipeline format:
  name: test-and-fix
  steps:
    - name: run-tests
      type: claude
      repo: git@github.com:org/api.git
      prompt: "Run all tests"
      on_failure: fix-tests       # jump to this step on failure
    - name: fix-tests
      type: claude
      repo: git@github.com:org/api.git
      prompt: "Fix failing tests"
      retry: 2                     # retry up to 2 times
    - name: create-pr
      type: claude
      repo: git@github.com:org/api.git
      prompt: "Create a PR"
      depends_on: fix-tests        # only run after this step succeeds

Options:
  --dry-run    Show execution plan without running

Examples:
  agent pipeline pipeline.yaml
  agent pipeline pipeline.yaml --dry-run
EOF
}

cmd_pipeline() {
  local topic="pipeline"
  local manifest=""
  local dry_run=false

  while [ $# -gt 0 ]; do
    case "$1" in
      --dry-run)
        dry_run=true
        shift
        ;;
      -h|--help)
        print_help_pipeline
        return 0
        ;;
      *)
        if [ -z "$manifest" ]; then
          manifest="$1"
        else
          die_with_help "$topic" "Unexpected argument '$1'."
        fi
        shift
        ;;
    esac
  done

  [ -n "$manifest" ] || die_with_help "$topic" "Pipeline file required."
  [ -f "$manifest" ] || die "Pipeline not found: $manifest"

  info "Parsing pipeline: $manifest"

  python3 -c "
import json, sys, subprocess, time

# Parse simple YAML
steps = []
current = None
pipeline_name = ''
with open('$manifest') as f:
    for line in f:
        line = line.rstrip()
        if line.startswith('name:'):
            pipeline_name = line.split(':', 1)[1].strip()
        if line.strip().startswith('- name:'):
            if current: steps.append(current)
            current = {'name': line.split(':', 1)[1].strip()}
        elif current and ':' in line and (line.startswith('    ') or line.startswith('      ')):
            k, v = line.strip().split(':', 1)
            v = v.strip()
            try: v = int(v)
            except: pass
            current[k.strip()] = v
if current: steps.append(current)

dry_run = '$dry_run' == 'true'
print(f'Pipeline: {pipeline_name} ({len(steps)} steps)')

completed = set()
failed = set()

def run_step(step, attempt=1):
    name = step['name']
    retry = int(step.get('retry', 0))
    depends = step.get('depends_on', '')

    if depends and depends not in completed:
        print(f'  SKIP {name} (depends on {depends} which has not completed)')
        return False

    args = ['agent', 'spawn', step.get('type', 'claude')]
    if step.get('repo'): args += ['--repo', step['repo']]
    args += ['--name', f'pipeline-{name}']
    if step.get('ssh', '').lower() in ('true', 'yes', '1'): args += ['--ssh']
    if step.get('reuse_auth', '').lower() in ('true', 'yes', '1'): args += ['--reuse-auth']
    if step.get('prompt'): args += ['--prompt', step['prompt']]

    print(f'  RUN  {name} (attempt {attempt}/{retry+1})')
    if dry_run:
        print(f'       {\" \".join(args)}')
        completed.add(name)
        return True

    result = subprocess.run(args, capture_output=True, text=True)
    if result.returncode == 0:
        completed.add(name)
        print(f'  DONE {name}')
        return True
    else:
        if attempt <= retry:
            print(f'  RETRY {name} (attempt {attempt+1})')
            time.sleep(5)
            return run_step(step, attempt + 1)
        failed.add(name)
        print(f'  FAIL {name}')
        on_fail = step.get('on_failure', '')
        if on_fail:
            fail_step = next((s for s in steps if s['name'] == on_fail), None)
            if fail_step:
                print(f'  -> Triggering failure handler: {on_fail}')
                run_step(fail_step)
        return False

for step in steps:
    if step['name'] not in completed and step['name'] not in failed:
        run_step(step)

print(f'\nCompleted: {len(completed)}/{len(steps)}')
if failed:
    print(f'Failed: {\", \".join(failed)}')
    sys.exit(1)
" 2>/dev/null

  local exit_code=$?
  if [ "$exit_code" -eq 0 ]; then
    ok "Pipeline complete"
  else
    err "Pipeline had failures"
    return 1
  fi
}
```

Add to dispatch:

```bash
  pipeline)   cmd_pipeline "$@" ;;
```

- [ ] **Step 4: Run all tests**

Run: `bash tests/run-all.sh`
Expected: All pass

- [ ] **Step 5: Commit**

```bash
git add bin/agent tests/test-pipeline.sh
git commit -m "feat: add agent pipeline for multi-step workflow orchestration

YAML-defined pipelines with sequential steps, retry policies,
failure handlers, and step dependencies. Supports --dry-run."
```

---

### Task 11: Update Help, Docs, and CLAUDE.md

**Files:**
- Modify: `bin/agent` (update `print_help_general` and `cmd_help`)
- Modify: `CLAUDE.md` (add new commands to reference)

- [ ] **Step 1: Update `print_help_general`**

Add the new commands to the appropriate sections:

```bash
${GREEN}Manage:${NC}
  agent list
  agent tui
  agent attach <name>|--latest
  agent cp <name>|--latest <container-path> <host-path>
  agent peek <name>|--latest [--lines N]
  agent diff <name>|--latest [--stat]
  agent stop <name>|--latest|--all
  agent cleanup [--auth]

${GREEN}Workflow:${NC}
  agent checkpoint <create|list|revert> <name>|--latest
  agent todo <add|list|check|uncheck> <name>|--latest
  agent pr <name>|--latest [--title T] [--base B]
  agent review <name>|--latest [--base B]

${GREEN}Fleet:${NC}
  agent fleet <manifest.yaml> [--dry-run]
  agent pipeline <pipeline.yaml> [--dry-run]

${GREEN}Analytics:${NC}
  agent cost <name>|--latest
  agent audit [--lines N]
```

- [ ] **Step 2: Update `cmd_help` dispatch**

Add help topics for all new commands:

```bash
    diff)       print_help_diff ;;
    checkpoint) print_help_checkpoint ;;
    todo)       print_help_todo ;;
    pr)         print_help_pr ;;
    review)     print_help_review ;;
    audit)      print_help_audit ;;
    cost)       print_help_cost ;;
    fleet)      print_help_fleet ;;
    pipeline)   print_help_pipeline ;;
```

- [ ] **Step 3: Update CLAUDE.md**

Add to the Commands section:

```markdown
# Workflow
agent diff <name>|--latest [--stat]   # show git diff from agent working tree
agent checkpoint create <name> [label] # snapshot working tree
agent checkpoint list <name>           # list snapshots
agent checkpoint revert <name> <ref>   # revert to snapshot
agent todo add <name> "text"           # add merge requirement
agent todo list <name>                 # show todos
agent todo check <name> <index>        # mark done
agent pr <name> [--title T --base B]   # create GitHub PR
agent review <name> [--base B]         # AI code review

# Fleet & Pipelines
agent fleet manifest.yaml              # spawn agents from manifest
agent pipeline pipeline.yaml           # run multi-step pipeline

# Analytics
agent cost <name>                      # estimate API spend
agent audit                            # show operation log
```

- [ ] **Step 4: Run full test suite**

Run: `bash tests/run-all.sh`
Expected: All pass

- [ ] **Step 5: Commit**

```bash
git add bin/agent CLAUDE.md
git commit -m "docs: update help text and CLAUDE.md for new commands

Add diff, checkpoint, todo, pr, review, fleet, pipeline, cost,
and audit to help output and project documentation."
```

---

### Task 12: Update TUI Footer for All New Keybindings

**Files:**
- Modify: `tui/footer.go` (update `allShortcuts` slice)

- [ ] **Step 1: Update the shortcuts array**

Replace the `allShortcuts` slice in `tui/footer.go`:

```go
var allShortcuts = []shortcut{
	{"a", "Attach"},
	{"r", "Resume"},
	{"s", "Stop"},
	{"l", "Logs"},
	{"d", "Describe"},
	{"f", "Diff"},
	{"x", "Checkpoint"},
	{"t", "Todos"},
	{"g", "PR"},
	{"y", "YAML"},
	{"n", "New"},
	{"p", "Preview"},
	{"e", "Export"},
	{"c", "Copy"},
	{"m", "MCP Login"},
	{"/", "Filter"},
	{":", "Command"},
	{"ctrl-d", "Delete"},
	{"ctrl-k", "Kill All"},
	{"q", "Quit"},
}
```

Update `shortcutRows` to 4 to accommodate the extra shortcuts:

```go
const shortcutRows = 4
```

- [ ] **Step 2: Rebuild TUI**

Run: `make -C tui build`

- [ ] **Step 3: Run TUI tests**

Run: `cd tui && go test ./...`
Expected: All pass

- [ ] **Step 4: Commit**

```bash
git add tui/footer.go
git commit -m "feat(tui): add keybindings for diff, checkpoint, todos, PR

Updated footer to show all new shortcuts. Expanded to 4 rows."
```

---

## Summary

| Phase | Tasks | New Commands | TUI Keys |
|-------|-------|-------------|----------|
| **1: Developer Workflow** | Tasks 1-4 | `diff`, `checkpoint`, `todo` + lifecycle scripts | `f`, `x`, `t` |
| **2: Competitive Parity** | Tasks 5-8 | `pr`, `review`, `audit`, `cost` | `g` |
| **3: Differentiation** | Tasks 9-10 | `fleet`, `pipeline` | — |
| **Docs** | Tasks 11-12 | — | Updated footer |

Total: 12 tasks, 10 new CLI commands, 4 new TUI keybindings, 8 new test files.
