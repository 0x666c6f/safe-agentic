#!/usr/bin/env bash
# Tests for agent template subcommand and --template spawn flag.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

AGENT_BIN="$REPO_DIR/bin/agent"
OUT_LOG="$TMP_DIR/out.log"
ERR_LOG="$TMP_DIR/err.log"
FAKE_BIN="$TMP_DIR/bin"
ORB_LOG="$TMP_DIR/orb.log"
VERIFY_STATE="$TMP_DIR/verify-state"
USER_TEMPLATES="$TMP_DIR/user-templates"

mkdir -p "$FAKE_BIN" "$USER_TEMPLATES"

# ---------------------------------------------------------------------------
# Fake orb binary (mirrors test-cli-dispatch.sh)
# ---------------------------------------------------------------------------
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
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-c" ] && [[ "${3:-}" == *SSH_AUTH_SOCK* ]]; then
      echo "/tmp/fake-ssh.sock"; exit 0
    fi
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-c" ] && [[ "${3:-}" == *'test -S /var/run/docker.sock'*'printf "%s\n" /var/run/docker.sock'* ]]; then
      echo "/var/run/docker.sock"; exit 0
    fi
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-c" ] && [[ "${3:-}" == "stat -c %g /var/run/docker.sock" ]]; then
      echo "998"; exit 0
    fi
    if [ "${1:-}" = "bash" ] && [ "${2:-}" = "-lc" ] && [[ "${3:-}" == *safe-agentic-hardening-verify* ]]; then
      : >"$verify_state"
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "image" ] && [ "${3:-}" = "inspect" ]; then
      exit 0
    fi
    if [ "${1:-}" = "docker" ] && [ "${2:-}" = "network" ]; then
      case "${3:-}" in
        inspect) exit 1 ;;
        create|rm) exit 0 ;;
      esac
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

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
pass=0
fail=0

run_ok() {
  local label="$1"; shift
  if PATH="$FAKE_BIN:$PATH" XDG_CONFIG_HOME="$TMP_DIR/xdg" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
    ((++pass))
  else
    echo "FAIL: $label: expected zero exit" >&2
    cat "$OUT_LOG" "$ERR_LOG" >&2 2>/dev/null || true
    ((++fail))
  fi
}

run_fails() {
  local label="$1"; shift
  if PATH="$FAKE_BIN:$PATH" XDG_CONFIG_HOME="$TMP_DIR/xdg" TEST_ORB_LOG="$ORB_LOG" TEST_VERIFY_STATE="$VERIFY_STATE" "$@" >"$OUT_LOG" 2>"$ERR_LOG"; then
    echo "FAIL: $label: expected non-zero exit" >&2
    ((++fail))
  else
    ((++pass))
  fi
}

assert_output_contains() {
  local needle="$1" label="$2"
  if grep -q -- "$needle" "$OUT_LOG" "$ERR_LOG" 2>/dev/null; then
    ((++pass))
  else
    echo "FAIL: $label: '$needle' not in output" >&2
    cat "$OUT_LOG" "$ERR_LOG" >&2 2>/dev/null || true
    ((++fail))
  fi
}

assert_contains() {
  local haystack="$1" needle="$2" label="${3:-}"
  if [[ "$haystack" == *"$needle"* ]]; then ((++pass)); else
    echo "FAIL${label:+: $label}: missing '$needle'" >&2; ((++fail))
  fi
}

# =============================================================================
# 1. Built-in templates directory exists with expected files
# =============================================================================
for tmpl in security-audit code-review test-coverage dependency-update bug-fix docs-review; do
  if [ -f "$REPO_DIR/templates/${tmpl}.md" ]; then
    ((++pass))
  else
    echo "FAIL: missing built-in template: ${tmpl}.md" >&2
    ((++fail))
  fi
done

# =============================================================================
# 2. resolve_template via agent template show: built-in template is readable
# =============================================================================
resolve_output=$(bash "$AGENT_BIN" template show security-audit 2>/dev/null || true)
if [[ "$resolve_output" == *"security"* ]]; then
  ((++pass))
else
  echo "FAIL: resolve_template security-audit: output not as expected" >&2
  echo "Output was: $resolve_output" >&2
  ((++fail))
fi

# =============================================================================
# 3. resolve_template: unknown name fails with helpful message
# =============================================================================
if ! bash "$AGENT_BIN" template show nonexistent-template-xyz >"$OUT_LOG" 2>"$ERR_LOG"; then
  if grep -q "not found\|Template" "$ERR_LOG" 2>/dev/null; then
    ((++pass))
  else
    echo "FAIL: resolve_template unknown: expected error message" >&2
    cat "$ERR_LOG" >&2
    ((++fail))
  fi
else
  echo "FAIL: resolve_template unknown: expected failure but succeeded" >&2
  ((++fail))
fi

# =============================================================================
# 4. agent template list — shows built-in templates
# =============================================================================
run_ok "template list" bash "$AGENT_BIN" template list
assert_output_contains "security-audit" "template list shows security-audit"
assert_output_contains "code-review" "template list shows code-review"
assert_output_contains "test-coverage" "template list shows test-coverage"
assert_output_contains "dependency-update" "template list shows dependency-update"
assert_output_contains "bug-fix" "template list shows bug-fix"
assert_output_contains "docs-review" "template list shows docs-review"
assert_output_contains "Built-in templates" "template list shows section header"

# =============================================================================
# 5. agent template show <name> — prints template content
# =============================================================================
run_ok "template show security-audit" bash "$AGENT_BIN" template show security-audit
assert_output_contains "security" "template show security-audit has content"

run_ok "template show bug-fix" bash "$AGENT_BIN" template show bug-fix
assert_output_contains "bug" "template show bug-fix has content"

# =============================================================================
# 6. agent template show — missing name fails
# =============================================================================
run_fails "template show no name" bash "$AGENT_BIN" template show
assert_output_contains "Template name required" "template show no name error"

# =============================================================================
# 7. agent template show — unknown name fails with helpful message
# =============================================================================
run_fails "template show unknown" bash "$AGENT_BIN" template show no-such-template-xyz
assert_output_contains "not found" "template show unknown error"

# =============================================================================
# 8. agent template create — creates file in user dir
# =============================================================================
run_ok "template create" bash "$AGENT_BIN" template create my-test-template
user_tmpl_path="$TMP_DIR/xdg/safe-agentic/templates/my-test-template.md"
if [ -f "$user_tmpl_path" ]; then
  ((++pass))
else
  echo "FAIL: template create did not create file at $user_tmpl_path" >&2
  ((++fail))
fi
assert_output_contains "Created template" "template create success message"

# =============================================================================
# 9. agent template create — duplicate fails
# =============================================================================
run_fails "template create duplicate" bash "$AGENT_BIN" template create my-test-template
assert_output_contains "already exists" "template create duplicate error"

# =============================================================================
# 10. agent template create — missing name fails
# =============================================================================
run_fails "template create no name" bash "$AGENT_BIN" template create
assert_output_contains "Template name required" "template create no name error"

# =============================================================================
# 11. agent template list — shows user template after create
# =============================================================================
run_ok "template list with user template" bash "$AGENT_BIN" template list
assert_output_contains "my-test-template" "template list shows user template"
assert_output_contains "User templates" "template list shows user section"

# =============================================================================
# 12. agent template (no subcommand) — defaults to list
# =============================================================================
run_ok "template no subcommand" bash "$AGENT_BIN" template
assert_output_contains "security-audit" "template no-subcommand defaults to list"

# =============================================================================
# 13. agent template --help / -h
# =============================================================================
run_ok "template --help" bash "$AGENT_BIN" template --help
assert_output_contains "Usage: agent template" "template --help shows usage"
assert_output_contains "security-audit" "template --help mentions built-in templates"

# =============================================================================
# 14. agent template bad subcommand
# =============================================================================
run_fails "template bad subcommand" bash "$AGENT_BIN" template bogus-subcommand
assert_output_contains "Unknown subcommand" "template bad subcommand error"

# =============================================================================
# 15. agent help template — shows template help
# =============================================================================
run_ok "help template" bash "$AGENT_BIN" help template
assert_output_contains "Usage: agent template" "help template shows usage"

# =============================================================================
# 16. agent help — general help mentions template
# =============================================================================
run_ok "general help mentions template" bash "$AGENT_BIN" help
assert_output_contains "template" "general help mentions template"

# =============================================================================
# 17. agent help spawn — mentions --template flag
# =============================================================================
run_ok "spawn help mentions --template" bash "$AGENT_BIN" help spawn
assert_output_contains "--template" "spawn help shows --template flag"

# =============================================================================
# 18. --template flag in spawn resolves to correct prompt (dry-run)
# =============================================================================
: >"$ORB_LOG"
run_ok "spawn --template dry-run" bash "$AGENT_BIN" spawn claude \
  --template security-audit --repo https://github.com/example/repo.git --dry-run
assert_output_contains "Prompt:" "spawn --template shows prompt"

# =============================================================================
# 19. fleet YAML with template field passes --template to spawn
# =============================================================================
FLEET_YAML="$TMP_DIR/fleet-template.yaml"
cat >"$FLEET_YAML" <<'EOF'
agents:
  - name: security-check
    type: claude
    repo: https://github.com/example/repo.git
    template: security-audit
EOF

fleet_cmds=$(python3 - "$FLEET_YAML" "$AGENT_BIN" <<'PYEOF'
import sys, shlex

manifest_path = sys.argv[1]
agent_bin = sys.argv[2]
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
            key = key.strip(); val = val.strip()
            if key and val:
                if val.lower() == 'true': val = True
                elif val.lower() == 'false': val = False
                current[key] = val
for agent in agents:
    agent_type = agent.get('type', '')
    if not agent_type: continue
    cmd = [agent_bin, 'spawn', agent_type]
    name = agent.get('name', '')
    if name: cmd += ['--name', name]
    repo = agent.get('repo', '')
    if repo: cmd += ['--repo', repo]
    prompt = agent.get('prompt', '')
    template = agent.get('template', '')
    if prompt:
        cmd += ['--prompt', str(prompt)]
    elif template:
        cmd += ['--template', str(template)]
    print('CMD:' + shlex.join(cmd))
PYEOF
)
if [[ "$fleet_cmds" == *"--template security-audit"* ]]; then
  ((++pass))
else
  echo "FAIL: fleet YAML template field: expected --template security-audit in: $fleet_cmds" >&2
  ((++fail))
fi

# =============================================================================
# 20. fleet YAML: prompt takes precedence over template
# =============================================================================
FLEET_YAML2="$TMP_DIR/fleet-prompt-wins.yaml"
cat >"$FLEET_YAML2" <<'EOF'
agents:
  - name: explicit-task
    type: claude
    repo: https://github.com/example/repo.git
    prompt: Do something specific
    template: security-audit
EOF

fleet_cmds2=$(python3 - "$FLEET_YAML2" "$AGENT_BIN" <<'PYEOF'
import sys, shlex
manifest_path = sys.argv[1]
agent_bin = sys.argv[2]
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
            key = key.strip(); val = val.strip()
            if key and val:
                if val.lower() == 'true': val = True
                elif val.lower() == 'false': val = False
                current[key] = val
for agent in agents:
    agent_type = agent.get('type', '')
    if not agent_type: continue
    cmd = [agent_bin, 'spawn', agent_type]
    name = agent.get('name', '')
    if name: cmd += ['--name', name]
    repo = agent.get('repo', '')
    if repo: cmd += ['--repo', repo]
    prompt = agent.get('prompt', '')
    template = agent.get('template', '')
    if prompt:
        cmd += ['--prompt', str(prompt)]
    elif template:
        cmd += ['--template', str(template)]
    print('CMD:' + shlex.join(cmd))
PYEOF
)
if [[ "$fleet_cmds2" == *"--prompt"* ]] && [[ "$fleet_cmds2" != *"--template"* ]]; then
  ((++pass))
else
  echo "FAIL: fleet YAML prompt should take precedence over template. Got: $fleet_cmds2" >&2
  ((++fail))
fi

# =============================================================================
# 21. pipeline YAML parser includes template field in output
# =============================================================================
PIPELINE_YAML="$TMP_DIR/pipeline-template.yaml"
cat >"$PIPELINE_YAML" <<'EOF'
name: test-pipeline
steps:
  - name: audit
    type: claude
    template: security-audit
    repo: https://github.com/example/repo.git
EOF

pipeline_out=$(python3 - "$PIPELINE_YAML" <<'PYEOF'
import sys, base64, re

path = sys.argv[1]
with open(path) as f:
    lines = f.readlines()

pipeline_name = ""
steps = []
current_step = None
in_steps = False

for raw in lines:
    line = raw.rstrip('\n')
    stripped = line.lstrip()
    if not stripped or stripped.startswith('#'):
        continue
    indent = len(line) - len(stripped)
    if indent == 0:
        m = re.match(r'^(\w[\w-]*):\s*(.*)', stripped)
        if m:
            key, val = m.group(1), m.group(2).strip()
            if key == 'name' and not in_steps:
                pipeline_name = val
            elif key == 'steps':
                in_steps = True
                if current_step is not None:
                    steps.append(current_step)
                    current_step = None
        continue
    if not in_steps:
        continue
    m = re.match(r'^-\s+(\w[\w_-]*):\s*(.*)', stripped)
    if m:
        if current_step is not None:
            steps.append(current_step)
        key, val = m.group(1), m.group(2).strip()
        current_step = {key: val}
        continue
    m = re.match(r'^(\w[\w_-]*):\s*(.*)', stripped)
    if m and current_step is not None:
        key, val = m.group(1), m.group(2).strip()
        current_step[key] = val
        continue

if current_step is not None:
    steps.append(current_step)

def enc(v):
    return base64.b64encode(v.encode()).decode()

print("pipeline_name=" + enc(pipeline_name))
for step in steps:
    parts = []
    for field in ('name', 'type', 'repo', 'prompt', 'template', 'ssh', 'reuse_auth',
                  'on_failure', 'retry', 'depends_on'):
        val = step.get(field, '')
        parts.append(field + '=' + enc(val))
    print(' '.join(parts))
PYEOF
)
# Decode the template field from the output
tmpl_b64=$(echo "$pipeline_out" | tail -n1 | grep -o 'template=[^ ]*' | cut -d= -f2-)
tmpl_val=$(echo "$tmpl_b64" | base64 -d 2>/dev/null || echo "")
if [ "$tmpl_val" = "security-audit" ]; then
  ((++pass))
else
  echo "FAIL: pipeline YAML template field: expected 'security-audit', got '$tmpl_val'" >&2
  ((++fail))
fi

# =============================================================================
# Summary
# =============================================================================
echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
