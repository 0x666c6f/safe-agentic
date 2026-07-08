# Evidence Mount (Phase 2) Implementation Plan

> Execute subagent-driven. Steps use checkbox syntax.

**Goal:** `berth spawn --evidence <local-path>` streams suspicious files from the host into the agent container as a **read-only** `/evidence` volume, records a sha256 chain-of-custody manifest in the audit log at ingestion, and defaults the network to `api-only` — so untrusted local files can be analyzed without a git repo and without weakening the VM boundary.

**Architecture:** berth runs on the macOS host, so it reads the host path directly, computes a sha256 manifest, and tars it. The tar is streamed (host stdin → `container machine run -i` → a throwaway `docker run -i` populate container) into a per-agent named Docker volume; the volume is then mounted **read-only** at `/evidence` in the agent container. Samples never touch git and never use a host bind mount, so `--home-mount none` stays intact. The manifest goes to the append-only audit log for chain of custody.

**Tech Stack:** Go (host-side tar + crypto/sha256, cobra, FakeExecutor tests), Docker named volumes, the existing `container machine run -i` stdin-stream pattern (see `cmd/berth/setup.go` `syncBuildContextToVMImpl`).

## Global Constraints

- Evidence is mounted **read-only** (`type=volume,...,readonly`). Execution is contained by Phase 1's api-only network + `cap-drop ALL` + `no-new-privileges`; noexec is NOT kernel-enforced on the volume (documented trade-off — do not claim noexec).
- Samples enter ONLY via host-side tar stream into a Docker volume. No host bind mount, no git. The VM stays `home-mount=none`.
- The evidence volume is named `<container>-evidence`, labeled `app=berth` and `berth.type=evidence` (+ parent container), so existing cleanup removes it.
- `--evidence` defaults `--network` to `api-only` when the user gave no explicit network (same safe-by-default chokepoint that `--forensic` uses). Explicit `--network` wins. Fold the condition in — do not duplicate the logic.
- A sha256 manifest (per-file relative path + hex digest + size) is written to the audit log via one `audit` entry at ingestion, before the container launches.
- Host path validation: the path must exist and be a regular file or directory; reject otherwise with a clear error. Follow symlinks to the top-level target but do NOT archive symlink entries pointing outside the tree (store them as regular files' contents or skip with a note) — keep it simple: archive regular files and directories; skip other node types with a logged note.
- Files in the volume must be readable by the in-container agent user (uid 1000, userns-remapped). The populate step extracts with `--no-same-owner` and `chmod -R a+rX` so the agent can read regardless of remapped ownership.

---

### Task 1: `pkg/evidence` — host-side manifest + tar

**Files:**
- Create: `pkg/evidence/evidence.go`
- Test: `pkg/evidence/evidence_test.go`

**Interfaces:**
- Produces:
  - `type Entry struct { Path string; SHA256 string; Size int64 }`
  - `type Manifest struct { Root string; Entries []Entry }`
  - `func Build(root string) (Manifest, error)` — validates `root` exists (regular file or dir); walks it (a single file is allowed — its Path is its basename); for each regular file computes sha256 (hex) and size; returns the manifest sorted by Path. Returns an error if root doesn't exist or is an unsupported node type.
  - `func WriteTar(w io.Writer, root string) error` — writes a tar of the same regular files (paths relative to `root`, or basename for a single file), mode forced to `0444`, no uid/gid, so extraction is deterministic and read-only-friendly.
  - `func (m Manifest) String() string` — compact one-line-per-file `sha256  size  path` rendering (for audit/detail serialization).

- [ ] **Step 1: Write failing tests** covering: a temp dir with 2 files + a subdir → manifest has 2 entries, correct sha256 (compare against `sha256.Sum256` of known content), sorted; a single file root → one entry with basename; a nonexistent path → error; `WriteTar` output can be read back with `archive/tar` and yields the same file set with mode 0444. Provide the actual test code.
- [ ] **Step 2: Run — RED.** `go test ./pkg/evidence/ -v`
- [ ] **Step 3: Implement** `evidence.go` with `crypto/sha256`, `archive/tar`, `path/filepath.WalkDir`. Keep it dependency-free (stdlib only).
- [ ] **Step 4: Run — GREEN.** `go test ./pkg/evidence/`
- [ ] **Step 5: Commit** `feat(evidence): host-side sha256 manifest and tar builder`

---

### Task 2: read-only named-volume mount on DockerRunCmd

**Files:**
- Modify: `pkg/docker/runtime.go` (near `AddNamedVolume`, line 65)
- Test: `pkg/docker/runtime_test.go`

**Interfaces:**
- Consumes: existing `AddNamedVolume`, `mustSafeMountValue`.
- Produces: `func (d *DockerRunCmd) AddNamedVolumeRO(src, dst string)` — appends `--mount type=volume,src=<src>,dst=<dst>,readonly`.

- [ ] **Step 1: Failing test** asserting the built args contain `type=volume,src=ev,dst=/evidence,readonly`.
- [ ] **Step 2: RED.** `go test ./pkg/docker/ -run NamedVolumeRO -v`
- [ ] **Step 3: Implement** the method (mirror `AddNamedVolume` + `,readonly`, with the same `mustSafeMountValue` guards).
- [ ] **Step 4: GREEN + full package.** `go test ./pkg/docker/`
- [ ] **Step 5: Commit** `feat(docker): AddNamedVolumeRO for read-only volume mounts`

---

### Task 3: evidence ingestion (validate → manifest → audit → populate volume)

**Files:**
- Create: `cmd/berth/evidence.go` (host-side ingestion glue)
- Test: `cmd/berth/evidence_test.go`
- Modify: `pkg/audit` only if needed (the `Log(action, container, details map[string]string)` API already fits — serialize the manifest into one detail field).

**Interfaces:**
- Consumes: `evidence.Build`, `evidence.WriteTar`, `docker.CreateLabeledVolume`, `audit.Logger`, the `container machine run -i` stdin-stream pattern from `setup.go`.
- Produces:
  - `func ingestEvidence(ctx, exec vmexec.Executor, vmName, containerName, hostPath, imageName string, dryRun bool) (string, error)` — returns the evidence **volume name** (`<containerName>-evidence`). Steps: `evidence.Build(hostPath)` (validate + manifest); log an audit entry `action="evidence-ingest"`, container=containerName, details `{"root": hostPath, "files": manifest.String(), "count": N}`; if dryRun, return the volume name without touching Docker; else `docker.CreateLabeledVolume(... "evidence", containerName)`, then populate it by streaming `evidence.WriteTar` into a throwaway container.
  - Populate helper (host-side, like `syncBuildContextToVMImpl` — uses `os/exec` directly, not the Executor, because it needs a live stdin pipe):
    `container machine run -i -n <vm> -u root -- docker run --rm -i -v <vol>:/evidence-dest --entrypoint /bin/bash <image> -c "tar xf - -C /evidence-dest --no-same-owner && chmod -R a+rX /evidence-dest"` — stream the tar to its stdin. Use `<image>` = the resolved agent image (has bash + tar). Check the exit and surface stderr on failure.

Note: keep the audit write BEFORE the Docker work so chain-of-custody is recorded even if population later fails. The manifest in the audit log is the source of truth; do not also write a manifest file into the (read-only) volume.

- [ ] **Step 1: Failing tests** (FakeExecutor + a temp dir): `ingestEvidence(dryRun=true)` returns `<container>-evidence`, writes ONE audit entry whose details include the file count and a sha256 substring, and does NOT create a volume (assert via a temp audit path you Read back, and `fake.CommandsMatching("docker volume create")` is empty). A nonexistent host path → error before any audit/Docker call. (The live stdin-populate path is exercised in Task 6, not unit-tested — note this.)
- [ ] **Step 2: RED.**
- [ ] **Step 3: Implement** `evidence.go`. For the populate step, factor the raw `exec.Command("container","machine","run","-i",...)` into a package var (like `syncBuildContextToVM` is a swappable var) so tests don't invoke real Docker; in dry-run and unit tests it isn't called.
- [ ] **Step 4: GREEN + full package.** `go test ./cmd/berth/`
- [ ] **Step 5: Commit** `feat(evidence): ingest host files into a labeled evidence volume with an audit manifest`

---

### Task 4: spawn `--evidence` wiring + cleanup

**Files:**
- Modify: `cmd/berth/spawn.go` (SpawnOpts, both flag sets, the api-only default chokepoint `applySpawnConfigDefaults`, the run-command build, and the spawn execution path where `ingestEvidence` is called and the volume is mounted), `cmd/berth/lifecycle.go` (remove the evidence volume when the container is stopped)
- Test: `cmd/berth/spawn_test.go`

**Interfaces:**
- Consumes: `ingestEvidence`, `docker.AddNamedVolumeRO`, `policy.NetworkAPIOnly`.
- Produces: `Evidence string` on SpawnOpts (`--evidence <path>`), registered on spawn AND run. When set: the api-only default applies (extend the existing `opts.Forensic` condition in `applySpawnConfigDefaults` to also trigger on `opts.Evidence != ""`); during spawn execution (not dry-run) `ingestEvidence` runs and its returned volume is mounted read-only at `/evidence` via `AddNamedVolumeRO`; the volume is removed on `berth stop`.

- [ ] **Step 1: Failing tests**: (a) `applySpawnConfigDefaults` with `Evidence:"/x", Network:""` → `opts.Network == "api-only"`; explicit network wins. (b) the run command for a spawn with a resolved evidence volume contains `type=volume,src=<container>-evidence,dst=/evidence,readonly` (build the cmd with the evidence volume already resolved, mirroring how other mount tests are structured). (c) `--evidence` is registered on both `spawnCmd` and `runCmd`.
- [ ] **Step 2: RED.**
- [ ] **Step 3: Implement.** Add the field + flags (`--evidence <path>` "Mount a host file/dir read-only at /evidence for analysis; records a sha256 manifest and defaults --network to api-only"). Extend the api-only default condition. In the spawn execution path, after the container name is known and before building/starting the container, call `ingestEvidence(...)` (skip on dry-run) and mount the returned volume RO. Add evidence-volume removal in the stop/remove path next to the managed-network removal (`docker volume rm <container>-evidence`, best-effort).
- [ ] **Step 4: GREEN + full suite.** `go test ./cmd/berth/ ./pkg/...`
- [ ] **Step 5: Commit** `feat(spawn): --evidence mounts host files read-only at /evidence with api-only default`

---

### Task 5: preamble + docs

**Files:**
- Modify: `entrypoint.sh` (security preamble — mention `/evidence` when present), `config/security-preamble.md` only if it enumerates mounts, `docs/reference/cli.md`, `docs/security.md`
- [ ] **Step 1:** In `entrypoint.sh`, if `/evidence` exists (or a `BERTH_EVIDENCE=1` env berth sets when `--evidence` is used), add a preamble line: read-only evidence is mounted at `/evidence`; treat it as untrusted data, never execute. Set that env in spawn when `--evidence` is used.
- [ ] **Step 2:** `docs/reference/cli.md`: document `--evidence` under spawn/run. `docs/security.md`: in the Forensic triage area, add an "Evidence mount" note — read-only volume, sha256 manifest in the audit log (`berth audit`), defaults to api-only, no host bind / no git, and the explicit noexec trade-off (read-only prevents tampering; execution is contained by api-only + capless, not by a noexec mount).
- [ ] **Step 3: Commit** `docs(evidence): document --evidence read-only mount and chain of custody`

---

### Task 6: live verification (acceptance gate — heavy)

- [ ] Build (`make build-all`). Create a temp dir with a couple of sample files on the host.
- [ ] `berth spawn claude --evidence <dir> --forensic --dry-run` → run command shows `.../evidence...,readonly`, network resolves to `api-only`, and an `evidence-ingest` line appears in `berth audit`.
- [ ] Non-dry-run: spawn, then `docker exec <c>` → `/evidence` is populated, files readable by the agent, and a write to `/evidence` FAILS (read-only). `berth audit` shows the sha256 manifest matching `sha256sum /evidence/*` inside the container.
- [ ] `berth stop <c>` → the `<c>-evidence` volume is gone (`docker volume ls`).
- [ ] Record results in the PR.

---

## Out of scope
- noexec enforcement on the evidence mount (chosen trade-off: read-only volume; execution contained by api-only + capless). A tmpfs-noexec variant is a possible later hardening.
- Dynamic detonation (Phase 4 — separate disposable VM).
