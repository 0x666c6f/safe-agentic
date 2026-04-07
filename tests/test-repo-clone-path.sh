#!/usr/bin/env bash
# Unit tests for repo_clone_path() from entrypoint.sh.
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Extract the function definition from entrypoint.sh so we can test it
# without running the side-effectful top-level code.
eval "$(sed -n '/^repo_clone_path()/,/^}/p' "$REPO_DIR/entrypoint.sh")"

pass=0
fail=0

expect_ok() {
  local input="$1"
  local expected="$2"
  local actual
  if actual=$(repo_clone_path "$input"); then
    if [ "$actual" = "$expected" ]; then
      ((++pass))
      return
    fi
    echo "FAIL: repo_clone_path '$input' → '$actual' (expected '$expected')" >&2
  else
    echo "FAIL: repo_clone_path '$input' returned non-zero (expected '$expected')" >&2
  fi
  ((++fail))
}

expect_fail() {
  local input="$1"
  local label="${2:-}"
  if repo_clone_path "$input" >/dev/null 2>&1; then
    echo "FAIL: repo_clone_path '$input' should have failed${label:+ ($label)}" >&2
    ((++fail))
    return
  fi
  ((++pass))
}

# --- Valid URLs ---
expect_ok "git@github.com:acme/repo.git"       "acme/repo"
expect_ok "git@github.com:acme/repo"            "acme/repo"
expect_ok "https://github.com/acme/repo.git"    "acme/repo"
expect_ok "https://github.com/acme/repo"        "acme/repo"
expect_ok "ssh://git@github.com/acme/repo.git"  "acme/repo"
expect_ok "git@github.com:my-org/my-repo.git"   "my-org/my-repo"
expect_ok "git@github.com:Org.Name/repo_name.git" "Org.Name/repo_name"
expect_ok "git@github.com:a/b.git"              "a/b"
expect_ok "https://github.com/A1/B2"            "A1/B2"

# --- Path traversal attacks ---
expect_fail "git@github.com:../etc/passwd.git"     "path traversal (..)"
expect_fail "git@github.com:./hidden/repo.git"     "path traversal (.)"
expect_fail "git@github.com:acme/..repo"           "dot-dot in repo"
expect_fail "git@github.com:..org/repo"            "dot-dot as owner"

# --- Empty or missing components ---
expect_fail "git@github.com:/repo.git"              "empty owner"
expect_fail "git@github.com:acme/.git"              "empty repo (just .git)"
expect_fail "not-a-url"                             "no separator"
expect_fail "https://github.com/single"             "single path component"
expect_fail "https://github.com/acme/repo/extra"    "extra path segment"
expect_fail "ssh://git@github.com/acme/repo/extra.git" "extra path segment over ssh"
expect_fail "git@github.com:acme/repo.git/extra"    "suffix after .git"
expect_fail "https://github.com/acme//repo"         "double slash"
expect_fail ""                                      "empty string"

# --- Special characters (shell injection) ---
expect_fail 'git@github.com:acme/repo;rm -rf.git'  "semicolon injection"
expect_fail 'git@github.com:acme/repo$(id).git'    "command substitution"
expect_fail 'git@github.com:acme/repo`id`.git'     "backtick injection"
expect_fail 'git@github.com:ac me/repo.git'        "space in owner"
expect_fail 'git@github.com:acme/re po.git'        "space in repo"
expect_fail 'git@github.com:acme/repo&bg.git'      "ampersand"
expect_fail 'git@github.com:acme/repo|pipe.git'    "pipe"
expect_fail 'git@github.com:acme/repo>out.git'     "redirect"

# --- GitHub Enterprise / custom hosts ---
expect_ok "git@ghe.corp.com:team/service.git"     "team/service"
expect_ok "https://gitlab.internal/group/proj.git" "group/proj"
expect_ok "ssh://git@bitbucket.org/org/repo.git"   "org/repo"

# --- Edge cases ---
expect_ok  "git@github.com:a/b"                   "a/b"
expect_fail "git@github.com:a/b/c.git"            "three-level path"
expect_fail "git://github.com/acme/repo.git"      "git:// protocol (no auth)"
expect_fail "https://github.com/acme/repo.git?ref=main"  "query params"
expect_fail "ftp://files.example.com/a/b.git"     "ftp scheme"

# --- Double .git suffix ---
expect_ok "git@github.com:acme/repo.git.git"      "acme/repo.git"

# --- Trailing slash is invalid ---
expect_fail "https://github.com/acme/repo/"        "trailing slash"

# --- Only dot in components ---
expect_fail "git@github.com:./."                    "dot owner and repo"

echo "$((pass + fail)) tests, $pass passed, $fail failed"
[ "$fail" -eq 0 ]
