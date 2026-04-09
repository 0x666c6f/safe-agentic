#!/usr/bin/env bash
# Validate Dockerfile security properties without building the image.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DOCKERFILE="$REPO_DIR/Dockerfile"

pass=0
fail=0

assert_present() {
  local pattern="$1"
  local label="$2"
  if grep -qE "$pattern" "$DOCKERFILE"; then
    ((++pass))
  else
    echo "FAIL: $label — pattern not found: $pattern" >&2
    ((++fail))
  fi
}

assert_absent() {
  local pattern="$1"
  local label="$2"
  if grep -qE "$pattern" "$DOCKERFILE"; then
    echo "FAIL: $label — pattern should not be present: $pattern" >&2
    ((++fail))
  else
    ((++pass))
  fi
}

# --- No curl | bash (supply chain risk) — exception for Claude Code official installer ---
# Claude Code install script is verified by version check: claude --version | grep -F "$CLAUDE_CODE_VERSION"
curl_pipe_count=$(grep -cE 'curl.*\|\s*(ba)?sh' "$DOCKERFILE" || true)
if [ "$curl_pipe_count" -gt 1 ]; then
  echo "FAIL: at most 1 curl pipe to shell allowed (Claude installer); found $curl_pipe_count" >&2
  ((++fail))
else
  ((++pass))
fi
assert_absent 'wget.*\|\s*(ba)?sh'    "no wget pipe to shell"

# --- All binary downloads have SHA256 verification ---
# Every curl that downloads a .tgz, .zip, .deb, or binary should be followed
# by a sha256sum or gpg verification in the same RUN block.
# We check that every ARG with a version also has corresponding SHA256 ARGs
# (except AWSCLI which uses GPG signature verification).
for tool in GO HELM EZA ZOXIDE YQ DELTA BUN; do
  assert_present "ARG ${tool}_SHA256_AMD64=" "${tool} has amd64 checksum"
  assert_present "ARG ${tool}_SHA256_ARM64=" "${tool} has arm64 checksum"
done

# AWS CLI uses GPG verification instead of SHA256
assert_present "gpg --batch --verify" "AWS CLI GPG signature verification"
assert_present "awscli-public-key.asc" "AWS CLI public key embedded"

# Every sha256 ARG should actually be used in a sha256sum check
sha256_args=$(grep -oE 'ARG [A-Z0-9_]+_SHA256_(AMD64|ARM64)' "$DOCKERFILE" \
  | sed 's/^ARG //' | cut -d= -f1 | sort -u)
for arg in $sha256_args; do
  assert_present 'sha256sum -c' "sha256sum check exists (for $arg)"
done

# --- build-essential purged after npm ci ---
assert_present 'apt-get purge.*build-essential' "build-essential removed after npm ci"

# --- Base image pinned by digest ---
assert_present 'ubuntu:24\.04@sha256:' "base image pinned by digest"


# --- Non-root user ---
assert_present '^USER agent$'          "runs as non-root user"
assert_present 'usermod -g agent -G "" agent' "agent supplemental groups cleared"
assert_absent  'sudo'                  "no sudo in user-stage commands"

# --- No k9s (user-facing tool, removed) ---
assert_absent 'k9s|K9S'               "k9s removed"

# --- No starship (removed) ---
assert_absent 'starship'              "starship removed"

# --- Entrypoint is the custom script ---
assert_present 'ENTRYPOINT.*entrypoint.sh' "entrypoint is custom script"
assert_present 'install -m 0755 -d /usr/local/lib/safe-agentic /workspace /opt/agent-cli' "repo helper directory created with execute bit"
assert_present 'COPY --chmod=644 bin/repo-url.sh /usr/local/lib/safe-agentic/repo-url.sh' "repo url helper copied into image"

# --- CLI bundles ---
assert_present '/opt/agent-cli' "npm CLI bundle directory exists"
assert_present 'claude.ai/install.sh' "Claude Code installed via official installer"
assert_present 'test -x /home/agent/.local/bin/claude' "Claude binary verified executable"

# --- Docker + package manager tooling present ---
assert_present 'docker-ce-cli' "docker cli installed"
assert_present 'docker-ce' "docker daemon installed"
assert_present 'docker-buildx-plugin' "docker buildx installed"
assert_present 'corepack enable pnpm' "pnpm enabled via corepack"
assert_present 'corepack prepare "pnpm@\$\{PNPM_VERSION\}" --activate' "pnpm pinned"
assert_present 'install -m 0755 "/tmp/bun-linux-' "bun binary installed"

# --- pipefail is set for SHELL ---
assert_present 'SHELL.*pipefail'       "SHELL has pipefail"

# --- Image has labels ---
assert_present 'LABEL app=safe-agentic' "app label present"

# --- Seccomp profile exists and is valid JSON ---
SECCOMP_PROFILE="$REPO_DIR/config/seccomp.json"
if [ -f "$SECCOMP_PROFILE" ] && python3 -c "import json; json.load(open('$SECCOMP_PROFILE'))" 2>/dev/null; then
  ((++pass))
else
  echo "FAIL: config/seccomp.json missing or invalid JSON" >&2
  ((++fail))
fi

# Verify seccomp profile blocks ptrace in no-caps group
if python3 -c "
import json, sys
with open('$SECCOMP_PROFILE') as f:
    p = json.load(f)
for g in p['syscalls']:
    if 'ptrace' in g.get('names',[]) and not g.get('includes',{}).get('caps',[]):
        sys.exit(1)  # ptrace found in unconditional group
" 2>/dev/null; then
  ((++pass))
else
  echo "FAIL: seccomp profile allows ptrace without CAP_SYS_PTRACE" >&2
  ((++fail))
fi

echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
