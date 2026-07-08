# Phase 4 Implementation: `detonate` harness (Tier A) — Plan

> Execute subagent-driven. Steps use checkbox syntax. Implements the strategy in `2026-07-07-phase4-detonation-strategy.md`.

**Goal:** A real, tested `detonate` CLI (a SEPARATE binary from `berth`) that orchestrates the Tier-A disposable-VM detonation lifecycle with the design's hard boundaries **enforced in code**: routing by sample architecture, an isolated-network guard, clone-from-immutable-golden + destroy (no reuse), offline sample injection, out-of-band capture, and a sha256 chain of custody. It ships **no golden images and no malware**, and **fails closed** when the environment (golden VM, isolated no-uplink network) isn't provisioned — so it is safe to build and merge now; the operator supplies the golden + sacrificial boundary to actually detonate.

**Architecture:** Go, mirroring berth's patterns. A `pkg/detonate` package holds the pure lifecycle state machine + containment guards + routing (fully unit-tested). A `Runner` interface abstracts the VM/network side effects (clone/configure-net/inject/boot/capture/poweroff/destroy) with a `FakeRunner` for tests and a `TartRunner` that shells out to `tart`/`qemu`/`hdiutil` (written, guard-gated, not live-exercised here). A `cmd/detonate` cobra CLI exposes `route|create|inject|run|collect|destroy`, gates `run` behind a typed confirmation, and records custody via the existing `pkg/audit`.

**Tech Stack:** Go (cobra, `crypto/sha256`, `pkg/audit`, `pkg/validate` reuse), a Runner interface with Fake (mirrors `pkg/vmexec.Executor`).

## Global Constraints

- `detonate` is a SEPARATE binary (`cmd/detonate/`, built to `bin/detonate`). It is NOT a `berth` subcommand — keeping berth's blast-radius boundary intact is the whole point.
- The tool ships NO VM images and NO samples. It must **fail closed** (clear error, non-zero exit) when: the named golden doesn't exist, the network config is not provably isolated, or a state precondition is unmet. Never "make it run" by relaxing a guard.
- Hard boundaries enforced in code (from the strategy doc §8): (1) `run` refuses unless the network attachment is isolated — no NAT, no bridge, no uplink; (2) no reuse — the only lifecycle is clone-golden → … → destroy-clone; there is NO restore/revert verb; (3) injection is offline only (guest powered off); (4) `collect` refuses unless the guest is powered off; (5) auto-destroy on completion AND on error; (6) `run` requires an explicit typed confirmation; (7) golden is treated read-only (never booted/mutated by a run).
- Chain of custody: sha256 the sample at inject and every artifact at collect; append to an append-only audit log (reuse `pkg/audit.Logger`), operation class `detonate-*`.
- Routing (strategy §1/§9): from a berth forensic JSON's arch/format field → TierA (arm/script/elf/macho) | TierB (x86-64 windows PE → self-hosted cloud) | TierC (fallback) | Refuse (unknown). Only TierA is orchestrated locally; TierB/TierC print guidance and do NOT attempt local detonation.
- No unit test may boot a VM or run a sample. All side effects go through the `Runner` interface; tests use `FakeRunner`. The `TartRunner` real impl is written but exercised only by the operator with a provisioned environment.

---

### Task 1: `pkg/detonate` — routing + run-state model + containment guards

**Files:**
- Create: `pkg/detonate/detonate.go`, `pkg/detonate/guard.go`
- Test: `pkg/detonate/detonate_test.go`

**Interfaces (produce):**
- `type Tier int` with `TierLocalARM, TierCloudX86, TierCommercial, TierRefuse` + `String()`.
- `type StaticFindings struct { SHA256 string; FileType string; Arch string; Format string }` and `func Route(f StaticFindings) (Tier, string)` — returns the tier and a human reason. Rules: `arch` in {arm64, aarch64} OR format in {script, macro, elf-arm, macho} → TierLocalARM; format=pe && arch in {x86-64, amd64, i386} → TierCloudX86 (reason: "x86 Windows PE — no faithful local detonation on Apple Silicon; use a self-hosted cloud x86 sandbox"); empty/unknown → TierRefuse (reason names what's missing). Provide a small decision table in a comment.
- `type State int` with ordered `StateCreated, StateInjected, StateDetonated, StateCollected, StateDestroyed`; `func (s State) CanTransition(to State) bool` — only forward, single-step, and `StateDestroyed` is terminal from any state (destroy is always allowed). No backward transitions (no reuse/revert).
- `type NetAttachment struct { Mode string; HasUplink bool }` and `func ValidateIsolated(n NetAttachment) error` — returns an error unless `Mode == "isolated"` (or "host-none"/"fakenet") AND `!HasUplink`. Reject `Mode` in {nat, bridge, shared, host} or `HasUplink==true`, with a message naming the violation. This is the load-bearing containment guard.

- [ ] **Step 1: Failing tests** — `Route` for each tier (arm→local, x86 PE→cloud, unknown→refuse); `CanTransition` (forward ok; backward rejected; destroy always ok; destroyed terminal); `ValidateIsolated` (isolated+no-uplink ok; nat/bridge/shared/host rejected; isolated+uplink rejected). Real assertions.
- [ ] **Step 2: RED** — `go test ./pkg/detonate/ -v`
- [ ] **Step 3: Implement** the types/functions above. Stdlib only.
- [ ] **Step 4: GREEN** — `go test ./pkg/detonate/`
- [ ] **Step 5: Commit** `feat(detonate): routing, lifecycle state model, and isolated-network guard`

---

### Task 2: `Runner` interface + FakeRunner + TartRunner skeleton

**Files:**
- Create: `pkg/detonate/runner.go` (interface + FakeRunner), `pkg/detonate/tart.go` (TartRunner)
- Test: `pkg/detonate/runner_test.go`

**Interfaces (produce):**
- `type Runner interface { GoldenExists(ctx, golden string) (bool, error); Clone(ctx, golden, run string) error; ConfigureIsolatedNet(ctx, run string, gw string) (NetAttachment, error); InjectOffline(ctx, run, samplePath string) error; Run(ctx, run string, timeout time.Duration) error; Collect(ctx, run, destDir string) ([]string, error); PoweredOff(ctx, run string) (bool, error); Destroy(ctx, run string) error }`
- `FakeRunner` — records calls, configurable return values/errors (mirror `pkg/vmexec.FakeExecutor`: a call log + `SetX` setters). Used by all tests.
- `TartRunner` — real impl shelling out via an injected command runner (reuse a thin `exec.Command` wrapper): `GoldenExists` = `tart list` parse; `Clone` = `tart clone`; `InjectOffline` = attach a read-only sample ISO/dmg built with `hdiutil` (offline — guest not running); `Run` = `tart run` with the isolated net + timeout; `Collect` = pull the artifact dir + poweroff-state check; `Destroy` = `tart delete`. Each method that performs egress-capable actions MUST first assert the run's `NetAttachment` passed `ValidateIsolated`. **Do not invent commands you can't name** — where a Tart capability is uncertain, leave a clearly-commented `// operator-provisioned:` stub returning a "not configured" error (fail closed), not a fake success.

- [ ] **Step 1: Failing tests** — with `FakeRunner`: `GoldenExists`/`Clone`/`Destroy` call recording; assert the higher-level lifecycle (Task 3) can drive the fake. Keep `TartRunner` tested only for pure arg-building where feasible (e.g. a helper that builds the `tart clone` argv), NOT live.
- [ ] **Step 2: RED**
- [ ] **Step 3: Implement** interface + FakeRunner + TartRunner (real args; fail-closed stubs where a capability needs operator provisioning).
- [ ] **Step 4: GREEN** — `go test ./pkg/detonate/`
- [ ] **Step 5: Commit** `feat(detonate): Runner interface, FakeRunner, and Tart-backed runner`

---

### Task 3: `cmd/detonate` CLI — the enforced state machine

**Files:**
- Create: `cmd/detonate/main.go`, `cmd/detonate/commands.go`
- Test: `cmd/detonate/commands_test.go`

**Interfaces (produce):** a cobra CLI with verbs, each wired to `pkg/detonate` + a `Runner`, using `FakeRunner` in tests:
- `detonate route <static.json>` — parse berth forensic JSON → `Route` → print tier + reason; exit non-zero on TierRefuse. TierCloudX86/Commercial print guidance and DO NOT orchestrate locally.
- `detonate create <run> --golden <name>` — `GoldenExists` (fail closed if missing) → `Clone`. Audit `detonate-create`.
- `detonate inject <run> <sample>` — assert state Created (guest not booted) → sha256 the sample → audit `detonate-inject` with the hash BEFORE the Docker/VM work → `InjectOffline`.
- `detonate run <run> --timeout 180s [--yes]` — `ConfigureIsolatedNet` → `ValidateIsolated` (fail closed if not isolated) → require a typed confirmation ("type: detonate <run>") unless `--yes` AND `DETONATE_I_UNDERSTAND=1` → `Run` with timeout → always attempt poweroff. On ANY error, best-effort `Destroy` (auto-destroy on failure). Audit `detonate-run`.
- `detonate collect <run> --out <dir>` — `PoweredOff` must be true (fail closed otherwise) → `Collect` → sha256 each artifact → audit `detonate-collect` with hashes.
- `detonate destroy <run>` — `Destroy` → audit `detonate-destroy`. Idempotent/best-effort.
- There is intentionally NO `restore`/`revert`/`reuse` verb.

- [ ] **Step 1: Failing tests** (FakeRunner + temp audit path): route picks the right tier and exit code; create fails closed when `GoldenExists`→false; run fails closed when `ConfigureIsolatedNet` yields a non-isolated attachment (`ValidateIsolated` error) and never calls `Runner.Run`; run auto-calls `Destroy` when `Run` errors; collect fails closed when `PoweredOff`→false; inject/collect write audit entries with sha256. Assert via the FakeRunner call log + audit read-back.
- [ ] **Step 2: RED** — `go test ./cmd/detonate/ -v`
- [ ] **Step 3: Implement** the CLI. Reuse `pkg/audit`, `pkg/validate` (run name validation). Confirmation prompt reads stdin; `--yes`+env bypass for automation.
- [ ] **Step 4: GREEN + full build** — `go test ./cmd/detonate/ ./pkg/detonate/`; `go build ./...`
- [ ] **Step 5: Commit** `feat(detonate): CLI state machine with in-code containment enforcement`

---

### Task 4: build wiring + docs

**Files:**
- Modify: `Makefile` (build `bin/detonate` in `build-all`), `docs/security.md` and/or a new `docs/guide/detonate.md`, `README`/CLAUDE.md pointer if natural.
- [ ] **Step 1** Add `bin/detonate` to the `build-all` target (`go build -o bin/detonate ./cmd/detonate`). Keep berth targets unchanged.
- [ ] **Step 2** Write `docs/guide/detonate.md`: what it is (Tier-A local detonation orchestrator), that it needs an operator-provisioned immutable golden + isolated no-uplink network + INetSim gateway + a sacrificial boundary, the verb lifecycle, the enforced hard boundaries, and the loud "benign in sandbox ≠ safe". Link the strategy doc. In `docs/security.md`, add one paragraph: `detonate` is separate from berth by design and fails closed without an isolated environment.
- [ ] **Step 3: Commit** `docs(detonate): document the Tier-A detonation harness and its prerequisites`

---

### Task 5: verification (safe — no live detonation)

- [ ] `make build-all` → `bin/detonate` builds.
- [ ] `go test ./pkg/detonate/ ./cmd/detonate/` green.
- [ ] Safe CLI smoke (no VM): `detonate route` on a sample JSON prints the right tier; `detonate run <run> --yes` against a FakeRunner-less real run FAILS CLOSED with "network not isolated / golden not found" (prove the guard, not a detonation). Record outputs.
- [ ] Confirm `detonate` has no `restore`/`revert` verb and `--help` states the safety model.

---

## Out of scope (documented, not built here)
- Shipping golden VM images, INetSim gateway automation beyond the isolated-net guard, memory-dump/Volatility integration, TierB cloud provisioning, TierC API clients — all operator/follow-up. The harness fails closed without them.
- Any live detonation — cannot be safely exercised without the operator's golden + sacrificial boundary; verification here is unit tests + fail-closed guard proof only.
