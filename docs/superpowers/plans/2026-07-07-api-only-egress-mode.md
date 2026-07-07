# API-Only Egress Mode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a `--network api-only` mode so an agent keeps access to the Anthropic API (and the git host it clones from) through a VM-side allowlisting proxy, while the container's own internet egress is default-dropped — so a malicious file in the workspace cannot open an arbitrary socket or resolve arbitrary DNS. This is not a complete phone-home block: the model conversation itself and the allowlisted GitHub clone path remain reachable and are real, if narrow, residual channels (see docs/security.md for the full analysis).

**Architecture:** api-only containers attach to a managed bridge whose interface name uses a distinct `bti` prefix. `vm/setup.sh` runs a `tinyproxy` instance in the VM (allowlist, default-deny) and prepends one `iptables` rule that REJECTs *all* forwarded egress from `bti+` interfaces. The container reaches the proxy over the host-gateway address (an INPUT-path packet, unaffected by the FORWARD-chain drop) via injected `HTTPS_PROXY`/`HTTP_PROXY` env. All external DNS and direct sockets from the container are dropped; only the proxy, resolving names VM-side, can reach the network, and only for allowlisted hosts.

**Tech Stack:** Go 1.x (cobra CLI, `pkg/vmexec.FakeExecutor` for tests), Alpine 3.22 VM, `iptables`, `tinyproxy`, Docker bridge networking.

## Global Constraints

- Language/tooling: Go for host CLI (`cmd/`, `pkg/`); bash for VM/container scripts (`vm/setup.sh`, `entrypoint.sh`) — Go is not installed in the VM or containers. Copied verbatim from CLAUDE.md conventions.
- All container/network names go through `pkg/validate` before use.
- All Go tests use `vmexec.FakeExecutor` — no real Apple container/Docker in unit tests.
- New network mode string is exactly `api-only` (matches the existing `managed`/`none` convention in `pkg/policy`).
- api-only bridge interface prefix is exactly `bti`. This is safe because managed bridges are `bt` + hex (sha1), and hex digits are `[0-9a-f]` — never `i` — so `bti+` iptables matches api-only bridges only and never a managed `bt` bridge.
- Proxy listens on `0.0.0.0:8119` inside the VM. Container reaches it as host `berth-proxy` mapped to Docker `host-gateway`.
- Proxy is default-deny; the seed allowlist contains exactly: `api.anthropic.com`, `statsig.anthropic.com`, `sentry.io`, `github.com`, `codeload.github.com`, `objects.githubusercontent.com`. (Anthropic API + Claude Code telemetry + HTTPS git clone from GitHub.)
- api-only requires HTTPS repo URLs. SSH clone (port 22) is dropped by design; do not attempt to support it in this phase.

---

### Task 1: api-only managed network creation in `pkg/docker`

**Files:**
- Modify: `pkg/docker/network.go`
- Test: `pkg/docker/network_test.go`

**Interfaces:**
- Consumes: `ManagedNetworkName(containerName string) string` (existing — returns `<name>-net`), `labels.App`, `labels.AppValue`, `validate.NetworkName`.
- Produces:
  - `APIOnlyBridgeName(containerName string) string` — returns `bti` + 10 hex chars (`sha1(containerName)[:5]`).
  - `CreateAPIOnlyNetwork(ctx context.Context, exec vmexec.Executor, containerName string) (string, error)` — creates a bridge network named `ManagedNetworkName(containerName)` whose bridge interface is `APIOnlyBridgeName(containerName)`; returns the network name.
  - `PrepareNetwork` gains a third mode: when `customNetwork == "api-only"` it returns `(ManagedNetworkName(containerName), "api-only", nil)` (dry-run) or creates the api-only network and returns `(name, "api-only", err)`.

- [ ] **Step 1: Write the failing test**

Add to `pkg/docker/network_test.go`:

```go
func TestAPIOnlyBridgeNameHasBtiPrefix(t *testing.T) {
	name := APIOnlyBridgeName("my-forensic-agent")
	if !strings.HasPrefix(name, "bti") {
		t.Fatalf("api-only bridge %q must start with bti", name)
	}
	if len(name) > 15 {
		t.Fatalf("bridge name %q exceeds Linux IFNAMSIZ (15)", name)
	}
	// Deterministic for a given container name.
	if name != APIOnlyBridgeName("my-forensic-agent") {
		t.Fatal("bridge name must be deterministic")
	}
	// Managed bridge for the same name must NOT collide with the bti prefix.
	if strings.HasPrefix(ManagedBridgeName("my-forensic-agent"), "bti") {
		t.Fatal("managed bridge unexpectedly starts with bti")
	}
}

func TestPrepareNetworkAPIOnly(t *testing.T) {
	fake := vmexec.NewFakeExecutor()
	name, mode, err := PrepareNetwork(context.Background(), fake, "agent1", "api-only", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != "api-only" {
		t.Fatalf("mode = %q, want api-only", mode)
	}
	if name != ManagedNetworkName("agent1") {
		t.Fatalf("network name = %q, want %q", name, ManagedNetworkName("agent1"))
	}
	created := fake.CommandsMatching("docker network create")
	if len(created) != 1 {
		t.Fatalf("expected one network create, got %d", len(created))
	}
	if !strings.Contains(created[0], "com.docker.network.bridge.name="+APIOnlyBridgeName("agent1")) {
		t.Fatalf("create did not pin bti bridge name: %s", created[0])
	}
}

func TestPrepareNetworkAPIOnlyDryRun(t *testing.T) {
	fake := vmexec.NewFakeExecutor()
	name, mode, err := PrepareNetwork(context.Background(), fake, "agent1", "api-only", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != "api-only" || name != ManagedNetworkName("agent1") {
		t.Fatalf("dry-run returned name=%q mode=%q", name, mode)
	}
	if len(fake.CommandsMatching("docker network create")) != 0 {
		t.Fatal("dry-run must not create a network")
	}
}
```

If `strings` is not already imported in the test file, add it to the import block.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/docker/ -run 'APIOnly|PrepareNetworkAPIOnly' -v`
Expected: FAIL — `undefined: APIOnlyBridgeName` and `CreateAPIOnlyNetwork` / api-only branch missing.

- [ ] **Step 3: Write minimal implementation**

In `pkg/docker/network.go`, add after `ManagedBridgeName`:

```go
func APIOnlyBridgeName(containerName string) string {
	sum := sha1.Sum([]byte(containerName))
	return fmt.Sprintf("bti%x", sum[:5])
}

func CreateAPIOnlyNetwork(ctx context.Context, exec vmexec.Executor, containerName string) (string, error) {
	netName := ManagedNetworkName(containerName)
	_, err := exec.Run(ctx, "docker", "network", "create",
		"--driver", "bridge",
		"--opt", "com.docker.network.bridge.name="+APIOnlyBridgeName(containerName),
		"--label", fmt.Sprintf("%s=%s", labels.App, labels.AppValue),
		netName)
	if err != nil {
		return "", fmt.Errorf("create api-only network %s: %w", netName, err)
	}
	return netName, nil
}
```

In `PrepareNetwork`, add the api-only branch **before** the `validate.NetworkName` custom-network branch (so `api-only` is never treated as a user-supplied Docker network name):

```go
	if customNetwork == "api-only" {
		if dryRun {
			return ManagedNetworkName(containerName), "api-only", nil
		}
		name, err := CreateAPIOnlyNetwork(ctx, exec, containerName)
		return name, "api-only", err
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/docker/ -run 'APIOnly|PrepareNetworkAPIOnly' -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Run the full docker package to check for regressions**

Run: `go test ./pkg/docker/`
Expected: `ok  github.com/0x666c6f/berth/pkg/docker`

- [ ] **Step 6: Commit**

```bash
git add pkg/docker/network.go pkg/docker/network_test.go
git commit -m "feat(docker): add api-only managed network with bti bridge prefix"
```

---

### Task 2: policy constant for the api-only network mode

**Files:**
- Modify: `pkg/policy/policy.go:14-20`
- Test: `pkg/policy/policy_test.go`

**Interfaces:**
- Consumes: existing `NetworkManaged`, `NetworkNone` constants and `Enforce`/`enforce` (network is checked via `containsAllowed(rule.Allow.Networks, req.Network)` — a free-form string list, so no allowlist change is needed for enforcement).
- Produces: `NetworkAPIOnly = "api-only"` constant, used by `cmd/berth` and risk wiring.

- [ ] **Step 1: Write the failing test**

Add to `pkg/policy/policy_test.go`:

```go
func TestNetworkAPIOnlyConstant(t *testing.T) {
	if NetworkAPIOnly != "api-only" {
		t.Fatalf("NetworkAPIOnly = %q, want api-only", NetworkAPIOnly)
	}
}

func TestEnforceAllowsAPIOnlyWhenListed(t *testing.T) {
	allow := []string{NetworkAPIOnly}
	rules := []RuleSet{{Source: "test", Allow: AllowRules{Networks: &allow}}}
	err := Enforce(rules, SpawnRequest{DockerMode: DockerModeOff, Network: NetworkAPIOnly})
	if err != nil {
		t.Fatalf("api-only should be allowed: %v", err)
	}
}

func TestEnforceDeniesAPIOnlyWhenNotListed(t *testing.T) {
	allow := []string{NetworkManaged}
	rules := []RuleSet{{Source: "test", Allow: AllowRules{Networks: &allow}}}
	err := Enforce(rules, SpawnRequest{DockerMode: DockerModeOff, Network: NetworkAPIOnly})
	if err == nil {
		t.Fatal("expected api-only to be denied when not in allow list")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/policy/ -run APIOnly -v`
Expected: FAIL — `undefined: NetworkAPIOnly`.

- [ ] **Step 3: Write minimal implementation**

In `pkg/policy/policy.go`, add to the `const` block:

```go
	NetworkAPIOnly       = "api-only"
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/policy/ -run APIOnly -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add pkg/policy/policy.go pkg/policy/policy_test.go
git commit -m "feat(policy): add api-only network mode constant"
```

---

### Task 3: risk notice for api-only mode

**Files:**
- Modify: `pkg/risk/risk.go` (the notice-building function, near the existing `input.NetworkMode == "custom"` block, ~line 63)
- Test: `pkg/risk/risk_test.go`

**Interfaces:**
- Consumes: `risk.SpawnInput` (has `NetworkMode string`), `risk.Notice{Flag, Summary string}`, `risk.SpawnNotices(SpawnInput) []Notice`.
- Produces: when `input.NetworkMode == "api-only"`, `SpawnNotices` includes a `Notice{"--network api-only", "egress restricted to an allowlisted VM proxy; direct internet and DNS are dropped"}`.

Note the polarity: api-only is *safer* than default, but it is still a non-default posture the operator should see confirmed, so it emits an informational notice (consistent with how the security summary lists posture). It is not a "danger" widener.

- [ ] **Step 1: Write the failing test**

Add to `pkg/risk/risk_test.go` (match the existing test style in that file):

```go
func TestSpawnNoticesAPIOnly(t *testing.T) {
	notices := SpawnNotices(SpawnInput{NetworkMode: "api-only"})
	found := false
	for _, n := range notices {
		if n.Flag == "--network api-only" {
			found = true
			if !strings.Contains(n.Summary, "allowlisted") {
				t.Fatalf("summary should mention allowlist: %q", n.Summary)
			}
		}
	}
	if !found {
		t.Fatal("expected an api-only notice")
	}
}
```

Ensure `strings` is imported in the test file.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pkg/risk/ -run APIOnly -v`
Expected: FAIL — no notice emitted.

- [ ] **Step 3: Write minimal implementation**

In `pkg/risk/risk.go`, immediately after the existing `if input.NetworkMode == "custom" { ... }` block:

```go
	if input.NetworkMode == "api-only" {
		notices = append(notices, Notice{"--network api-only", "egress restricted to an allowlisted VM proxy; direct internet and DNS are dropped"})
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./pkg/risk/ -run APIOnly -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/risk/risk.go pkg/risk/risk_test.go
git commit -m "feat(risk): surface api-only network posture in spawn notices"
```

---

### Task 4: spawn wiring — proxy env + host-gateway alias for api-only

**Files:**
- Modify: `cmd/berth/spawn.go` (the function that builds the docker run command and appends env/labels — around the `AppendRuntimeHardening` call at ~line 608 and the env block ~line 598-624)
- Test: `cmd/berth/spawn_test.go` (or the existing spawn-parity test file in `cmd/berth`)

**Interfaces:**
- Consumes: `resolved.NetworkMode` (set by `prepareSpawnNetwork` from Task 1's `PrepareNetwork`), `docker.DockerRunCmd.AddEnv`, `docker.DockerRunCmd.AddFlag`, `policy.NetworkAPIOnly`.
- Produces: for an api-only spawn, the generated `docker run` argv contains `--add-host berth-proxy:host-gateway` and env `HTTPS_PROXY=http://berth-proxy:8119`, `HTTP_PROXY=http://berth-proxy:8119`, `NO_PROXY=localhost,127.0.0.1,::1`, `NODE_USE_ENV_PROXY=1`, and `BERTH_NETWORK_MODE=api-only`.

Find the helper that assembles the run command (the one containing the `AppendRuntimeHardening(cmd, ...)` call and the `cmd.AddEnv("AGENT_TYPE", ...)` lines). Locate the exact function name with:

```bash
rg -n "AppendRuntimeHardening|func .*DockerRunCmd|cmd.AddEnv\(\"AGENT_TYPE\"" cmd/berth/spawn.go
```

- [ ] **Step 1: Write the failing test**

Add to the spawn test file in `cmd/berth`. Use whatever constructor the existing spawn tests use to build a `SpawnOpts` + `spawnResolved` and render the command to a string; mirror the nearest existing test. Skeleton:

```go
func TestSpawnAPIOnlyInjectsProxy(t *testing.T) {
	cmd := buildSpawnRunCmdForTest(t, SpawnOpts{
		AgentType: "claude",
		Network:   "api-only",
	}, spawnResolved{
		ContainerName: "forensic1",
		NetworkName:   "forensic1-net",
		NetworkMode:   "api-only",
		Memory:        "8g",
		CPUs:          "4",
		PIDsLimit:     512,
	})
	s := cmd.String()
	for _, want := range []string{
		"--add-host berth-proxy:host-gateway",
		"HTTPS_PROXY=http://berth-proxy:8119",
		"HTTP_PROXY=http://berth-proxy:8119",
		"NODE_USE_ENV_PROXY=1",
		"BERTH_NETWORK_MODE=api-only",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("api-only run cmd missing %q\ngot: %s", want, s)
		}
	}
}

func TestSpawnManagedDoesNotInjectProxy(t *testing.T) {
	cmd := buildSpawnRunCmdForTest(t, SpawnOpts{
		AgentType: "claude",
	}, spawnResolved{
		ContainerName: "agent1",
		NetworkName:   "agent1-net",
		NetworkMode:   "managed",
		Memory:        "8g", CPUs: "4", PIDsLimit: 512,
	})
	if strings.Contains(cmd.String(), "HTTPS_PROXY") {
		t.Error("managed mode must not inject a proxy")
	}
}
```

`buildSpawnRunCmdForTest` should call the same command-builder helper the real spawn path uses. If no such test helper exists yet, add a thin one in the test file that calls the builder function you located in the pre-step and returns the `*docker.DockerRunCmd`. If the builder needs a `context.Context` and `vmexec.Executor`, pass `context.Background()` and `vmexec.NewFakeExecutor()`.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/berth/ -run 'SpawnAPIOnly|SpawnManagedDoesNotInjectProxy' -v`
Expected: FAIL — env/flags absent.

- [ ] **Step 3: Write minimal implementation**

In the run-command builder in `cmd/berth/spawn.go`, right after the `AppendRuntimeHardening(cmd, docker.HardeningOpts{...})` call, add:

```go
	if resolved.NetworkMode == policy.NetworkAPIOnly {
		cmd.AddFlag("--add-host", "berth-proxy:host-gateway")
		const proxyURL = "http://berth-proxy:8119"
		cmd.AddEnv("HTTPS_PROXY", proxyURL)
		cmd.AddEnv("HTTP_PROXY", proxyURL)
		cmd.AddEnv("NO_PROXY", "localhost,127.0.0.1,::1")
		cmd.AddEnv("NODE_USE_ENV_PROXY", "1")
	}
	cmd.AddEnv("BERTH_NETWORK_MODE", resolved.NetworkMode)
```

Confirm `policy` is imported in `spawn.go` (it is used elsewhere in `cmd/berth`; add the import if this file does not already have it).

Note: `BERTH_NETWORK_MODE` was previously never set by spawn (the entrypoint defaulted it to `managed`). Setting it unconditionally here also fixes the preamble's network line for `none`/`custom` modes — intended.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/berth/ -run 'SpawnAPIOnly|SpawnManagedDoesNotInjectProxy' -v`
Expected: PASS.

- [ ] **Step 5: Run the full cmd/berth suite for regressions**

Run: `go test ./cmd/berth/`
Expected: `ok`. In particular the spawn-parity test (retry reconstruction) must still pass — if it asserts an exact env set, add `BERTH_NETWORK_MODE` to its expected values.

- [ ] **Step 6: Update the `--network` flag help and usage example**

In `cmd/berth/spawn.go`, update both `--network` flag registrations (lines ~138 and ~162) to mention api-only:

```go
	f.StringVar(&spawnOpts.Network, "network", "", "Network mode: 'api-only' (allowlisted proxy egress, safe for untrusted files), a named Docker network like agent-isolated for no internet, or default dedicated bridge")
```

Apply the identical string to the `rf.StringVar(...)` registration.

- [ ] **Step 7: Commit**

```bash
git add cmd/berth/spawn.go cmd/berth/spawn_test.go
git commit -m "feat(spawn): route api-only agents through the VM allowlist proxy"
```

---

### Task 5: VM-side proxy + api-only iptables drop in `vm/setup.sh`

**Files:**
- Modify: `vm/setup.sh` (package install block ~line 185-213; egress guardrails block ~line 288-318)
- Test: manual VM verification (bash/iptables/tinyproxy cannot be exercised by `FakeExecutor`; verified live in Task 7)

**Interfaces:**
- Consumes: existing `as_root`, `step` helpers; the existing `BERTH_EGRESS` chain build.
- Produces: a running `tinyproxy` on `0.0.0.0:8119` with default-deny filtering against `/etc/tinyproxy/allowlist`, and a `BERTH_EGRESS` rule at position 1 that REJECTs all forwarded egress from `bti+` interfaces.

- [ ] **Step 1: Add tinyproxy to the Alpine package install**

In the Alpine branch of the package install (the `apk add --no-cache \` list that already contains `socat`), add `tinyproxy` on its own continuation line:

```sh
      socat \
      tinyproxy \
```

For the Debian/`apt-get` branch, append `tinyproxy` to the install line:

```sh
    as_root apt-get install -y -qq docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin socat tinyproxy
```

`tinyproxy` is in Alpine `community`. If `apk add tinyproxy` fails because community is disabled, enable it first:

```sh
    as_root sh -c 'grep -q "/community" /etc/apk/repositories || echo "https://dl-cdn.alpinelinux.org/alpine/v3.22/community" >> /etc/apk/repositories'
    as_root apk update
```

Add that guard immediately before the `apk add` in the Alpine branch.

- [ ] **Step 2: Write the tinyproxy config and allowlist, then (re)start it**

Add a new `step` block **after** the "Docker is ready" confirmation and **before** the egress-guardrails step (so the proxy exists before we lock down the bridge). Insert:

```sh
# =============================================================================
# API-only egress proxy (tinyproxy, default-deny allowlist)
# =============================================================================
step 6 "Configuring api-only egress proxy..."
as_root mkdir -p /etc/tinyproxy

as_root tee /etc/tinyproxy/allowlist >/dev/null <<'ALEOF'
^api\.anthropic\.com$
^statsig\.anthropic\.com$
^sentry\.io$
^github\.com$
^codeload\.github\.com$
^objects\.githubusercontent\.com$
ALEOF

as_root tee /etc/tinyproxy/tinyproxy.conf >/dev/null <<'TPEOF'
User tinyproxy
Group tinyproxy
Port 8119
Listen 0.0.0.0
Timeout 600
LogLevel Warning
MaxClients 100
Filter "/etc/tinyproxy/allowlist"
FilterDefaultDeny Yes
FilterExtended On
FilterCaseSensitive Off
# Allow CONNECT (HTTPS) to standard TLS port only; hosts still filtered above.
ConnectPort 443
TPEOF

# Restart tinyproxy with our config (nohup: the VM has no assumed service manager here).
as_root pkill -x tinyproxy >/dev/null 2>&1 || true
as_root sh -c 'nohup tinyproxy -c /etc/tinyproxy/tinyproxy.conf >/var/log/tinyproxy.log 2>&1 &'
```

Notes for the implementer:
- `tinyproxy -c` runs in the foreground unless the config sets no `PidFile`; the `nohup ... &` backgrounds it, mirroring how `dockerd` is started earlier in this script.
- The `tinyproxy` user/group ships with the package. If `pkill`/user is missing on the Debian path, that's fine — Alpine is the supported VM.
- `FilterDefaultDeny Yes` + `Filter` means only hosts matching a line in the allowlist are reachable; everything else returns 403 from the proxy. `FilterExtended On` uses POSIX extended regex, so `\.` is a literal dot.

- [ ] **Step 3: Prepend the api-only forward-drop iptables rule**

In the egress-guardrails block, the chain is flushed with `iptables -F BERTH_EGRESS` and then rules are appended in order. **After** the flush and **before** the existing `iptables -A BERTH_EGRESS -m conntrack --ctstate ESTABLISHED,RELATED -j RETURN` line, insert:

```sh
# api-only bridges (bti*): drop ALL forwarded egress. The only reachable path
# is the VM-local proxy (host-gateway), which is INPUT-path, not FORWARD, so it
# is unaffected by this rule. This kills direct internet, external DNS, and C2.
as_root iptables -A BERTH_EGRESS -i 'bti+' -j REJECT
```

Because this is appended first (position 1) it wins before the permissive `bt+` port-allow rules that follow. Managed `bt` bridges never match `bti+` (hex third char), so they are unaffected.

- [ ] **Step 4: Renumber the egress step label if needed**

The egress-guardrails `step` call is currently `step 5`. If your inserted proxy block is `step 6`, bump the egress block to `step 7` and any later `step` calls accordingly, keeping the sequence monotonic. (Cosmetic — `step` only prints progress.)

- [ ] **Step 5: Shellcheck / syntax check**

Run: `bash -n vm/setup.sh`
Expected: no output (syntax OK). If `shellcheck` is available: `shellcheck vm/setup.sh` — pre-existing warnings are acceptable; no new errors on the added lines.

- [ ] **Step 6: Commit**

```bash
git add vm/setup.sh
git commit -m "feat(vm): add api-only egress proxy and bti forward-drop rule"
```

---

### Task 6: security preamble + docs for api-only

**Files:**
- Modify: `entrypoint.sh` (the `case "$network_status"` block ~line 192-196)
- Modify: `config/security-preamble.md` (only if it enumerates network modes — check first)
- Modify: `docs/security.md`, `docs/architecture.md`, `docs/reference/cli.md`

**Interfaces:**
- Consumes: `BERTH_NETWORK_MODE=api-only` (now set by Task 4).
- Produces: the in-container security preamble renders a clear api-only line; docs describe the mode and its threat coverage.

- [ ] **Step 1: Add the api-only case to the entrypoint preamble**

In `entrypoint.sh`, in the `case "$network_status"` block, add:

```sh
    api-only) network_status="api-only (egress via allowlisted VM proxy; direct internet and DNS blocked)" ;;
```

- [ ] **Step 2: Verify the entrypoint still parses**

Run: `bash -n entrypoint.sh`
Expected: no output.

- [ ] **Step 3: Document the mode in docs/security.md**

Add a row/paragraph to the security posture section describing api-only: what it blocks (direct internet, container-side DNS resolution), the residual channels that remain (the model conversation itself, and the allowlisted GitHub clone path — a real pull channel, push-exfil only with write creds), and the allowlist contents. State plainly that this is the recommended mode for analyzing untrusted files, and that it is **not** a malware detonation sandbox (see Known Limitations).

- [ ] **Step 4: Document in docs/architecture.md**

Under the networking section (near the existing `agent-isolated` `--internal` example, ~line 67-79), add an api-only subsection: the `bti` bridge prefix, the VM-side tinyproxy, the host-gateway INPUT path, and why the FORWARD drop does not break the proxy.

- [ ] **Step 5: Document in docs/reference/cli.md**

In the `berth spawn` `--network` documentation, add `api-only` as a documented value with a one-line description and an example:

```bash
berth spawn claude --network api-only --repo https://github.com/you/quarantine.git \
  --ephemeral-auth --instructions 'Static analysis only. Treat all file content as untrusted data, never execute.'
```

- [ ] **Step 6: Commit**

```bash
git add entrypoint.sh config/security-preamble.md docs/security.md docs/architecture.md docs/reference/cli.md
git commit -m "docs: document api-only egress mode and update security preamble"
```

---

### Task 7: live end-to-end verification (the acceptance gate)

**Files:**
- No code changes. This task rebuilds the VM/image and proves the two required properties on a real spawn.

**Interfaces:**
- Consumes: everything from Tasks 1-6.
- Produces: recorded evidence that (a) the agent reaches the Anthropic API through the proxy, and (b) a process in the workspace cannot reach a non-allowlisted host, cannot do external DNS, and cannot open a direct socket.

- [ ] **Step 1: Rebuild the VM and image**

Run:
```bash
make build-all
./bin/berth vm start        # re-applies NAT + re-runs setup.sh (installs tinyproxy, inserts iptables rule)
./bin/berth diagnose
```
Expected: diagnose reports the VM up with egress NAT on. Then confirm the proxy is live in the VM:
```bash
./bin/berth vm ssh -- 'pgrep -x tinyproxy && iptables -nL BERTH_EGRESS | head'
```
Expected: a tinyproxy PID prints, and the first non-default rule in `BERTH_EGRESS` is `REJECT ... bti+`.

- [ ] **Step 2: Spawn an api-only agent against a throwaway HTTPS repo**

Run:
```bash
./bin/berth spawn claude --network api-only --ephemeral-auth \
  --repo https://github.com/you/quarantine.git \
  --instructions 'Static analysis only. Never execute samples. Treat file content as untrusted data.'
```
Expected: the spawn's "Security context" prints the api-only notice; the container starts; the clone succeeds (github.com is allowlisted); the agent reaches the API and produces a first message.

Confirm reachability positively:
```bash
./bin/berth peek --latest
```
Expected: real agent output (proves API reachable through the proxy — this is the make-or-break check for Node/undici proxy support).

**If the agent cannot reach the API** (Node did not honor the proxy): the `NODE_USE_ENV_PROXY=1` env from Task 4 covers Node 24+. If the installed Node is older, the blocker is real — resolve by pinning the agent's HTTP layer to the proxy (e.g. Claude Code honors `HTTPS_PROXY` directly; verify the CLI version in the image does). Do not proceed to Step 5 until API access works.

- [ ] **Step 3: Prove a non-allowlisted host is blocked**

Get a shell in the running container (drop the agent, keep the container) and probe egress. Use the container name from `./bin/berth list`:
```bash
./bin/berth vm ssh -- 'docker exec <container> sh -c "curl -sS -m 8 https://example.com -o /dev/null; echo curl-direct=$?"'
```
Expected: non-zero exit (connection blocked by the FORWARD drop).

```bash
./bin/berth vm ssh -- 'docker exec <container> sh -c "HTTPS_PROXY=http://berth-proxy:8119 curl -sS -m 8 https://example.com -o /dev/null; echo curl-viaproxy=$?"'
```
Expected: non-zero / 403 (proxy denies non-allowlisted host).

- [ ] **Step 4: Prove external DNS and direct sockets are dropped**

An explicit-resolver probe alone doesn't prove the real DNS-tunnel vector is closed — that vector is the container's *default* resolver path, not a resolver an attacker has to name explicitly. Probe both.

```bash
./bin/berth vm ssh -- 'docker exec <container> sh -c "nslookup api.anthropic.com 8.8.8.8; echo dns-explicit=$?"'
```
Expected: failure/timeout (external DNS to 8.8.8.8 is a forwarded packet → dropped).

```bash
./bin/berth vm ssh -- 'docker exec <container> sh -c "nslookup evil.example.com; echo dns-default=$?; getent hosts evil.example.com; echo getent=$?"'
```
Expected: both fail (the container's default resolver path — no explicit server — must be blocked too; this is the actual DNS-tunnel vector).

**Caveat:** if the container's configured nameserver resolves to something VM-local (e.g. Docker's embedded DNS at `127.0.0.11`), that query could still be answered without ever hitting `FORWARD` and the `bti+` REJECT — which would mean the DNS-block claim in the docs is false and needs revisiting. Confirm the default-resolver probe above actually fails before treating "DNS blocked" as proven.

```bash
./bin/berth vm ssh -- 'docker exec <container> sh -c "curl -sS -m 8 https://api.anthropic.com/v1/models -o /dev/null; echo direct-api=$?"'
```
Expected: failure (direct, non-proxied connection even to an allowlisted host is dropped — only the proxy may egress).

- [ ] **Step 5: Prove the allowlisted API host works only through the proxy**

```bash
./bin/berth vm ssh -- 'docker exec <container> sh -c "HTTPS_PROXY=http://berth-proxy:8119 curl -sS -m 10 -o /dev/null -w %{http_code} https://api.anthropic.com/v1/models; echo"'
```
Expected: an HTTP status (401/200/etc.) — a *response*, proving the proxy reaches the allowlisted host. Contrast with Step 4's direct failure.

- [ ] **Step 6: Record results and clean up**

Write the six probe results into the PR description (or `docs/security.md` verification note). Then:
```bash
./bin/berth stop --latest
```

- [ ] **Step 7: Commit any doc/evidence updates**

```bash
git add -A
git commit -m "test: record api-only egress verification results"
```

---

## Self-Review Notes

- **Spec coverage:** VM proxy (Task 5), iptables default-drop for api-only bridges (Task 5), proxy env injection so the agent keeps API access (Task 4), network-mode plumbing (Tasks 1-2), operator visibility (Tasks 3, 6), and a live acceptance gate proving both directions (Task 7). All covered.
- **Key risk called out inline:** Node/undici honoring `HTTPS_PROXY` is the single make-or-break behavior; Task 7 Step 2 gates on it and names the fallback rather than discovering it at the end.
- **No collision:** managed `bt`+hex bridges never match `bti+` (hex has no `i`); documented as a Global Constraint and asserted in Task 1's test.
- **Cleanup unchanged:** api-only networks reuse `ManagedNetworkName`, so `RemoveManagedNetwork(ManagedNetworkName(name))` in `lifecycle.go` already tears them down — no new cleanup code needed.

## Out of Scope (later phases)

- Read-only, noexec `--evidence <dir>` sample mount + sha256 chain-of-custody (Phase 2).
- Forensic tool image variant and `forensic-triage` template (Phase 3).
- Dynamic detonation in a separate disposable VM (Phase 4).
- Per-spawn custom allowlists (`--allow-host`) — Phase 1 ships a fixed seed allowlist; make it configurable only if a real need appears (YAGNI).
