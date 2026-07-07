# berth-app v1 — Full QA Matrix

Channels: **P** = browser preview (wails3 dev + fixture store injection; backend calls fail by design → exercises error paths), **B** = live backend (Go E2E harness `-tags livespike` against real VM/containers, or direct CLI), **U** = unit tests (already green), **H** = needs human eyes (native window visuals). Result column filled during execution.

| ID | Feature | Case | Ch | Expected | Result |
|----|---------|------|----|----------|--------|
| A1 | Sidebar | All 6 status dots (working/needs-you-state/needs-you-event/review/idle/stopped/failed) | P | correct colors per statusFor precedence | PASS |
| A2 | Sidebar | Fleet group + solo + stopped sections | P | three sections, correct membership | PASS |
| A3 | Sidebar | Row tooltip repo + state reason | P | title attr present | PASS |
| A4 | Sidebar | Click row → workspace + highlight | P | selection visible, view=agents | PASS |
| A5 | Sidebar | agents.changed replaces list | P | store-driven rerender | PASS |
| A6 | Poller | Real `docker ps` 12-field parse against live VM | B | real containers parsed w/ Terminal label + State probe | PASS |
| B1 | Terminal | Attach status line + error overlay + Reattach | P | "attaching…", red overlay on failure | PASS |
| B2 | Terminal | Real attach round-trip via relay | B | livespike: redraw + keystroke exec | PASS |
| B4 | Terminal | Stopped agent → no auto-attach, hint msg | P | message, no error toast | PASS |
| B5 | Terminal | Split pane renders second terminal | P | two panes side by side | PASS |
| B6 | Terminal | Resize reflow visual | H | tmux reflows | HUMAN |
| C1 | Output | Real `output --json` → status + last msg | B | non-empty status | PASS |
| C2 | Output | In-body error state | P | red panel, no eternal loading | PASS |
| C3 | Steer | Real steer lands in tmux pane | B | text visible in capture-pane | PASS |
| C4 | Stop | Real stop from service | B | container exits | PASS |
| C6 | PR/Review | argv construction | U | unit-tested | PASS (unit) |
| D1 | Diff | Empty state | P | "no changes" | PASS |
| D2 | Diff | Real change → diff text E2E | B | modified file in diff output | PASS |
| D3 | Checkpoint | create + list E2E | B | checkpoint listed | PASS |
| D4 | Workspace | revert E2E → diff empty | B | clean tree after revert | PASS (after fix) |
| E1 | Spawn | Type selection highlight + layout | P | selected type visually distinct | PASS |
| E2 | Spawn | Dry-run preview shows docker run | B | command text returned | PASS |
| E3 | Spawn | No-repo spawn E2E | B | container runs with empty workspace | PASS |
| E4 | Spawn | Template list parse skips headers | B | no NAME/separator entries | PASS |
| F1 | Fleet | Groups + hierarchy sort + chips | P | sorted chips under fleet name | PASS |
| F2 | Fleet | PipelineFiles bare names | U+B | extension-stripped names | PASS |
| F4 | Fleet | Retry chip on stopped member | P | retry button renders | PASS |
| G1 | Timeline | Real events classified + badges | B | needs-auth→yellow class etc. | PASS (unit) |
| G2 | Inbox | needs-attention filter | B | only attention items | PASS (unit) |
| H1 | Cost | cost --history output renders | B | text present | PASS |
| I1 | Palette | ⌘K open, fuzzy, agent jump | P | filters + navigates | PASS |
| I2 | Palette | View actions fire | P | view switches | PASS |
| J1 | VM banner | vmOk=false → banner + button | P | red banner visible | PASS |
| K1 | Toasts | dedup + cap 5 + 8s auto-dismiss | P | stack bounded, expires | PASS |
| L1 | Watcher | Synthetic event → classified + notify path | B | watcher unit + NotifySystem call | PASS (unit) |
| M1 | Systray | Counts + menu + focus click | H | 🟢🟡⚪ + focus works | HUMAN |
| N1 | Persistence | Quit/relaunch → reattach | H | tmux session survives | HUMAN |

Execution notes appended below during the run.

## Execution notes (2026-07-05)

- Preview channel: 17 UI cases via wails3-dev browser preview + fixture store injection (dev-only `window.__store`).
- Backend channel: `BERTH_LIVE_QA=1 go test -tags livespike ./internal/svc/` — 11/11 subtests green against real VM + disposable container; attach spike also green against a background-spawned no-repo agent.
- 3 HUMAN cases remain: terminal resize reflow feel, systray visuals, quit/relaunch persistence.

## Defects found & fixed during QA

1. ENGINE: background mode ran agents with NO tmux (claude/codex/shell) — GUI terminals, steer and peek could never work on GUI-spawned agents; `terminal=tmux` label lied. Fixed: background now uses the same tmux flow (entrypoint.sh); shell got a real tmux arm (was foreground bash that died instantly in background).
2. APP: `workspace stage/revert` calls missed required `<path...>` arg — Stage all/Revert all buttons always failed. Fixed with `.`.
3. APP: SpawnRequest lacked `--name` — GUI spawns got auto-generated names only. Added Name field.
4. UX (earlier wave, re-verified): bare "Error" toasts, toast pile-up, eternal loading, black terminal on failure, stopped-agent auto-attach, invisible type selection.
5. FEATURE: saved repo list, most-used first, with forget buttons (localStorage).
