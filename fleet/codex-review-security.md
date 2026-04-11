# Security Review: Go Rewrite

**Branch:** `feature/pla-1192-rewrite-safe-agentic-cli-from-bash-to-go-phase-1-foundation`  
**Reviewer:** Codex  
**Date:** 2026-04-11  
**Scope:** All Go code under `cmd/safe-ag/`, `pkg/`, and `tui/`

## Executive Summary

The Go rewrite improves safety in a few places: most host-side process launches use argument vectors instead of shell parsing, `pkg/validate` blocks unsafe network modes, and `cmd/safe-ag/observe.go:828-856` defends against tar path traversal during session export.

I still found **5 actionable security issues**. The largest gaps are:

1. The documented security model says unsafe Docker modes are blocked, but the Go code still enables both `--privileged` DinD and raw Docker socket passthrough.
2. AWS credential handling overexposes secrets by passing the full host credentials file through container env and refresh flows.
3. The docs claim repo clone paths are validated, but `spawn`/`fleet` do not enforce that, and later log paths are interpolated into `bash -c`.

## Findings

### 1. High: documented Docker hardening is not actually enforced for `--docker` / `--docker-socket`

**Evidence**

- `CLAUDE.md:183-201` says unsafe Docker flags such as `--privileged` are blocked.
- `cmd/safe-ag/spawn.go:396-406` exposes both `--docker` and `--docker-socket`.
- `pkg/docker/dind.go:47-73` starts the DinD sidecar with `--privileged`.
- `pkg/docker/dind.go:34-44` mounts `/var/run/docker.sock` directly into the agent container.

**Impact**

The advertised hardening in `pkg/docker/runtime.go:128-156` only applies to the main agent container. An agent started with `--docker` can talk to a privileged daemon, and an agent started with `--docker-socket` gets raw control of the VM Docker daemon. Both options bypass the "safe by default" model documented in `CLAUDE.md`.

**Why this matters**

This is not just a documentation drift issue. Anyone relying on the current `CLAUDE.md` guarantees could assume privileged Docker modes are impossible, while the Go CLI still exposes them behind normal flags.

**Recommendation**

- Either remove these flags from Phase 1, or document them as explicit trust-boundary breaks.
- If they remain, require an additional danger acknowledgment flag and emit a prominent warning.

### 2. High: `--aws <profile>` injects the entire host credentials file into container env and refresh paths

**Evidence**

- `CLAUDE.md:189` documents `--aws <profile>` as profile-scoped and tmpfs-backed.
- `cmd/safe-ag/spawn.go:329-341` calls `inject.ReadAWSCredentials(...)` and adds every returned value as container env.
- `pkg/inject/inject.go:168-187` base64-encodes the full `~/.aws/credentials` file into `SAFE_AGENTIC_AWS_CREDS_B64`; it does not extract just the requested profile.
- `cmd/safe-ag/config_cmd.go:549-576` reads the same full file again during `aws-refresh` and writes it wholesale into the container.

**Impact**

Selecting one AWS profile exposes all profiles from the host credentials file to the container. Because the file is passed through Docker env at container creation time, the secret also becomes visible in container metadata via `docker inspect`, not just inside the tmpfs-backed `~/.aws` directory.

**Why this matters**

This is broader than the documented security model. The docs describe an opt-in profile injection. The implementation currently copies the full credential set and stores it in a much more observable channel.

**Recommendation**

- Parse the INI file and inject only the selected profile.
- Stop using Docker env for credential transport; use a temp file, `docker cp`, or a short-lived exec stream instead.
- Treat `aws-refresh` the same way so the refresh path does not reintroduce the same leak.

### 3. High: repo validation promised in `CLAUDE.md` is missing, and repo-derived paths are later interpolated into `bash -c`

**Evidence**

- `CLAUDE.md:244` says repo clone paths are validated by `pkg/repourl.ClonePath()`.
- `pkg/repourl/parse.go:11-51` contains the validator, but `cmd/safe-ag/spawn.go:155-173` never calls it for `opts.Repos`.
- `cmd/safe-ag/spawn.go:221-223` stores raw repo strings in `REPOS`, and `cmd/safe-ag/spawn.go:257-259` stores `repourl.DisplayLabel(opts.Repos)` in a label.
- When `ClonePath()` fails, `pkg/repourl/parse.go:63-66` falls back to the raw repo string for display.
- `cmd/safe-ag/observe.go:88-114` builds `find ...` and `tail ...` shell snippets from `repoLabel`-derived paths and `jsonlPath` without shell quoting.
- The TUI repeats the same pattern in `tui/actions.go:359-383` and `tui/actions.go:516-527`.

**Impact**

A crafted repo value accepted by `spawn`, `fleet`, or `pipeline` can survive into labels and later be interpolated into `bash -c` during `safe-ag logs` or TUI log loading. That creates a container-side command-injection path and, at minimum, breaks the documented guarantee that repo paths are validated before use.

**Recommendation**

- Validate every repo argument at the `spawn` / manifest boundary and reject anything `ClonePath()` would reject.
- Do not build these log lookups with `bash -c`; prefer direct `docker exec find ...` / `docker exec tail ...` argument vectors.
- If shell use is unavoidable, quote derived paths defensively.

### 4. Medium: template commands allow host-side path traversal

**Evidence**

- `cmd/safe-ag/config_cmd.go:377-415` resolves template names with `filepath.Join(userDir, name...)` and `filepath.Join(repoDir, name...)` with no basename validation.
- `cmd/safe-ag/config_cmd.go:427-441` writes new templates to `filepath.Join(dir, name+".md")` with the same lack of validation.

**Impact**

`safe-ag template show ../../some/file` can read outside the templates directories, and `safe-ag template create ../../some/file` can write outside `~/.config/safe-agentic/templates`. This is a direct host-side path traversal issue in the CLI.

**Recommendation**

- Restrict template names to a safe basename pattern such as `[A-Za-z0-9._-]+`.
- After joining, verify the cleaned path still stays under the intended template directory.

### 5. Medium: sensitive callback and notification values are stored in Docker labels

**Evidence**

- `pkg/labels/labels.go:12-18` defines labels for prompt, callback, and notification metadata.
- `cmd/safe-ag/spawn.go:348-388` stores:
  - the first 100 characters of the prompt in `safe-agentic.prompt`
  - `--on-complete` and `--on-fail` values in reversible base64 labels
  - `--notify` values in a reversible base64 label

**Impact**

Base64 is not protection. Any process or user with Docker inspect access in the VM can recover callback commands, notification targets, and prompt fragments. That is especially risky for webhook URLs, tokens embedded in shell commands, or prompts that contain proprietary data.

**Recommendation**

- Keep labels non-sensitive and state-like only.
- Store callbacks / notify targets in a file inside a private per-container volume or tmpfs instead of container metadata.

## Areas Reviewed With No Findings

- `pkg/validate/validate.go`: correctly rejects `host`, `bridge`, and `container:*` network modes.
- `pkg/orb/orb.go`: uses argument vectors for `orb` execution rather than shell concatenation.
- `cmd/safe-ag/workflow.go`: validates stash refs and branch names before interpolating them into shell commands.
- `cmd/safe-ag/observe.go:828-856`: blocks tar extraction path traversal during session export.

## Overall Assessment

The Go rewrite is moving in the right direction, but the current implementation does not yet match the security claims in `CLAUDE.md`. The biggest gaps are not cosmetic. They materially weaken the documented isolation story and expose more secret material than the old model implies.
