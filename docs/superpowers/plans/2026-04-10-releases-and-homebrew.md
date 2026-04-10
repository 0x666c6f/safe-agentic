# Releases & Homebrew Distribution Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Automated semver releases on every push to `main` with Homebrew tap distribution.

**Architecture:** GitHub Actions release workflow computes semver from conventional commits, builds a universal macOS TUI binary, packages a tarball, creates a GitHub Release, then pushes an updated formula to a separate Homebrew tap repo.

**Tech Stack:** GitHub Actions, Go cross-compilation, `lipo` (macOS universal binary), Homebrew formula (Ruby DSL), `gh` CLI

---

### Task 1: Add `VERSION` variable and `--version` flag to `bin/agent`

**Files:**
- Modify: `bin/agent:5-8` (add VERSION variable)
- Modify: `bin/agent` bottom dispatch section (add `--version` case)
- Modify: `tests/test-cli-dispatch.sh` (add version test)

- [ ] **Step 1: Add the version test to `tests/test-cli-dispatch.sh`**

Add after the existing `--help flag` test (around line 123):

```bash
run_ok   "--version flag"   bash "$REPO_DIR/bin/agent" --version
assert_output_contains "safe-agentic" "--version shows project name"
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `bash tests/test-cli-dispatch.sh 2>&1 | tail -5`
Expected: FAIL — `--version` is treated as unknown command

- [ ] **Step 3: Add `VERSION` variable and `--version` dispatch to `bin/agent`**

Add `VERSION` variable after line 4 (`REPO_DIR=...`):

```bash
VERSION="dev"
```

Add `--version` case in the dispatch `case` block at the bottom of the file, before `help|-h|--help)`:

```bash
  --version)  echo "safe-agentic $VERSION"; exit 0 ;;
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `bash tests/test-cli-dispatch.sh 2>&1 | tail -5`
Expected: PASS

- [ ] **Step 5: Run full test suite**

Run: `bash tests/run-all.sh 2>&1 | tail -3`
Expected: all pass, 0 failed

- [ ] **Step 6: Commit**

```bash
git add bin/agent tests/test-cli-dispatch.sh
git commit -m "feat: add --version flag to agent CLI"
```

---

### Task 2: Create the release workflow

**Files:**
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Create `.github/workflows/release.yml`**

```yaml
name: Release

on:
  push:
    branches: [main]

permissions:
  contents: write

jobs:
  ci:
    name: CI Gate
    uses: ./.github/workflows/ci.yml

  release:
    name: Release
    needs: ci
    runs-on: macos-latest
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    steps:
      - name: Checkout
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Compute next version
        id: version
        run: |
          latest_tag=$(git describe --tags --abbrev=0 2>/dev/null || echo "")
          if [ -z "$latest_tag" ]; then
            # No tags yet — first release
            echo "version=v0.1.0" >> "$GITHUB_OUTPUT"
            echo "skip=false" >> "$GITHUB_OUTPUT"
            echo "changelog=Initial release" >> "$GITHUB_OUTPUT"
            exit 0
          fi

          # Check if HEAD is already tagged
          if git tag --points-at HEAD | grep -q '^v'; then
            echo "skip=true" >> "$GITHUB_OUTPUT"
            exit 0
          fi

          # Collect commits since last tag
          commits=$(git log "${latest_tag}..HEAD" --pretty=format:"%s" --no-merges)
          if [ -z "$commits" ]; then
            echo "skip=true" >> "$GITHUB_OUTPUT"
            exit 0
          fi

          # Determine bump type from conventional commit prefixes
          bump="none"
          while IFS= read -r msg; do
            if echo "$msg" | grep -qE '^[a-z]+(\(.+\))?!:|BREAKING CHANGE'; then
              bump="major"
              break
            elif echo "$msg" | grep -qE '^feat(\(.+\))?:'; then
              [ "$bump" != "major" ] && bump="minor"
            elif echo "$msg" | grep -qE '^fix(\(.+\))?:'; then
              [ "$bump" = "none" ] && bump="patch"
            fi
          done <<< "$commits"

          if [ "$bump" = "none" ]; then
            echo "skip=true" >> "$GITHUB_OUTPUT"
            exit 0
          fi

          # Parse current version and bump
          current="${latest_tag#v}"
          IFS='.' read -r major minor patch <<< "$current"
          case "$bump" in
            major) major=$((major + 1)); minor=0; patch=0 ;;
            minor) minor=$((minor + 1)); patch=0 ;;
            patch) patch=$((patch + 1)) ;;
          esac
          next="v${major}.${minor}.${patch}"

          # Generate changelog
          changelog=""
          feats=$(git log "${latest_tag}..HEAD" --pretty=format:"- %s" --no-merges | grep -E '^\- feat' || true)
          fixes=$(git log "${latest_tag}..HEAD" --pretty=format:"- %s" --no-merges | grep -E '^\- fix' || true)
          other=$(git log "${latest_tag}..HEAD" --pretty=format:"- %s" --no-merges | grep -vE '^\- (feat|fix)' || true)
          [ -n "$feats" ] && changelog="${changelog}### Features\n${feats}\n\n"
          [ -n "$fixes" ] && changelog="${changelog}### Fixes\n${fixes}\n\n"
          [ -n "$other" ] && changelog="${changelog}### Other\n${other}\n\n"

          echo "version=$next" >> "$GITHUB_OUTPUT"
          echo "skip=false" >> "$GITHUB_OUTPUT"
          # Use a delimiter for multiline output
          {
            echo "changelog<<CHANGELOGEOF"
            printf '%b' "$changelog"
            echo "CHANGELOGEOF"
          } >> "$GITHUB_OUTPUT"

      - name: Skip if no release needed
        if: steps.version.outputs.skip == 'true'
        run: echo "No user-facing changes since last release. Skipping."

      - name: Set up Go
        if: steps.version.outputs.skip != 'true'
        uses: actions/setup-go@v5
        with:
          go-version-file: tui/go.mod
          cache-dependency-path: tui/go.sum

      - name: Build universal TUI binary
        if: steps.version.outputs.skip != 'true'
        run: |
          cd tui
          GOOS=darwin GOARCH=amd64 go build -o agent-tui-amd64 .
          GOOS=darwin GOARCH=arm64 go build -o agent-tui-arm64 .
          lipo -create -output agent-tui agent-tui-amd64 agent-tui-arm64
          rm agent-tui-amd64 agent-tui-arm64
          chmod +x agent-tui

      - name: Package tarball
        if: steps.version.outputs.skip != 'true'
        id: package
        run: |
          version="${{ steps.version.outputs.version }}"
          staging="safe-agentic-${version}"
          mkdir -p "$staging"/{bin,config,templates,tui,vm}

          # Scripts
          cp bin/agent bin/agent-lib.sh bin/agent-claude bin/agent-codex \
             bin/agent-alias bin/agent-session.sh bin/docker-runtime.sh \
             bin/repo-url.sh "$staging/bin/"

          # TUI binary
          cp tui/agent-tui "$staging/tui/"

          # Config, templates, VM
          cp config/* "$staging/config/"
          cp templates/* "$staging/templates/"
          cp vm/setup.sh "$staging/vm/"

          # Root files
          cp Dockerfile entrypoint.sh package.json package-lock.json op-env.sh "$staging/"

          # Inject version
          sed -i '' "s/VERSION=\"dev\"/VERSION=\"${version}\"/" "$staging/bin/agent"

          # Create tarball
          tarball="safe-agentic-${version}-darwin-universal.tar.gz"
          tar -czf "$tarball" "$staging"
          sha256=$(shasum -a 256 "$tarball" | awk '{print $1}')

          echo "tarball=$tarball" >> "$GITHUB_OUTPUT"
          echo "sha256=$sha256" >> "$GITHUB_OUTPUT"

      - name: Create GitHub Release
        if: steps.version.outputs.skip != 'true'
        env:
          GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          version="${{ steps.version.outputs.version }}"
          tarball="${{ steps.package.outputs.tarball }}"

          git tag "$version"
          git push origin "$version"

          gh release create "$version" "$tarball" \
            --title "$version" \
            --notes "${{ steps.version.outputs.changelog }}"

      - name: Update Homebrew tap
        if: steps.version.outputs.skip != 'true'
        env:
          TAP_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}
        run: |
          version="${{ steps.version.outputs.version }}"
          sha256="${{ steps.package.outputs.sha256 }}"
          url="https://github.com/0x666c6f/safe-agentic/releases/download/${version}/safe-agentic-${version}-darwin-universal.tar.gz"

          git clone "https://x-access-token:${TAP_TOKEN}@github.com/0x666c6f/homebrew-tap.git" /tmp/homebrew-tap
          cd /tmp/homebrew-tap

          cat > Formula/safe-agentic.rb << FORMULA
          class SafeAgentic < Formula
            desc "Isolated environment for running AI coding agents safely"
            homepage "https://github.com/0x666c6f/safe-agentic"
            url "${url}"
            sha256 "${sha256}"
            version "${version#v}"
            license "MIT"

            def install
              libexec.install Dir["*"]
              %w[agent agent-claude agent-codex].each do |cmd|
                bin.install_symlink libexec/"bin"/cmd
              end
            end

            test do
              assert_match "safe-agentic", shell_output("\#{bin}/agent --version")
            end
          end
          FORMULA

          # Fix indentation (heredoc above is indented)
          sed -i '' 's/^          //' Formula/safe-agentic.rb

          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git add Formula/safe-agentic.rb
          git commit -m "safe-agentic ${version}"
          git push
```

- [ ] **Step 2: Validate the workflow YAML syntax**

Run: `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/release.yml'))" && echo "YAML valid"`
Expected: `YAML valid`

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "feat: add automated release workflow with Homebrew tap update"
```

---

### Task 3: Create the Homebrew tap repository

**Files:**
- Create (external repo): `0x666c6f/homebrew-tap` with `Formula/safe-agentic.rb` and `README.md`

- [ ] **Step 1: Create the tap repository on GitHub**

```bash
gh repo create 0x666c6f/homebrew-tap --public --description "Homebrew formulae for 0x666c6f projects"
```

- [ ] **Step 2: Clone and set up initial structure**

```bash
cd /tmp
git clone git@github.com:0x666c6f/homebrew-tap.git
cd homebrew-tap
mkdir -p Formula
```

- [ ] **Step 3: Create placeholder formula**

Write `Formula/safe-agentic.rb`:

```ruby
class SafeAgentic < Formula
  desc "Isolated environment for running AI coding agents safely"
  homepage "https://github.com/0x666c6f/safe-agentic"
  url "https://github.com/0x666c6f/safe-agentic/releases/download/v0.1.0/safe-agentic-v0.1.0-darwin-universal.tar.gz"
  sha256 "placeholder"
  version "0.1.0"
  license "MIT"

  def install
    libexec.install Dir["*"]
    %w[agent agent-claude agent-codex].each do |cmd|
      bin.install_symlink libexec/"bin"/cmd
    end
  end

  test do
    assert_match "safe-agentic", shell_output("#{bin}/agent --version")
  end
end
```

- [ ] **Step 4: Create `README.md`**

```markdown
# Homebrew Tap

Homebrew formulae for [0x666c6f](https://github.com/0x666c6f) projects.

## Install

```
brew tap 0x666c6f/tap
brew install safe-agentic
```
```

- [ ] **Step 5: Push initial commit**

```bash
git add Formula/safe-agentic.rb README.md
git commit -m "feat: add safe-agentic formula"
git push origin main
```

---

### Task 4: Create `HOMEBREW_TAP_TOKEN` secret and trigger first release

**Files:** None (GitHub settings + manual trigger)

- [ ] **Step 1: Create a GitHub PAT**

Go to https://github.com/settings/tokens and create a fine-grained PAT:
- Name: `homebrew-tap-push`
- Repository access: `0x666c6f/homebrew-tap` only
- Permissions: Contents → Read and write

- [ ] **Step 2: Add the secret to safe-agentic repo**

```bash
gh secret set HOMEBREW_TAP_TOKEN --repo 0x666c6f/safe-agentic
```

Paste the PAT when prompted.

- [ ] **Step 3: Push all changes to trigger the first release**

```bash
git push origin main
```

The release workflow will:
1. Run CI
2. Detect `feat:` commits since there are no tags → `v0.1.0`
3. Build universal TUI binary
4. Create tarball and GitHub Release
5. Update the Homebrew tap formula with the real SHA256

- [ ] **Step 4: Verify the release**

```bash
gh release view v0.1.0
```

Expected: Release exists with `safe-agentic-v0.1.0-darwin-universal.tar.gz` attached

- [ ] **Step 5: Verify Homebrew installation**

```bash
brew tap 0x666c6f/tap
brew install safe-agentic
agent --version
```

Expected: `safe-agentic v0.1.0`
