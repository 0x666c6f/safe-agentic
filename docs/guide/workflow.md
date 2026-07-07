# Review & Ship

The "agent did work — now what?" loop: inspect, correct, review, and get it merged.

## Inspect the result

```bash
berth summary --latest    # quick state: repo, status, cost, last message
berth output --latest     # last meaningful message
berth diff --latest       # the code change (add --stat or -s for side-by-side)
berth review --latest     # AI review pass against a base branch
```

## Steer or retry

Course-correct a live agent without attaching:

```bash
berth steer --latest "Focus only on the failing test and avoid unrelated refactors"
```

Start over with the same session shape:

```bash
berth retry --latest
berth retry --latest --feedback "Focus only on src/ and add tests"
berth retry --latest --resume     # continue the conversation instead of re-running
```

## Review comments

File/line notes that survive between `review`, `diff`, `steer`, and PR handoff (stored on the host under `~/.berth/state/`):

```bash
berth review-comments add --latest cmd/main.go 42 "Handle empty input before parsing"
berth review-comments list --latest
berth steer --latest "Address the open review comments, then run tests"
berth review-comments resolve rc-123
```

## Checkpoints

Reversible snapshots before risky changes:

```bash
berth checkpoint create --latest "before big refactor"
berth checkpoint list --latest
berth checkpoint restore --latest stash@{0}
```

## Stage or revert selectively

Take only part of what the agent did:

```bash
berth workspace stage --latest src/api.go src/api_test.go
berth workspace unstage --latest src/api.go
berth workspace revert --latest scratch.txt --yes
berth workspace stage-patch --latest ./picked-hunks.patch
```

## Todos

The built-in merge checklist:

```bash
berth todo add --latest "Run integration tests"
berth todo list --latest
berth todo check --latest 1
```

## Open a PR

```bash
berth pr --latest --title "fix: stabilize flaky tests" --base main
```

Before `berth pr` you usually want SSH enabled at spawn time, GitHub auth in the container (`--reuse-gh-auth`), and todos completed.

### One-shot PR review workflows

Run a whole review pipeline against a PR without hand-building a manifest:

```bash
berth pr-review               # dual Claude+Codex review of the current PR
berth pr-review codex 128     # single-reviewer mode, explicit PR number
berth pr-fix 128              # spawn the fix workflow for a PR
```

Both infer the repo from the current checkout; pass `--repo` to override.

## Hand the work off

Get the workspace out of the container:

```bash
berth handoff --latest --to-local ./workspace-copy   # copy /workspace to a host path
berth handoff --latest --to-worktree                 # print the managed worktree path
```

`--to-worktree` only applies to agents spawned with `--worktree` — see [Worktrees](worktrees.md).

## Typical end-to-end loop

```bash
berth spawn claude --ssh --reuse-auth \
  --repo git@github.com:org/api.git \
  --prompt "Fix the failing CI tests"

berth peek --latest
berth steer --latest "Keep the fix narrow and add one regression test"
berth diff --latest
berth review --latest
berth todo add --latest "Verify locally"
berth todo check --latest 1
berth pr --latest --title "fix: resolve CI failures"
```

## Related

- [Monitor & Manage](managing.md) — attach, logs, cost, cleanup
- [Automation](automation.md) — hooks, notifications, scheduled runs
- [Fleets & Pipelines](fleet.md) — multi-agent orchestration
