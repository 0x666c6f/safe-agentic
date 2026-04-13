# Terminal UI

`safe-ag tui` is the fastest way to see all agents at once.

```bash
safe-ag tui
```

## What it shows

- header: VM context and running/total agents
- table: agent list, state, resource usage, network/auth metadata
- footer: key hints, filter bar, command bar, status messages
- optional preview pane for the selected agent

## What you can do from the TUI

| Key | Action |
|---|---|
| `Enter` / `a` | attach |
| `r` | resume |
| `s` | stop |
| `l` | logs |
| `d` | inspect |
| `f` | diff |
| `R` | review |
| `t` | todos |
| `x` | checkpoint |
| `g` | create PR |
| `e` | export sessions |
| `c` | copy files |
| `n` | spawn new agent |
| `p` | toggle preview |
| `/` | filter |
| `:` | command bar |
| `?` | help |
| `q` | quit |

## Important behaviors

- stopped containers still appear
- `attach` will restart a stopped container when needed
- preview uses session/log fallbacks depending on what is available
- filtering is case-insensitive
- narrow terminals automatically hide lower-priority columns

## Dashboard

There is also a web dashboard:

```bash
safe-ag dashboard --bind localhost:8420
```

Use the TUI when you want keyboard-first local control. Use the dashboard when you want a browser view.

## Spawn form

Press `n` to open the spawn form. It lets you set:
- type
- repo URL
- name
- prompt
- SSH
- auth reuse
- GitHub auth reuse
- AWS profile
- Docker access
- git identity

The spawned agent is launched in background mode; reconnect from the table when ready.
