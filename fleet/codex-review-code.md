# Code Review Findings

## Critical

- `cmd/safe-ag/fleet.go:275-283`, `cmd/safe-ag/fleet.go:302-334`  
  Pipeline stages are marked complete as soon as their containers exit. `waitForContainers` never inspects `.State.ExitCode`, so a failed stage still unblocks every dependent stage and the runner cannot enforce the documented "wait for this step to succeed" behavior.

- `pkg/fleet/manifest.go:34-38`, `cmd/safe-ag/fleet.go:168-177`, `cmd/safe-ag/fleet.go:243-284`  
  `on_failure`, `retry`, `when`, and `outputs` are parsed and advertised, but the pipeline runner never reads any of them. Those manifest fields are dead config surface today, so manifests can look valid while the runtime silently ignores key orchestration controls.

- `cmd/safe-ag/cron.go:336-405`  
  The scheduler does not implement the wall-clock semantics it documents. `daily 09:00` is reduced to `24h`, cron expressions are collapsed into rough intervals, and `shouldRun` only checks `now.Sub(lastRun)`, so jobs drift instead of firing at the requested times.

- `cmd/safe-ag/cron.go:141-145`, `cmd/safe-ag/cron.go:317-333`  
  `cron add` stores the command as a flat string and `executeCronJob` later re-tokenizes it with `strings.Fields`. Quoted arguments such as `--prompt "Run tests"` or paths with spaces are split incorrectly when the job actually runs.

## Important

- `cmd/safe-ag/lifecycle.go:430-434`, `cmd/safe-ag/spawn.go:348-355`  
  `retry` tries to rebuild the original prompt from `SAFE_AGENTIC_PROMPT_B64`, but `spawn` never persists that env var. Retried agents therefore lose the original prompt unless the operator manually adds new context, so `retry` does not faithfully reproduce the previous run.

- `cmd/safe-ag/spawn.go:266-277`  
  Fleet auth isolation ignores both the per-container volume creation error and the seeding copy error. If either step fails, the agent still starts against an empty auth volume, turning an infrastructure failure into a confusing later authentication problem.

- `pkg/fleet/manifest.go:206-223`  
  `mergeDefaults` uses plain bools as if they were tri-state config. Once defaults set `ssh`, `reuse_auth`, `background`, `docker`, etc. to `true`, an individual agent cannot override them back to `false`, which is a surprising and non-idiomatic merge model for YAML configuration.

- `pkg/audit/audit.go:32-52`, `pkg/audit/audit.go:65-70`, `pkg/events/events.go:17-36`  
  Both JSONL appenders write without file locking or a single-writer coordinator. Concurrent CLI or cron processes can interleave writes, and `audit.Read` silently drops malformed lines, so a write race turns into silent audit/event data loss.

- `cmd/safe-ag/observe.go:87-109`  
  `safe-ag logs` fetches `createdAt` and repo metadata, then discards them and simply opens the newest JSONL under the repo/config path. Multiple containers for the same repo can therefore display the wrong session transcript.

- `cmd/safe-ag/workflow.go:123-128`, `cmd/safe-ag/workflow.go:150-214`  
  `checkpoint create` calls `docker commit`, but `checkpoint list` and `checkpoint revert` only interact with `git stash`. The committed image is never surfaced or restored, so the "full container state" capture is dead code and the feature over-promises what it can recover.

- `pkg/tmux/tmux.go:20-41`, `pkg/docker/container.go:12-17`, `pkg/docker/volume.go:10-15`  
  These helpers collapse any Docker error into "not present". That masks daemon and transport failures, makes `WaitForSession` spin for 60 seconds on infrastructure problems, and sends callers down the wrong recovery path.

## Minor

- `cmd/safe-ag/observe.go:55-57`, `cmd/safe-ag/observe.go:62-132`  
  `logs --follow` is registered but never read, so the CLI advertises streaming behavior it does not implement.

- `cmd/safe-ag/lifecycle.go:335-345`  
  `retry` says `[name|--latest]` in its usage string, but the command never registers `--latest`, so the advertised flag is actually unknown.

- `cmd/safe-ag/config_cmd.go:560`, `cmd/safe-ag/config_cmd.go:582-585`  
  `mustReadFile` is both misnamed and redundant: it swallows read errors and returns `nil`, which is the opposite of normal Go `Must*` semantics, and `runAWSRefresh` had already validated the same file earlier in the function.
