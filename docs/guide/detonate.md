# Detonate

`detonate` is a **separate binary** (`cmd/detonate`, built to `bin/detonate`) for Tier-A local behavioral malware analysis — actually *running* a live sample in a disposable VM to observe what it does. It is deliberately **not** a `berth` subcommand: berth's entire value is safe-by-default *agent* execution against untrusted code and repos, and bolting live-malware detonation onto that binary would blur the exact blast-radius boundary berth exists to protect. `detonate` is a distinct, higher-privilege, explicitly-invoked operation.

If you only need to inspect a suspicious file without running it, you don't need `detonate` — use [`berth spawn --forensic`](../security.md#forensic-triage) or [`--evidence`](../security.md#evidence-mount) for static triage instead.

## What it is not

`detonate` does not ship a sandbox. It ships an **enforced state machine** around a sandbox you build and prove is isolated. There is no golden VM image, no INetSim gateway, and no bundled malware — you provision all of it yourself, per the [detonation strategy doc](../superpowers/plans/2026-07-07-phase4-detonation-strategy.md).

## Operator prerequisites — it FAILS CLOSED without these

`detonate` refuses to run rather than fake success. Before `run` will do anything, you need:

- **An immutable golden VM image** — a pre-built, read-only, hashed analysis VM (instrumentation installed, network pre-pinned to the isolated segment, powered off clean). Every run clones it; the golden itself is never booted.
- **An isolated, no-uplink network** — a segment with no NAT, no bridge, and no route to the host, berth's network, or your LAN. Isolation is **code-enforced**: the Tart backend boots the guest with `tart run --net-softnet --net-softnet-allow=<cidr>`, never `--net-bridged` (a bridge can reach the host LAN/internet). The `--gateway` you pass is that softnet allow-list CIDR, and `detonate` rejects any allow-list that isn't a single private range — `0.0.0.0/0`, `::/0`, or any public/routable CIDR fails closed before boot.
- **An INetSim (or equivalent) fakenet gateway VM** on that private segment (e.g. `10.0.0.0/24`), running out-of-band capture (`tcpdump`/`tshark`) and simulated DNS/HTTP/SMTP, so the sample gets a plausible network to talk to without ever reaching the real internet. The softnet allow-list pins the guest to exactly this gateway's private CIDR.
- **A sacrificial or segmented detonation host** — never the analyst's daily Mac. A full VM escape should land on a throwaway box, not your primary machine.

The Tart backend now runs a real detonation: `inject` builds a read-only (`hdiutil` UDRO) disk image of the sample and `run` attaches it read-only via `tart run --disk=<image>:ro`, so the sample disk can never be written back to. Provisioning the golden image and the fakenet gateway VM is still operator work — `detonate` enforces the containment around them, it does not build them for you.

**Known hardening trade-off:** artifacts leave the guest through a writable host share (`tart run --dir=out:<dir>`), which is an escape surface a compromised guest could abuse. `collect` is already symlink-safe (it never dereferences guest-planted symlinks), but the stronger fix — collecting artifacts from an offline disk after poweroff — is deferred, since it can't be implemented or verified without a real Tart guest.

## Verb lifecycle

```
detonate route   <static.json>              # pick Tier A/B/C, or refuse
detonate create  <run> --golden <name>       # CoW-clone the immutable golden
detonate inject  <run> <sample>              # offline disk-mount; hash into custody
detonate run     <run> --gateway <private-cidr> --timeout 180s [--yes]
                                              # boot on softnet pinned to <private-cidr>; hard timeout
detonate collect <run> --out <dir>           # offline RO-mount; pull + hash artifacts
detonate destroy <run>                       # delete the clone; golden stays untouched
```

Example sequence:

```bash
detonate route static-findings.json
detonate create run-042 --golden remnux-golden
detonate inject run-042 ./sample.bin
detonate run run-042 --gateway 10.0.0.0/24 --timeout 300s
detonate collect run-042 --out ~/detonate-out/run-042
detonate destroy run-042
```

`route` reads berth's static-triage JSON (arch/format) and decides Tier A (local), Tier B/C (self-hosted or commercial cloud — not orchestrated by this tool), or refuses. The decision to detonate is always a separate, explicit, human act — never automatic.

## Enforced hard boundaries

These are enforced in code, not left to operator discipline:

- **Isolated-network-only.** `run` calls `ConfigureIsolatedNet` (which rejects any allow-list that isn't a single private CIDR), validates the result before boot, and re-validates the captured CIDR again immediately before invoking `tart run`; anything that isn't isolated fails closed and `Run` is never invoked. The guest is booted with `--net-softnet`, never `--net-bridged`.
- **Offline sample injection, read-only.** `inject` builds a read-only disk image of the sample; `run` refuses to boot a run with no injected sample and attaches the image via `--disk=<image>:ro`. There is no live network or shared-folder path for the sample to enter the guest, and the sample disk is never writable.
- **PoweredOff-gated collect.** `collect` checks `PoweredOff` first and refuses to pull artifacts from a running VM.
- **Typed confirmation.** `run` prints a warning and requires you to type `detonate <run-name>` back exactly, or pass `--yes` **and** set `DETONATE_I_UNDERSTAND=1` — a bare `--yes` in a script is not enough.
- **Auto-destroy on failure.** If `Run` errors, `detonate` destroys the clone automatically before returning the error.
- **No reuse.** Every run's lifecycle state (`Created` → `Injected` → `Detonated` → `Collected`, plus the terminal `Destroyed`) is persisted to a per-run JSON file (`~/.berth/detonate/<run>.json`, overridable via `DETONATE_STATE_DIR`) and checked before every verb: `create` refuses if a run by that name already exists, `inject` requires `Created`, `run` requires `Injected` and refuses outright — "run `<run>` already detonated" — if the state is already `Detonated`/`Collected`, and `collect` requires `Detonated`. This holds across separate invocations, not just within one process. `run` marks state `Detonated` *before* booting, not after success, so a boot that errors and then fails to auto-destroy still leaves the run blocked from reuse. Each gated verb also takes an exclusive per-run advisory lock (`<run>.lock`) for its whole check-then-act section, so two concurrent invocations against the same run name can't both pass the state check before either persists the result. `destroy` is the sole escape hatch: it's always allowed, from any state, and clears both the state and lock files so the run name can be created fresh.

## Chain of custody

Every verb appends an entry to berth's existing append-only audit log: `inject` records the sample's sha256 before any VM work happens (so the attempt is logged even if the injection fails closed); `collect` hashes every extracted artifact. Review the trail with:

```bash
berth audit
```

## Benign in the sandbox is NOT proof of safety

**Read this before trusting a quiet run.** A sample that shows no malicious behavior during detonation has not been proven safe — it may be evasive, environment-aware, waiting for a trigger the sandbox didn't provide, or simply sleeping past the timeout. Only *positive* findings (observed C2, drops, injection, persistence) are trustworthy. Never green-light a sample as safe based on a quiet detonation, and say so loudly in every report.

See the [detonation strategy doc](../superpowers/plans/2026-07-07-phase4-detonation-strategy.md) for the full threat model, isolation substrate rationale, and the complete list of inviolable boundaries.
