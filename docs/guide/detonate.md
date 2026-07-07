# Detonate

`detonate` is a **separate binary** (`cmd/detonate`, built to `bin/detonate`) for Tier-A local behavioral malware analysis — actually *running* a live sample in a disposable VM to observe what it does. It is deliberately **not** a `berth` subcommand: berth's entire value is safe-by-default *agent* execution against untrusted code and repos, and bolting live-malware detonation onto that binary would blur the exact blast-radius boundary berth exists to protect. `detonate` is a distinct, higher-privilege, explicitly-invoked operation.

If you only need to inspect a suspicious file without running it, you don't need `detonate` — use [`berth spawn --forensic`](../security.md#forensic-triage) or [`--evidence`](../security.md#evidence-mount) for static triage instead.

## What it is not

`detonate` does not ship a sandbox. It ships an **enforced state machine** around a sandbox you build and prove is isolated. There is no golden VM image, no INetSim gateway, and no bundled malware — you provision all of it yourself, per the [detonation strategy doc](../superpowers/plans/2026-07-07-phase4-detonation-strategy.md).

## Operator prerequisites — it FAILS CLOSED without these

`detonate` refuses to run rather than fake success. Before `run` will do anything, you need:

- **An immutable golden VM image** — a pre-built, read-only, hashed analysis VM (instrumentation installed, network pre-pinned to the isolated segment, powered off clean). Every run clones it; the golden itself is never booted.
- **An isolated, no-uplink network** — a segment with no NAT, no bridge, and no route to the host, berth's network, or your LAN. `run` re-validates this immediately before boot and refuses if it isn't provably isolated.
- **An INetSim (or equivalent) gateway VM** on that segment, running out-of-band capture (`tcpdump`/`tshark`) and simulated DNS/HTTP/SMTP, so the sample gets a plausible network to talk to without ever reaching the real internet.
- **A sacrificial or segmented detonation host** — never the analyst's daily Mac. A full VM escape should land on a throwaway box, not your primary machine.

`InjectOffline` and real network-isolation verification are deliberate fail-closed stubs today: the harness refuses the operation rather than pretend it happened. Wiring them to a real hypervisor/network backend is operator work, not something `detonate` will fake for you.

## Verb lifecycle

```
detonate route   <static.json>              # pick Tier A/B/C, or refuse
detonate create  <run> --golden <name>       # CoW-clone the immutable golden
detonate inject  <run> <sample>              # offline disk-mount; hash into custody
detonate run     <run> --gateway <gw> --timeout 180s [--yes]
                                              # boot on the isolated net only; hard timeout
detonate collect <run> --out <dir>           # offline RO-mount; pull + hash artifacts
detonate destroy <run>                       # delete the clone; golden stays untouched
```

Example sequence:

```bash
detonate route static-findings.json
detonate create run-042 --golden remnux-golden
detonate inject run-042 ./sample.bin
detonate run run-042 --gateway isolated-gw --timeout 300s
detonate collect run-042 --out ~/detonate-out/run-042
detonate destroy run-042
```

`route` reads berth's static-triage JSON (arch/format) and decides Tier A (local), Tier B/C (self-hosted or commercial cloud — not orchestrated by this tool), or refuses. The decision to detonate is always a separate, explicit, human act — never automatic.

## Enforced hard boundaries

These are enforced in code, not left to operator discipline:

- **Isolated-network-only.** `run` calls `ConfigureIsolatedNet` and validates the result before boot; anything that isn't isolated fails closed and `Run` is never invoked.
- **Offline sample injection.** `inject` attaches the sample to the guest disk before boot. There is no live network or shared-folder path for the sample to enter the guest.
- **PoweredOff-gated collect.** `collect` checks `PoweredOff` first and refuses to pull artifacts from a running VM.
- **Typed confirmation.** `run` prints a warning and requires you to type `detonate <run-name>` back exactly, or pass `--yes` **and** set `DETONATE_I_UNDERSTAND=1` — a bare `--yes` in a script is not enough.
- **Auto-destroy on failure.** If `Run` errors, `detonate` destroys the clone automatically before returning the error.

**Not enforced — operator discipline required:** there is no restore or revert verb, only `create` (clone) and `destroy`. But nothing in code stops re-invoking `run` on an un-destroyed clone that has already touched a live sample — there is no run-state store tracking "this clone is used up." The operator MUST `destroy` a run after `collect` (or immediately on failure) rather than relying on the tool to refuse reuse.

## Chain of custody

Every verb appends an entry to berth's existing append-only audit log: `inject` records the sample's sha256 before any VM work happens (so the attempt is logged even if the injection fails closed); `collect` hashes every extracted artifact. Review the trail with:

```bash
berth audit
```

## Benign in the sandbox is NOT proof of safety

**Read this before trusting a quiet run.** A sample that shows no malicious behavior during detonation has not been proven safe — it may be evasive, environment-aware, waiting for a trigger the sandbox didn't provide, or simply sleeping past the timeout. Only *positive* findings (observed C2, drops, injection, persistence) are trustworthy. Never green-light a sample as safe based on a quiet detonation, and say so loudly in every report.

See the [detonation strategy doc](../superpowers/plans/2026-07-07-phase4-detonation-strategy.md) for the full threat model, isolation substrate rationale, and the complete list of inviolable boundaries.
