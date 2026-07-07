# Terminal UI

`berth tui` is the fastest way to see all agents at once.

```bash
berth tui
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
| `c` | transfer files |
| `n` | spawn new agent |
| `p` | toggle preview |
| `/` | filter |
| `:` | command bar |
| `?` | help |
| `q` | quit |

## Important behaviors

- stopped containers still appear
- `attach` will restart a stopped container when needed
- on macOS, `attach` and `resume` open iTerm2 by default and fall back to Terminal.app when iTerm2 is not installed
- preview uses session/log fallbacks depending on what is available
- `:profile <name> [prompt]` runs a saved agent profile
- `:action <name>` runs a configured action in the selected agent
- `:comments` opens saved review comments for the selected agent
- `:timeline` and `:inbox` open event views
- filtering is case-insensitive
- narrow terminals automatically hide lower-priority columns

Use the TUI when you want keyboard-first local control.

## Spawn form

Press `n` to open the spawn form. It lets you set:
- type
- repo URL
- name
- prompt
- SSH
- auth reuse
- GitHub auth reuse
- host auth seeding
- AWS profile
- Docker access
- Docker socket access
- git identity

The spawn form starts from `~/.berth/config.toml` defaults. Unchecking a default-enabled risky option emits the matching `--no-*` flag for that session. If the final spawn would enable SSH, shared auth, host auth seeding, AWS credentials, Docker, or Docker socket access, the footer asks for `y/n` confirmation before launch. The spawned agent is launched in background mode; reconnect from the table when ready.

## File transfer

Press `c` to transfer files between the selected agent and the Apple container VM.

Agent paths are normalized and must stay under `/workspace`. This keeps the TUI transfer flow focused on repo artifacts instead of agent auth/config directories. VM paths must be absolute paths in the VM, for example `/tmp/report.txt`; relative paths and Docker `container:path` syntax are rejected.
