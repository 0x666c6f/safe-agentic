# TUI Reference

This page is the reference for `safe-ag tui` and `safe-ag dashboard`.

## Entry points

Terminal UI:

```bash
safe-ag tui
```

Web dashboard:

```bash
safe-ag dashboard --bind localhost:8420
```

Binary help:

```bash
safe-ag-tui --help
```

Current top-level help:

```text
Usage: safe-ag-tui [--dashboard [--bind host:port]]
```

## Main regions

The TUI has four functional areas:

| Area | Purpose |
|---|---|
| header | VM context and running/total agents |
| table | all agents and live state |
| footer | key hints, input modes, status |
| preview pane | optional output preview for the selected agent |

## Main actions

| Key | Action |
|---|---|
| `Enter` / `a` | attach |
| `r` | resume |
| `s` | stop selected agent |
| `Ctrl-d` | stop selected agent |
| `Ctrl-k` | stop all agents |
| `n` | open spawn form |
| `l` | logs overlay |
| `d` | inspect overlay |
| `y` | raw inspect output |
| `f` | diff |
| `R` | review |
| `t` | todo list |
| `x` | checkpoint create |
| `g` | create PR |
| `e` | export sessions |
| `c` | copy files |
| `m` | MCP login |
| `$` | cost |
| `A` | audit |
| `p` | toggle preview pane |
| `/` | filter mode |
| `:` | command mode |
| `?` | help overlay |
| `q` / `Ctrl-c` | quit |

## Navigation

| Key | Action |
|---|---|
| `j` / `Down` | next row |
| `k` / `Up` | previous row |
| `1`-`9` | sort by column |
| `Esc` | close overlay or exit input mode |

## Modes

The footer switches between modes:

| Mode | Meaning |
|---|---|
| shortcuts | normal mode |
| filter | free-text filter input |
| command | command bar |
| confirm | destructive action confirmation |
| status | transient status message |

## Preview behavior

When preview is enabled:
- running tmux-backed agents try pane capture first
- stopped agents fall back to logs/session data
- preview updates as selection changes

## Spawn form fields

The TUI spawn form currently exposes:

| Field | Meaning |
|---|---|
| Type | `claude` or `codex` |
| Repo URL | optional repo to clone |
| Name | optional session name |
| Prompt | optional task |
| SSH | enable SSH forwarding |
| Reuse auth | persist Claude/Codex auth |
| Reuse GH auth | persist GitHub CLI auth |
| AWS profile | inject AWS credentials |
| Docker | enable DinD |
| Identity | explicit git identity |

Spawned agents are launched in background mode; reconnect from the list when ready.

## When to use TUI vs CLI

Use the TUI when:
- you have multiple agents running
- you want live resource/activity visibility
- you want keyboard-driven operations

Use the CLI when:
- you already know the target container
- you are scripting
- you need exact command output for automation
