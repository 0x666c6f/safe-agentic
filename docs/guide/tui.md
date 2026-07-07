# Terminal UI

`berth tui` is the fastest way to see every agent at once — a k9s-style dashboard for keyboard-first control.

```bash
berth tui
```

## What it shows

- **header** — VM context and running/total agents
- **table** — every agent (stopped ones included) with live state, resource usage, and network/auth metadata
- **preview pane** — the selected agent's recent output (`p` to toggle)
- **footer** — key hints, filter bar, command bar, status messages

## The core loop

From the table, everything is one key away: `Enter` attaches, `s` stops, `f` shows the diff, `R` runs a review, `g` opens a PR, `n` opens the spawn form. `/` filters, `:` opens the command bar (`:profile`, `:action`, `:comments`, `:timeline`, `:inbox`), `?` shows help.

The full keybinding and command table lives in the [TUI reference](../reference/tui.md).

## Behaviors worth knowing

- attaching to a stopped container restarts it first
- on macOS, attach/resume open iTerm2 when installed, Terminal.app otherwise
- narrow terminals hide lower-priority columns automatically
- spawned agents launch in background mode; reconnect from the table when ready

## Spawn form

`n` opens a form covering type, repo, name, prompt, and the auth/runtime toggles (SSH, auth reuse, gh auth, seeded auth, AWS, Docker, Docker socket, identity). It starts from your `config.toml` defaults; unchecking a default-enabled risky option emits the matching `--no-*` flag for that session. If the final spawn widens anything risky, the footer asks for `y/n` confirmation before launch.

## File transfer

`c` transfers files between the selected agent and the VM. Agent paths must stay under `/workspace`; VM paths must be absolute. This keeps transfers focused on repo artifacts, not agent auth/config directories.

## When to use what

| Situation | Surface |
|---|---|
| scripting, one-off commands, CI | [CLI](../reference/cli.md) |
| several live agents, terminal-native | TUI |
| embedded terminals, native notifications | [Desktop app](app.md) |
