#!/usr/bin/env bash
# Static analysis of vm/setup.sh security hardening.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SETUP="$REPO_DIR/vm/setup.sh"

pass=0
fail=0

assert_present() {
  local pattern="$1" label="$2"
  if grep -qE "$pattern" "$SETUP"; then ((++pass)); else
    echo "FAIL: $label" >&2; ((++fail))
  fi
}

# --- Strict mode ---
assert_present '^set -euo pipefail$' "strict mode"

# --- macOS mount blocking ---
for mnt in /Users /mnt/mac /Volumes /private /opt/orbstack; do
  assert_present "$(printf '%s' "$mnt" | sed 's|/|\\/|g')" "blocks $mnt"
done

# --- OrbStack integration command removal ---
for cmd in open osascript code mac; do
  assert_present "\\b$cmd\\b" "handles $cmd removal"
done

# --- Hardening verification step ---
assert_present 'Verifying.*hardening' "has verification step"

# --- Docker daemon security ---
assert_present 'userns-remap' "Docker user namespace remapping"
assert_present 'log-driver.*json-file' "Docker log driver configured"
assert_present 'max-size.*10m' "Docker log max size"

# --- Managed-network egress guardrails ---
assert_present 'SAFE_AGENTIC_EGRESS' "managed-network firewall chain"
assert_present 'DOCKER-USER' "hooks guardrails into docker-user chain"
assert_present "for port in 22 80 443" "declares allowed managed-network ports"
assert_present 'iptables -A SAFE_AGENTIC_EGRESS -i '"'"'sa\+'"'"' -p tcp --dport "\$port" -j RETURN' "allows declared web and ssh ports on managed bridges"
assert_present "192\\.168\\.0\\.0/16" "includes private cidr block list"
assert_present 'iptables -A SAFE_AGENTIC_EGRESS -i '"'"'sa\+'"'"' -d "\$cidr" -j REJECT' "blocks private egress on managed bridges"

# --- Docker installed from official repo ---
assert_present 'download.docker.com' "Docker from official repo"

# --- fstab entries to persist blocking ---
assert_present 'fstab' "persistent mount blocking via fstab"
assert_present 'tmpfs.*ro.*noexec.*nosuid' "tmpfs overlay is ro,noexec,nosuid"

echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
