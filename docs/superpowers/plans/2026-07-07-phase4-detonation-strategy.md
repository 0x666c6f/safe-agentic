# Phase 4: Dynamic Detonation — Strategy & Architecture

> **Status: design only.** This document defines the architecture and hard boundaries for a *behavioral* malware-analysis capability. It deliberately lives **outside berth** and no detonation code should be written until the containment design here is reviewed and a sacrificial execution boundary exists. This is defensive research — the tool **observes** sample behavior to build detections; it never attacks, propagates, or arms anything.

Synthesized from two independent architectures (deep-reasoner/Opus + Codex), reconciled here.

## Bottom line up front

On an Apple Silicon Mac you **cannot locally detonate the thing you most need to detonate** — x86-64 Windows PE — because there is no native x86 virtualization on this silicon (QEMU TCG emulation is 10–50× slow and trivially fingerprinted; Windows-11-ARM/Prism runs user-mode only and its ARM environment is a dead giveaway; neither runs an x86 *kernel* driver). So Phase 4 is **not one sandbox — it is a router to three tiers**:

- **Tier A — Local ARM VM (on the Mac):** shell/Python/JS/VBA/PowerShell/JAR, Linux ELF (ARM native, x86-64 via Rosetta-in-a-Linux-VM), ARM64/x86-64 Mach-O. Fast, cheap, nothing leaves. Handles the bulk of triage volume.
- **Tier B — Self-hosted cloud x86 disposable** (CAPEv2/Cuckoo3 on an isolated x86 cloud VM you own): real x86-64 Windows PE, confidentiality preserved.
- **Tier C — Commercial sandbox** (VMRay/Joe private; Hatching Triage/Any.run/Hybrid Analysis public): when self-hosting isn't worth the ops cost. Trade-off: the sample leaves your control, and uploading *targeted* malware to a public service can tip off the adversary.

The routing key is the **architecture/format field from berth's static triage (Phase 1–3)**. The decision to detonate is always an **explicit human, higher-privilege act** — never automatic.

Anyone promising a "local Apple Silicon sandbox for Windows PE" is selling emulation that evasive malware defeats on timing and environment checks alone. Don't build that fantasy; route Windows PE to Tier B/C and say so.

---

## 1. Isolation substrate

### Feasibility (honest)

| Sample class | Local on Apple Silicon? | How | Verdict |
|---|---|---|---|
| Shell/Python/JS/VBA/PowerShell/JAR | Yes | Interpreter in an ARM Linux/Windows VM | **Tier A.** Caveat: a macro's *final* payload is usually an x86 PE it downloads — you observe the dropper, not the payload. |
| Linux ELF (ARM64) | Yes, native speed | ARM Linux VM | **Tier A.** |
| Linux ELF (x86-64) | Partly | Rosetta-in-a-Linux-VM (user-mode, near-native) or `qemu-user` | **Tier A, with caveats** (fingerprintable translation env). |
| Mach-O (ARM64 / x86-64) | Yes | macOS guest on Virtualization.framework (≤2 VMs/Apple license) + Rosetta | **Tier A.** Small corpus. |
| **Windows PE x86-64 (user-mode)** | **No, not usefully** | Prism user-mode emu (drivers fail) or QEMU TCG (too slow) | **Tier B/C.** |
| **Windows PE x86-64 (kernel/rootkit)** | **No** | nothing local runs an x86 Windows kernel driver on ARM | **Tier B/C only.** |

Apple's hypervisor accelerates **ARM-on-ARM only**; Rosetta 2 is user-mode translation (usefully exposable to a *Linux* guest for x86-64 ELF). Neither gives an x86 kernel.

### Recommended local substrate: clone-per-run, not snapshot-revert

Build Tier A on **[Tart](https://tart.run)** (Cirrus Labs) — a CLI over Apple Virtualization.framework:

- `tart clone golden run-<id>` — copy-on-write clone from an **immutable golden** (the golden is never booted for a run) → run → capture → `tart delete run-<id>`.
- No host filesystem sharing by default; scriptable; OCI push/pull for goldens.

**Clone-from-immutable-golden + destroy beats snapshot-revert**: the golden is read-only and untouched, every detonation is a throwaway CoW clone, and there is *no restore step that can be skipped* and no contaminated state that can survive to the next run.

- **x86 headless fallback:** `qemu-system-x86_64 -snapshot` (all writes discarded on exit) for niche short detonations of simple x86 Linux samples — flagged everywhere as slow and evasion-prone.
- **Reject:** Lima/Colima/Docker-on-Mac (heavy host FS sharing = escape surface); Parallels/VMware convenience features (guest tools = RPC surface).

Trade-off: Virtualization.framework exposes a *smaller emulated device model* than full QEMU (fewer VENOM-class device CVEs) at the cost of fewer knobs. For containment, the smaller attack surface wins.

---

## 2. Network containment

**Real internet, real DNS, and real LAN are never allowed.** Rationale: exfiltration of victim data; C2 liveness tips off a targeted adversary; the sample fetches a worse second stage or joins a live botnet; lateral spread to your LAN; your IP now attacks third parties (legal liability). You get *more* behavioral signal from a simulated network anyway.

**Simulated network reveals C2 without egress:**
- **INetSim** (on REMnux) — fake DNS/HTTP(S)/SMTP/FTP/IRC that answers plausibly, so the sample completes its handshake and you record beacon interval, URI patterns, User-Agent, DGA domains, protocol.
- **FakeNet-NG** (Mandiant/FLARE) — redirects all guest traffic to local listeners with DNS/HTTP/SSL MITM; better for Windows-tier guests.

**Topology — isolated segment, out-of-band gateway, no uplink:**

```
[ detonation guest ] --- isolated vmnet (no NAT, no bridge) --- [ netsim gateway VM ]
     (malware runs here)                                        INetSim/FakeNet + tcpdump
                                                                (malware never code-execs here)
```

- Capture (`tcpdump`/`tshark`) runs on the **gateway VM, out-of-band** — never inside the malware's own VM (it can kill or lie to an in-guest sniffer).
- Apple specifics: `VZNATNetworkDeviceAttachment` (→ internet) and `VZBridgedNetworkDeviceAttachment` (→ LAN) are **both forbidden**. Use `VZFileHandleNetworkDeviceAttachment` (you own every frame; pipe the two VMs into a userspace switch with no uplink) or Tart `--net-softnet` with an allowlist pinning the guest to only the gateway's MAC/IP.
- Defense in depth: rely on **absence of a route** (no path to the host vmnet, berth's subnet, or 169.254 link-local) *and additionally* a default-deny pf/nftables rule — never on the firewall alone.

---

## 3. Snapshot / revert / ephemeral lifecycle

```
immutable golden (read-only, hashed)
      │ CoW clone
      ▼
ephemeral run VM ── inject sample (offline) ── boot ── detonate w/ hard timeout
      │                                                        │
      │◄──────────── capture out-of-band ◄────────────────────┘
      ▼
power off ── collect artifacts (offline, RO) ── DESTROY clone ── verify golden untouched
```

- **Golden:** analysis OS + instrumentation pre-installed (Sysmon config, Procmon, Noriben, sysdig…), network pinned to the isolated segment, powered off clean, marked read-only, and **hashed** (baseline chain of custody).
- **Per run:** CoW clone → offline-inject → boot → detonate with a hard timeout (start 120–300 s; malware often sleeps — consider clock fast-forwarding) → capture → power off → destroy the clone.
- **A detonated VM is never reused without destroy:** persistence (Run keys, services, scheduled tasks, WMI subscriptions, in-VM bootkits) survives a soft reboot; a sandbox-aware sample may lie dormant to poison the next run; reuse also destroys the before/after diff baseline. Immutable-golden + destroy-clone eliminates the entire class — there is no reuse path to forget.

---

## 4. Sample in, results out — both untrusted

### One-way injection (no writable shared folder)

Writable shared folders (virtiofs/9p/webdav) are a **bidirectional escape surface** and exactly the guest→host break you are defending against. In order of preference:

1. **Offline disk inject (strongest):** mount the *powered-off* clone's disk on the host with libguestfs (`guestfish`/`virt-copy-in`), copy the sample in, unmount, *then* boot. The guest never runs while the host touches the disk — no live guest↔host channel exists.
2. **Read-only ISO/CD-ROM:** build an ISO on the host containing the sample, attach read-only. Simple and robust; no write-back path.

Never: clipboard, drag-and-drop, guest-tools copy/paste, host-guest RPC.

### Results out — artifacts are malware-authored bytes

- **Never execute or double-click a captured drop.** Never open a capture in a tool with a known parser-RCE surface without treating the input as hostile.
- **Extract offline:** power off → mount the ephemeral disk **read-only** (ideally on a dedicated parsing VM, not the analyst's daily host) → copy → hash → then destroy the clone.
- **Prefer out-of-band artifacts** you didn't have to trust the guest for: pcap from the gateway VM, screenshots via the hypervisor framebuffer, RAM dump via the hypervisor. Hypervisor-observed data is far more trustworthy than files the malware handed you.
- **Parse on the host** using berth's Phase-3 forensic image in a read-only, `noexec,nodev,nosuid` context.

### Chain of custody (extend Phase 2's audit chain)

sha256 the sample **before** inject; record golden hash + clone id; sha256 every artifact **on extraction**; append all to the same append-only audit chain berth already uses. Phase 2's evidence-mount + sha256 pattern applies to **both** the input sample and the output artifacts.

---

## 5. Host / berth / LAN protection — escape vectors to close

| Vector | Action |
|---|---|
| Shared folders (virtiofs/9p/webdav) | **OFF.** Offline-inject + RO-ISO instead (§4). |
| Clipboard sharing | **OFF** — don't install the guest agent that provides it. |
| Guest tools/additions incl. **qemu-guest-agent** | **Do not install.** qemu-guest-agent literally lets the host run commands in the guest; its bugs run the other way. Instrument out-of-band. |
| Drag-and-drop | **OFF** (part of guest tools). |
| Host-guest RPC / vsock / backdoor channels | **Eliminate/minimize.** vsock has real guest→host CVE history. If a control channel is unavoidable, make it one-way, read-only, strictly validated. |
| NAT/bridge, routes to berth vmnet or host | **Forbidden** (§2). No shared vmnet/socket/volume between detonation and berth — the founding premise. |
| Hypervisor device-model CVEs (VENOM-class) | **Minimal device model** — no sound, no USB passthrough, no extra NICs, no unneeded emulated hardware. Prefer Virtualization.framework's smaller surface. Keep the hypervisor patched. |
| USB / external device passthrough, camera, mic, Bluetooth, smart card | **Never.** |
| Cloud metadata (Tier B) | **Block `169.254.169.254`.** No service-account/IAM role on the detonation VM; no SSM/Azure/GCP guest agent — else a sample steals cloud credentials. |
| Secrets in the guest | **None.** No 1Password, no SSH agent, no GitHub token, no browser logins, no corp certs, no MDM enrollment, no NTP-to-real-time (fake NTP via INetSim if needed). |
| Analyst interaction with a live guest | **No direct RDP/SSH from the Mac into a running malware guest.** If interaction is unavoidable, use a disposable console broker inside the isolated network, record the screen, destroy after. |
| macOS host privilege | Detonate under a **separate low-privilege local user**; SIP on, FileVault on, host firewalled off the LAN. |
| Blast radius | **Prefer a dedicated sacrificial Mac** (old Mac mini, VLAN-segmented/air-gapped) as the detonation box, so a full VM escape lands on a throwaway, not the analyst's daily driver. Highest-leverage control. |

Residual risk you can't fully close locally: a hypervisor VM-escape 0-day. Sacrificial box + no-LAN-route + low-priv user is what bounds it.

---

## 6. Instrumentation & tooling — build on existing, don't reinvent

**Guest images:**
- Windows (Tier B/C): **FLARE-VM** (Mandiant) — Sysmon (Olaf Hartong / SwiftOnSecurity config), Procmon → **Noriben** for behavioral summaries, **Regshot** for before/after registry+FS diff, x64dbg + PE tools.
- Linux (Tier A): **REMnux** (Lenny Zeltser) — INetSim, Wireshark, Volatility3; runtime capture via **sysdig**/**Falco** (eBPF), `strace`/`auditd`/`fanotify` for simple cases.

**Orchestration:**
- **CAPEv2** (best-maintained Cuckoo fork; behavioral + config extraction + memory) or **Cuckoo3** — but both assume **x86 guests under KVM/VirtualBox on a Linux host**, i.e. **the cloud x86 tier**, not Apple Silicon. Bending CAPE onto ARM/Virtualization.framework guests is a research project, not a config change.
- For **Tier A local**, a **thin custom orchestrator** (§9) driving Tart + INetSim + sysdig is more realistic than forcing CAPE onto Apple Silicon; reuse CAPE's *report format* for compatible output.

**Capture:**
- Network: **tcpdump/tshark on the gateway VM** (out-of-band) + INetSim/FakeNet logs; JA3/JA3S from the pcap.
- FS/registry diff: **Regshot** (Windows) or, more robustly, an **offline block/file-tree diff of pristine golden vs post-run ephemeral disk on the host** — done entirely outside the guest, so malware can't hide it.
- Memory: full guest RAM dump via the hypervisor at end-of-run, then **Volatility3** on the host. **Out-of-band memory capture is the key anti-evasion win** — malware can't hide from a hypervisor RAM snapshot the way it hides from in-guest tools.
- Screenshots: hypervisor framebuffer at intervals, not a guest screenshot utility.

---

## 7. Threat model & limitations

- **Evasive / sandbox-aware malware won't detonate** (MITRE ATT&CK [T1497](https://attack.mitre.org/techniques/T1497/), Virtualization/Sandbox Evasion). It checks VM MAC OUIs, the CPUID hypervisor bit, device names, tiny disks, few CPUs, no user activity, analysis process names, known hostnames; it sleeps past the timeout; it geofences by IP (fakenet gives the "wrong" IP) or waits for an authentic C2 the sim can't speak. On Apple Silicon it's worse: QEMU TCG's slowness fails timing checks, and an ARM/Rosetta environment is an instant giveaway for x86-targeted malware — so the local tier is inherently weaker against evasive samples. Another reason Windows PE goes to Tier B/C.
- **Arms race:** anti-anti-VM hardening (patched VM artifacts, faked user activity, decoy files, aged system, realistic uptime) vs ever-better detection. You raise the bar; you never win permanently.
- **Coverage/timing:** one execution path, one environment, one point in time. Miss the right date, target, mutex, parent process, or a live C2 and you see nothing.
- **The verdict that matters most: "benign in the sandbox" is NOT proof of safety.** No observed malice means the sandbox didn't trigger it — not that the sample is clean. **Only positive findings (observed C2, drops, injection, persistence) are trustworthy.** Never green-light a sample as safe from a quiet detonation. State this loudly in every report.

---

## 8. Hard boundaries — NEVER do (inviolable)

1. Never give a detonation VM real internet, real DNS, or real LAN. Simulated network only.
2. Never run detonation inside berth's VM, or anything sharing a network, socket, or volume with berth, the host, or the analyst's other work.
3. Never reuse a detonated VM. Destroy the clone; re-clone the immutable golden each run.
4. Never use writable shared folders, clipboard, drag-and-drop, or guest-agent RPC between guest and host.
5. Never execute or auto-open captured artifacts on the host. Treat them as inert data; parse read-only, `noexec`.
6. Never run detonation as a privileged user, and never on the analyst's primary machine without a sacrificial boundary.
7. Never let the sample reach the host during injection while the guest is live. Inject offline (disk-mount / RO-ISO).
8. Never treat a benign verdict as proof of safety.
9. Never weaponize: no building/arming C2, no propagating the sample, no running it against any third-party target, no live-fire outside the isolated fakenet. The tool **observes**; it never attacks.
10. Never skip chain of custody: hash the sample in, artifacts out, append to the audit chain.
11. Never disable host/hypervisor hardening (SIP, firewall, network isolation) to "make the sample run." If it won't detonate safely, it doesn't detonate locally.
12. Detonation is a distinct, explicitly-invoked, higher-privilege operation — never auto-triggered by static triage.

---

## 9. Handoff from berth

**Static → detonation decision.** berth Phases 1–3 emit a forensic JSON (sha256, file type via `file`/TrID, **architecture/format**, packing, YARA hits, capability guesses, a detonation-worthiness note). A **human** reads it and decides — detonation is never automatic. The arch/format field routes the tier:

- ARM/script/ELF/Mach-O → **Tier A local.**
- x86-64 Windows PE → **Tier B self-hosted cloud** (confidentiality preserved) or **Tier C commercial** (ops-cheap, but the sample leaves your control; route *targeted* samples to private/self-hosted only).
- Unsupported substrate → **refuse**, don't fake it.

**A separate `detonate` binary — deliberately not a `berth` subcommand.** berth's entire value is safe-by-default *agent* execution; bolting live-malware execution onto it would blur the exact blast-radius boundary this design protects. `detonate` is a strict state machine the analyst never bypasses by hand:

```
detonate route   <static.json>      # pick Tier A/B/C or refuse
detonate create  <run>              # CoW-clone immutable golden -> ephemeral VM
detonate inject  <run> <sample>     # offline disk-mount / RO-ISO; hash into custody
detonate run     <run> --timeout N  # boot on isolated vmnet + netsim gateway;
                                     #   instrument; hard timeout; power off
detonate collect <run>              # offline RO-mount; pull artifacts + pcap + RAM dump;
                                     #   hash into custody; emit behavioral report
detonate destroy <run>              # delete clone+disk; verify golden untouched
```

The state machine **enforces** the safety: `run` refuses without an isolated network; `collect` refuses before power-off; **reuse is impossible** (no restore verb — only clone/destroy); it auto-destroys on completion *and* on error; and `run` requires an explicit typed confirmation ("you are about to execute live malware"). The analyst **never manually touches a live-malware VM** — no GUI clicking, no console into a running guest.

**Why higher-privilege:** static triage only reads bytes; detonation manipulates VMs, isolated networks, offline disk mounts (root on the host), and runs live malware. It runs under a separate user / on the sacrificial box, is logged as a distinct operation class in the audit chain, and is gated behind explicit human confirmation.

---

## What we will NOT build

- Any path giving the sample real network reachability, a real target, or hosted/armed C2.
- Any writable shared folder or "mount host home into the detonation VM for convenience."
- Auto-detonate-on-upload, or any flow that runs live malware without an explicit human higher-privilege confirmation.
- A marketed "local Apple Silicon Windows-PE sandbox" — it can't honestly run kernel-mode x86 and would give false confidence. Route Windows PE to Tier B/C and say so.

## The one thing to internalize

For the dominant real-world case — x86-64 Windows PE, especially with kernel components — the correct answer for an Apple Silicon analyst is a **self-hosted cloud x86 disposable or a commercial sandbox**, not a local rig pretending to be one. Local Tier A is genuinely valuable for the high-volume script/document/Linux/Mach-O cases; be equally honest about where it stops.

---

## Recommended next steps (if/when this proceeds)

1. **Do not start with code.** Stand up the *boundary* first: a sacrificial/segmented detonation host (or an isolated cloud x86 VM for Tier B), off the LAN, low-priv user.
2. **Prototype Tier A** on that boundary: Tart golden (REMnux) + an INetSim gateway VM on a no-uplink `VZFileHandle` switch; validate with a *known-benign* EICAR-style test and a *known* open-source sample in a controlled setting — never a live unknown until the containment is proven.
3. **Validate containment before trust:** confirm no route to host/berth/LAN (packet-count the gateway, like the api-only live gate did), confirm offline inject/extract works, confirm clone-destroy leaves the golden hash unchanged.
4. **Only then** wire the `detonate` state machine, and keep it a separate binary with its own audit-log operation class.
5. **For Windows PE, evaluate Tier B vs C** on your confidentiality/ops constraints rather than attempting local emulation.

---

## References

The two independent designs converged on this architecture; the load-bearing claims trace to:

- **Apple Silicon x86 limits:** UTM ([system](https://docs.getutm.app/settings-qemu/system/), [network](https://docs.getutm.app/settings-qemu/devices/network/network/), [sharing](https://docs.getutm.app/settings-qemu/sharing/)) · VMware Fusion on Apple Silicon ([Broadcom KB 315602](https://knowledge.broadcom.com/external/article/315602)) · Windows-on-Arm x86 emulation is user-mode only ([Microsoft: apps-on-arm x86 emulation](https://learn.microsoft.com/en-us/windows/arm/apps-on-arm-x86-emulation), [drivers must be native Arm64](https://learn.microsoft.com/en-us/windows/arm/add-arm-support)) · Parallels x86 emulation preview ([KB 130217](https://kb.parallels.com/en/130217)).
- **Orchestration & lifecycle:** [CAPEv2](https://capev2.readthedocs.io/) ([snapshot save/revert](https://capev2.readthedocs.io/en/latest/installation/guest/saving.html), [routing — "internet routing is a dirty line"](https://capev2.readthedocs.io/en/latest/installation/host/routing.html)) · [Cuckoo3](https://github.com/cert-ee/cuckoo3) · [Cuckoo routing](https://cuckoo.readthedocs.io/en/latest/installation/host/routing/) / [snapshot](https://cuckoo.readthedocs.io/en/latest/installation/guest/saving/).
- **Simulated network:** [INetSim](https://www.inetsim.org/) · [FakeNet-NG](https://github.com/mandiant/flare-fakenet-ng).
- **Guest images / telemetry:** [REMnux](https://remnux.org/) · [FLARE-VM](https://github.com/mandiant/flare-vm) · [Sysmon](https://learn.microsoft.com/en-us/sysinternals/downloads/sysmon) · [Procmon](https://learn.microsoft.com/en-us/sysinternals/downloads/procmon) · [Noriben](https://github.com/rurik/noriben) · [sysdig](https://github.com/draios/sysdig) · [Falco](https://falco.org/docs/) · [Volatility3](https://github.com/volatilityfoundation/volatility3).
- **Escape-surface (guest tools):** [qemu-guest-agent lets the host run commands in the guest](https://docs.redhat.com/en/documentation/red_hat_enterprise_linux/7/html/virtualization_deployment_and_administration_guide/chap-qemu_guest_agent) · [Parallels Tools shared folders](https://docs.parallels.com/landing/pdfm-ug/v19-en-us/parallels-desktop-for-mac-19-users-guide/advanced-topics/installing-and-updating-parallels-tools/parallels-tools-overview).
- **Evasion & custody:** MITRE ATT&CK [T1497 Virtualization/Sandbox Evasion](https://attack.mitre.org/techniques/T1497/) · NIST [chain of custody](https://csrc.nist.gov/glossary/term/chain_of_custody) and [NISTIR 8387 (digital evidence preservation, hashing)](https://nvlpubs.nist.gov/nistpubs/ir/2022/NIST.IR.8387.pdf).
- **Commercial sandbox (Tier C):** [Joe Sandbox](https://www.joesandbox.com/) (private tier for confidentiality).
