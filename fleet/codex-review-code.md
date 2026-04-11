# Codex Go Code Review

## Critical

- `cmd/safe-ag/fleet.go:273` and `cmd/safe-ag/fleet.go:318`: `runPipelineManifest` reconstructs container names with a fresh `time.Now()` after `executeSpawn` returns. When a stage falls back to timestamp-based naming, that recomputed name can differ from the real container name. `waitForContainers` then treats every `docker inspect` failure as "not done yet" and loops forever under the `context.Background()` passed from `runPipeline`, so a successful spawn can still leave the pipeline hung indefinitely.

## Important

- `cmd/safe-ag/spawn.go:176`, `cmd/safe-ag/spawn.go:180`, `cmd/safe-ag/spawn.go:191`, `cmd/safe-ag/spawn.go:210`, `pkg/config/defaults.go:19`, `pkg/config/defaults.go:28`: most defaults parsed by `config.LoadDefaults` are never applied. `SAFE_AGENTIC_DEFAULT_SSH`, `SAFE_AGENTIC_DEFAULT_DOCKER`, `SAFE_AGENTIC_DEFAULT_DOCKER_SOCKET`, `SAFE_AGENTIC_DEFAULT_REUSE_AUTH`, `SAFE_AGENTIC_DEFAULT_REUSE_GH_AUTH`, and the explicit `GIT_*` author/committer fields are exposed through `config set/get`, but spawn only consumes memory, CPUs, PIDs, network, and `Identity`. That leaves a large dead config surface and makes the CLI silently ignore user defaults.

- `cmd/safe-ag/cron.go:317`: `executeCronJob` tokenizes `job.Command` with `strings.Fields`, so quoted arguments are destroyed. Any scheduled command that relies on grouped argv, including the documented `--prompt "Run tests"` style examples, is replayed incorrectly.

- `cmd/safe-ag/cron.go:335`, `cmd/safe-ag/cron.go:372`, `cmd/safe-ag/cron.go:381`: the scheduler does not implement real cron semantics, it only approximates schedules as durations. `daily 09:00` ignores the time entirely, and most 5-field cron expressions degrade to "hourly" unless they match one of two `*/N` cases. Jobs will run at the wrong wall-clock times.

- `cmd/safe-ag/cron.go:113`, `cmd/safe-ag/cron.go:129`, `cmd/safe-ag/cron.go:281`: cron state is updated with plain read/modify/write on `cron.json` and no locking or atomic replace. Running `cron add/remove/disable` while the daemon is writing `LastRun`/`LastErr` can lose one writer's changes.

- `cmd/safe-ag/workflow.go:123` and `cmd/safe-ag/workflow.go:191`: checkpoint creation commits a whole Docker image, but checkpoint restore only does `git stash pop`. The committed image is never referenced again, so "full container state" snapshots are not actually restorable and the extra image is effectively dead data.

- `pkg/fleet/manifest.go:201`: `mergeDefaults` cannot express "explicit false" for boolean fields. Once fleet or pipeline defaults set `ssh`, `reuse_auth`, `reuse_gh_auth`, `auto_trust`, `background`, or `docker` to true, an individual agent cannot opt back out because `false` is indistinguishable from "unset".

- `cmd/safe-ag/spawn.go:266`: fleet auth setup ignores both `docker volume create` and the seeding `docker run` errors. A failed seed silently degrades into an empty per-container auth volume, which breaks the intended shared-auth copy behavior and is difficult to diagnose.

- `cmd/safe-ag/observe.go:86`, `cmd/safe-ag/observe.go:91`, and `cmd/safe-ag/observe.go:101`: `runLogs` says it is finding the session log for the selected container, but it actually picks the newest JSONL under the repo path and explicitly discards `createdAt`. If multiple containers touch the same repo, `safe-ag logs` can show the wrong conversation.

## Minor

- `cmd/safe-ag/observe.go:46`, `cmd/safe-ag/observe.go:57`, and `cmd/safe-ag/observe.go:62`: `--follow` is registered for `safe-ag logs`, but `runLogs` never consults `logsFollow`. The flag is dead CLI surface today.

- `cmd/safe-ag/lifecycle.go:336`, `cmd/safe-ag/workflow.go:89`, `cmd/safe-ag/workflow.go:140`, `cmd/safe-ag/workflow.go:179`, `cmd/safe-ag/workflow.go:433`, and `cmd/safe-ag/workflow.go:493`: several commands advertise `--latest` in `Use`, but they never call `addLatestFlag`. `retry`, the checkpoint subcommands, `pr`, and `review` will reject `--latest` as an unknown flag unless the user passes the literal string positionally.

- `pkg/events/budget.go:3`, `pkg/events/notify.go:10`, `pkg/labels/labels.go:30`, `pkg/docker/container.go:12`, and `pkg/docker/volume.go:10`: these helpers are only referenced from tests, not from production code. They currently add maintenance surface without affecting runtime behavior; if they are intentional future hooks, they should be wired up or documented as such.
