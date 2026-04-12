# Safe-Agentic CLI & Library Reference
## Comprehensive Go Rewrite Source of Truth

---

## 1. ARCHITECTURE OVERVIEW

### Runtime Stack
- **Host**: macOS with OrbStack VM
- **VM Image**: safe-agentic (Linux, hardened, immutable)
- **Container Runtime**: Docker inside OrbStack VM
- **Agent Entry**: tmux sessions (Claude/Codex) or direct shell
- **CLI**: Bash scripts in `/bin/` → Go rewrite target

### Directory Structure
```
safe-agentic/
├── bin/
│   ├── agent              # Main CLI dispatcher (bash)
│   ├── agent-lib.sh       # Shared functions & Docker runtime builders
│   ├── agent-session.sh   # Container entrypoint session launcher
│   ├── agent-claude       # Alias: spawn claude agent
│   ├── agent-codex        # Alias: spawn codex agent
│   ├── agent-alias        # CLI parser for above aliases
│   ├── agent-tui          # Pre-compiled TUI dashboard (Go binary)
│   ├── docker-runtime.sh  # Docker-in-Docker setup
│   └── repo-url.sh        # Git repo URL parsing
├── entrypoint.sh          # Container entry: config + repo clone + agent launch
├── config/                # Baked-in configs
│   ├── seccomp.json       # Seccomp profile
│   ├── security-preamble.md
│   ├── bashrc
│   └── tmux.conf
├── templates/             # Agent prompt templates
├── examples/              # Fleet & pipeline YAML examples
├── tui/                   # TUI Go code (agent-tui binary)
└── tests/                 # Test suite with fake orb binary
```

---

## 2. bin/agent — ALL cmd_* FUNCTIONS

### Function Dispatch Pattern
- `cmd_<action>()` functions defined at specific line numbers
- Parsed from command-line in `main_dispatch()`
- Each parses own arguments, calls lib.sh functions

### Mapping: Line # → Function → Docker Ops

#### **cmd_template** (line 675)
- **Purpose**: List or render agent prompt templates
- **Args**: template name
- **Docker ops**: None
- **Env vars read**: None
- **Files I/O**: Read from `/templates/` directory on host

#### **cmd_setup** (line 740)
- **Purpose**: Create/harden OrbStack VM, build Docker image
- **Docker ops**: 
  - `docker build -t safe-agentic:latest .`
  - `docker run` for image validation
- **VM hardening**:
  - Copy seccomp.json to VM
  - Set up SSH relay sockets
  - Verify VM hardening profile

#### **cmd_run** (line 834)
- **Purpose**: High-level "run agent with prompt on repo"
- **Dispatches to**: `cmd_spawn` with auto-detected args
- **Docker ops**: Indirect (via spawn)
- **Logic**: 
  - Takes: `<repo-url> [repo-url...] "prompt"`
  - Auto-detects SSH from repo URLs
  - Auto-detects git identity
  - Builds spawn args

#### **cmd_spawn** (line 944) [CRITICAL]
- **Purpose**: Main agent container launcher
- **Arguments**:
  - `<claude|codex>` — agent type (required)
  - `--repo URL` — can repeat for multiple repos
  - `--name NAME` — container name suffix
  - `--prompt TEXT` — agent prompt (base64 encoded in label)
  - `--template FILE` — load prompt from template
  - `--ssh` — enable SSH agent forwarding
  - `--reuse-auth / --ephemeral-auth` — auth volume lifetime
  - `--reuse-gh-auth` — GitHub CLI auth volume
  - `--docker` / `--docker-socket` — Docker access mode
  - `--network NAME` — custom Docker network
  - `--memory SIZE` — container memory limit
  - `--cpus N` — CPU limit
  - `--pids-limit N` — PID limit
  - `--identity NAME <email>` — git author identity
  - `--auto-trust` — auto-trust /workspace in Claude
  - `--background` — non-interactive mode
  - `--aws PROFILE` — inject AWS credentials
  - `--instructions TEXT` — agent-specific instructions
  - `--on-exit CMD` — callback on container exit
  - `--on-complete CMD` — callback on success (exit 0)
  - `--on-fail CMD` — callback on failure (exit non-zero)
  - `--max-cost DOLLARS` — budget limit
  - `--notify TARGETS` — notification sinks
  - `--dry-run` — show command without running
  - `--fleet-volume NAME` — shared fleet volume

- **Docker Commands Run**:
  ```bash
  docker run -it \
    --name <container-name> \
    --hostname <container-name> \
    --pull=never \
    --network <network-name> \
    --cpus <cpus> \
    --memory <memory> \
    --pids-limit <limit> \
    --cap-drop=ALL \
    --security-opt=no-new-privileges:true \
    --security-opt "seccomp=/etc/safe-agentic/seccomp.json" \
    --read-only \
    --ulimit nofile=65536:65536 \
    --tmpfs /tmp:rw,noexec,nosuid,size=512m \
    --tmpfs /var/tmp:rw,noexec,nosuid,size=256m \
    --tmpfs /run:rw,noexec,nosuid,size=16m \
    --tmpfs /dev/shm:rw,noexec,nosuid,size=64m \
    -v <auth-volume>:/home/agent/.<agent-type> \
    -v <gh-auth-volume>:/home/agent/.config/gh \
    --mount type=volume,dst=/workspace \
    --mount type=volume,dst=/home/agent/.npm \
    --mount type=volume,dst=/home/agent/.cache/pip \
    --mount type=volume,dst=/home/agent/go \
    --mount type=volume,dst=/home/agent/.terraform.d/plugin-cache \
    [--ssh-mount] \
    [--docker-mount] \
    [--network-mount] \
    -e GIT_AUTHOR_NAME=... \
    -e GIT_AUTHOR_EMAIL=... \
    -e GIT_COMMITTER_NAME=... \
    -e GIT_COMMITTER_EMAIL=... \
    -e AGENT_TYPE=<type> \
    -e REPOS=<comma-joined> \
    -e GIT_CONFIG_GLOBAL=/home/agent/.config/git/config \
    -e SAFE_AGENTIC_TMUX_SESSION_NAME=safe-agentic \
    [--label safe-agentic.*] \
    safe-agentic:latest
  ```

- **Labels Set** (all via `append_runtime_hardening`):
  - `app=safe-agentic`
  - `safe-agentic.type=container`
  - `safe-agentic.agent-type=<claude|codex|shell>`
  - `safe-agentic.repo-display=<repo-label>`
  - `safe-agentic.ssh=<on|off>`
  - `safe-agentic.network-mode=<managed|custom|none>`
  - `safe-agentic.terminal=<tmux|direct>`
  - `safe-agentic.auth=<shared|ephemeral>`
  - `safe-agentic.gh-auth=<shared|ephemeral>`
  - `safe-agentic.docker=<off|dind|host-socket>`
  - `safe-agentic.prompt=<base64>`
  - `safe-agentic.instructions=<1 if present>`
  - `safe-agentic.on-exit=<1 if present>`
  - `safe-agentic.on-complete-b64=<base64>`
  - `safe-agentic.on-fail-b64=<base64>`
  - `safe-agentic.max-cost=<DOLLARS>`
  - `safe-agentic.notify-b64=<base64>`
  - `safe-agentic.mcp-oauth=<true if configured>`

- **Env Vars Set**:
  - `AGENT_TYPE` — passed to entrypoint
  - `REPOS` — comma-joined repo list
  - `GIT_*` — author/committer from defaults or auto-detect
  - `GH_CONFIG_DIR=/home/agent/.config/gh`
  - `GIT_CONFIG_GLOBAL=/home/agent/.config/git/config`
  - `SSH_AUTH_SOCK=/run/ssh-agent.sock` (if --ssh)
  - `DOCKER_HOST=unix://...` (if --docker)
  - `SAFE_AGENTIC_INSTRUCTIONS_B64` (if --instructions)
  - `SAFE_AGENTIC_ON_EXIT_B64` (if --on-exit)
  - `SAFE_AGENTIC_ON_COMPLETE_B64` (if --on-complete)
  - `SAFE_AGENTIC_ON_FAIL_B64` (if --on-fail)
  - `SAFE_AGENTIC_NOTIFY_B64` (if --notify)
  - `SAFE_AGENTIC_AWS_CREDS_B64` (if --aws)
  - `SAFE_AGENTIC_AUTO_TRUST` (if --auto-trust)
  - `SAFE_AGENTIC_BACKGROUND` (if --background)
  - `SAFE_AGENTIC_CODEX_CONFIG_B64` (if codex + host config)
  - `SAFE_AGENTIC_CLAUDE_CONFIG_B64` (if claude + host config)
  - `SAFE_AGENTIC_CLAUDE_SUPPORT_B64` (if claude + support files)

- **Files Read on Host**:
  - `$HOME/.codex/config.toml` (if codex agent)
  - `$CLAUDE_CONFIG_DIR/settings.json` (if claude agent)
  - `$CLAUDE_CONFIG_DIR/CLAUDE.md`, `hooks/`, `commands/` (claude support)
  - `$HOME/.aws/credentials` (if --aws)
  - `$HOME/.ssh/config`, `known_hosts` (if --ssh)
  - Git config for auto-detect

- **Files Written in Container** (during entrypoint):
  - `/workspace/.safe-agentic/` — session state dir
  - `/home/agent/.config/git/config` — git config
  - `/home/agent/.codex/config.toml` — codex config (decoded)
  - `/home/agent/.claude/settings.json` — claude config (decoded)
  - `/home/agent/.aws/credentials` — AWS creds (decoded)
  - `/workspace/AGENT-INSTRUCTIONS.md` — injected instructions

#### **cmd_shell** (line 1370)
- **Purpose**: Interactive shell in agent container
- **Args**: `[container-name]`, optional commands
- **Docker ops**:
  - `docker exec -it <container> bash -il [args]`
- **Env vars**: Container's env

#### **cmd_list** (line 1559)
- **Purpose**: List all safe-agentic containers
- **Docker ops**:
  - `docker ps -a --filter "name=^agent-" --format 'table ...'`
  - Or with `--json`: `docker ps -a --filter ... --format '{{json .}}'`

#### **cmd_attach** (line 1677)
- **Purpose**: Attach to running agent's tmux session
- **Docker ops**:
  - `docker exec <container> tmux attach-session -t safe-agentic`
- **Env**: Passes through terminal

#### **cmd_cp** (line 1735)
- **Purpose**: Copy files from agent container to host
- **Args**: `<container>:<path> <host-destination>`
- **Docker ops**:
  - `docker exec <container> tar -cf - /path > <host.tar>`
  - `docker cp <container>:/tmp/<staging> <host>`
  - `docker rm <staging>` after extract
- **Complexity**: Handles tar staging to bypass read-only root

#### **cmd_stop** (line 1808)
- **Purpose**: Stop a specific agent container
- **Args**: `[container-name]` or `--latest`
- **Docker ops**:
  - `docker stop <container-name>`
  - `docker rm <container-name>`
  - `docker network rm <managed-network>` (if managed)
- **Audit logging**: Calls `audit_log "stop" <name>`

#### **cmd_cleanup** (line 1878)
- **Purpose**: Stop all agent containers & clean up networks/volumes
- **Docker ops**:
  - `docker ps -q --filter "name=^agent-"` → get running IDs
  - `docker stop <ids>`
  - `docker ps -aq --filter "name=^agent-"` → get all IDs
  - `docker rm <ids>`
  - `docker network ls -q --filter "label=..." --filter "label=safe-agentic.type=container-network"`
  - `docker network rm <networks>`
  - Cleanup Docker runtimes (see docker-runtime.sh)
  - `docker volume rm` managed volumes

#### **cmd_update** (line 1940)
- **Purpose**: Update Docker image from Dockerfile
- **Docker ops**:
  - `docker build -t safe-agentic:latest .`
- **Rebuilds**: Image and validates

#### **cmd_diagnose** (line 2005)
- **Purpose**: Debug info about VM, Docker, agent containers
- **Docker ops**:
  - `docker ps -a`
  - `docker network ls`
  - `docker volume ls`
  - `docker inspect` per container
- **Checks**: VM running, Docker socket, image present

#### **cmd_vm** (line 2075)
- **Purpose**: Manage OrbStack VM (create, delete, harden)
- **Sub-commands**:
  - `vm create` — create VM, set up SSH
  - `vm delete` — destroy VM
  - `vm harden` — apply security hardening
  - `vm harden-verify` — check hardening applied
- **Docker ops**: None directly (VM-level)

#### **cmd_summary** (line 2129)
- **Purpose**: Show container runtime config summary
- **Reads**: Docker labels and inspects for config details

#### **cmd_config** (line 2308)
- **Purpose**: Manage defaults config file
- **File**: `$XDG_CONFIG_HOME/safe-agentic/defaults.sh`
- **Allowed keys**:
  - `SAFE_AGENTIC_DEFAULT_CPUS`
  - `SAFE_AGENTIC_DEFAULT_MEMORY`
  - `SAFE_AGENTIC_DEFAULT_PIDS_LIMIT`
  - `SAFE_AGENTIC_DEFAULT_NETWORK`
  - `SAFE_AGENTIC_DEFAULT_SSH`
  - `SAFE_AGENTIC_DEFAULT_DOCKER`
  - `SAFE_AGENTIC_DEFAULT_REUSE_AUTH`
  - `SAFE_AGENTIC_DEFAULT_REUSE_GH_AUTH`
  - `GIT_AUTHOR_NAME`, `GIT_AUTHOR_EMAIL`
  - `GIT_COMMITTER_NAME`, `GIT_COMMITTER_EMAIL`

#### **cmd_help** (line 2382)
- **Purpose**: Show help for commands

#### **cmd_mcp_login** (line 2511)
- **Purpose**: OAuth flow for MCP servers inside container
- **Publishes container port** range (default: 9000-9100)

#### **cmd_sessions** (line 2619)
- **Purpose**: List active tmux sessions from containers

#### **cmd_peek** (line 2731)
- **Purpose**: Peek at container logs without attaching
- **Docker ops**: `docker logs <container>`

#### **cmd_diff** (line 2776)
- **Purpose**: Show uncommitted changes in cloned repos
- **Docker ops**: `docker exec <container> git diff`

#### **cmd_checkpoint** (line 2825)
- **Purpose**: Checkpoint container state to branch
- **Docker ops**:
  - `docker commit <container> <new-image>`
  - `docker run` from checkpoint
  - Git operations inside container

#### **cmd_todo** (line 3016)
- **Purpose**: Manage todo list in container
- **Docker ops**: `docker exec` to manage files

#### **cmd_review** (line 3174)
- **Purpose**: Request GitHub PR review

#### **cmd_audit** (line 3236)
- **Purpose**: Show audit log (append-only JSONL)
- **File**: `$SAFE_AGENTIC_AUDIT_LOG` or `~/.config/safe-agentic/audit.jsonl`
- **Format**: One JSON object per line:
  ```json
  {"timestamp":"2026-04-10T12:34:56Z","action":"spawn","container":"agent-claude-1234","details":"..."}
  ```

#### **cmd_cost** (line 3283)
- **Purpose**: Compute API costs from session JSONL files
- **Logic**:
  - Reads `~/.claude/session-events.jsonl` or similar
  - Parses model + token counts
  - Multiplies by pricing table
  - Sums total cost in USD

#### **cmd_replay** (line 3657)
- **Purpose**: Replay agent session from events file

#### **cmd_output** (line 3865)
- **Purpose**: Dump container logs/output

#### **cmd_fleet** (line 4032) [COMPLEX]
- **Purpose**: Spawn multiple agents in parallel from YAML manifest
- **File format**: YAML with `agents:` list
- **Parsing**: Python3 inline (no YAML lib dependency)
- **Per-agent fields**:
  - `name`, `type`, `repo`, `ssh`, `reuse_auth`, `reuse_gh_auth`
  - `docker`, `auto_trust`, `background`
  - `prompt`, `aws`, `network`, `memory`, `cpus`, `pids_limit`
  - `identity`, `max_cost`, `notify`
- **Sub-command**: `fleet status` — query running fleet container state
- **Docker ops**:
  - Creates shared named volume `<fleet-name>-volume` with `--fleet-volume` flag
  - Labels: `safe-agentic.fleet=<name>`
  - Spawns multiple containers in parallel

#### **cmd_pipeline** (line 4360) [VERY COMPLEX]
- **Purpose**: Sequential agent tasks with dependencies
- **File format**: YAML with `steps:` list
- **Per-step fields**:
  - `name`, `type`, `repo`, ... (same as fleet)
  - `depends_on` — list of step names
  - `retry` — retry count
  - `timeout` — step timeout
  - `on_failure` — action if step fails
- **Execution**:
  - Topological sort by `depends_on`
  - Wait for deps to complete
  - Spawn, monitor, collect exit code
  - If `on_failure: stop`, abort pipeline
  - If `on_failure: continue`, skip to next available
- **Docker ops**: Spawns sequentially, passes workspace between steps

#### **cmd_orchestrate** (line 4661)
- **Purpose**: Advanced orchestration (fleet + pipeline combined)

#### **cmd_pr** (line 4750)
- **Purpose**: Create PR from agent work

#### **cmd_aws_refresh** (line 4859)
- **Purpose**: Refresh AWS credentials for running container
- **Docker ops**: `docker exec` to update credentials

#### **cmd_retry** (line 4949)
- **Purpose**: Retry failed agent from checkpoint/label
- **Retrieves**: Prompt from label, relaunches

---

## 3. bin/agent-lib.sh — ALL EXPORTED FUNCTIONS

### Validation Functions

#### `validate_name_component(value, label)` (line 124)
- **Regex**: `^[A-Za-z0-9][A-Za-z0-9_.-]*$`
- **Checks**: Container names, volume names, etc.

#### `validate_pids_limit(value)` (line 132)
- **Regex**: `^[0-9]+$`
- **Min**: 64 (dies if < 64)

#### `validate_network_name(value)` (line 138)
- **Rejects**: `bridge`, `host`, `container:*` (reserved)
- **Allows**: `none`, or custom name matching regex
- **Regex**: `^[A-Za-z0-9][A-Za-z0-9_.-]*$`

### Configuration Functions

#### `load_user_defaults()` (line 36)
- **File**: `$SAFE_AGENTIC_DEFAULTS_FILE` or `$XDG_CONFIG_HOME/safe-agentic/defaults.sh`
- **Format**:
  ```bash
  # Simple KEY=value (lines 40-44)
  SAFE_AGENTIC_DEFAULT_CPUS=8
  SAFE_AGENTIC_DEFAULT_MEMORY="16g"
  GIT_AUTHOR_NAME="Agent Bot"
  GIT_AUTHOR_EMAIL="bot@example.com"
  ```
- **Parsing**: Calls `parse_defaults_line()` per line
- **Allowed keys**: Whitelist in `default_key_allowed()` (line 58)

#### `parse_defaults_line(line, line_no)` (line 96)
- **Handles**:
  - Comments: `# ...`
  - `export KEY=value` syntax
  - Quoted values: `"..."` or `'...'`
  - Unquoted simple values
- **Errors**: Dies with line number if invalid

#### `detect_git_identity()` (line 959)
- **Reads**:
  - `git config --global user.name`
  - `git config --global user.email`
- **Returns**: `"Name <email>"` or empty string

#### `apply_identity(identity_string)` (line 179)
- **Parses**: `"Name <email@example.com>"` format
- **Sets**:
  - `GIT_AUTHOR_NAME`, `GIT_AUTHOR_EMAIL`
  - `GIT_COMMITTER_NAME`, `GIT_COMMITTER_EMAIL`
- **Exports**: All set to environment

#### `apply_default_identity()` (line 193)
- **Reads**: `$SAFE_AGENTIC_DEFAULT_IDENTITY`
- **Calls**: `apply_identity()` if set and no identity in env

### Container Resolution Functions

#### `container_exists(container_name)` (line 230)
- **Docker**: `docker ps -aq --filter "name=^<name>$"`
- **Returns**: 0 if exists, 1 otherwise

#### `resolve_container_name(prefix, explicit_name, fallback, [repos...])` (line 258)
- **Logic**:
  - If explicit_name: use it
  - Else if repos: generate from repo slug
  - Else: use fallback
  - Full name: `<prefix>-<suffix>`
  - If exists + exited: `docker rm` old one
  - If exists + running: append timestamp to avoid conflict

#### `resolve_container_reference(name)` (line 296)
- **Matches**: Full or partial container name
- **Returns**: First matching container name

#### `resolve_latest_container()` (line 292)
- **Docker**: `docker ps -a --latest --filter "name=^agent-" --format '{{.Names}}'`
- **Returns**: Most recent container name

### Network Functions

#### `create_managed_network(network_name)` (line 459)
- **Docker**:
  ```bash
  docker network create \
    --driver bridge \
    --opt "com.docker.network.bridge.name=$bridge_name" \
    --opt com.docker.network.bridge.enable_icc=false \
    --label "app=$IMAGE_NAME" \
    --label "safe-agentic.type=container-network" \
    "$network_name"
  ```
- **Bridge naming**: `br-<network_name>-<hash>`
- **Inter-container comms**: DISABLED (enable_icc=false)

#### `remove_managed_network(network_name)` (line 479)
- **Docker**: `docker network rm <name>`
- **Safety**: Only removes if label `safe-agentic.type=container-network`

#### `prepare_network(managed, container_name, custom_name, dry_run)` (line 835)
- **Orchestrates**:
  - If managed: calls `create_managed_network()`
  - If custom: calls `ensure_custom_network()`
  - Sets `network_name` variable

#### `ensure_custom_network(network_name)` (line 451)
- **Docker**: `docker network inspect` → exists check
- **Creates**: If missing

### Volume Functions

#### `auth_volume_exists(volume_name)` (line 345)
- **Docker**: `docker volume inspect <name> >/dev/null 2>&1`
- **Returns**: 0 if exists

#### `volume_contains_file(volume_name, file_path)` (line 351)
- **Docker**: `docker run --rm -v <vol>:/ ... test -f <file>`
- **Checks**: File inside named volume

#### `append_ephemeral_volume(destination)` (line 523)
- **Appends** to `docker_cmd[]`:
  ```
  --mount "type=volume,dst=<destination>"
  ```

#### `append_named_volume(source, destination)` (line 528)
- **Appends** to `docker_cmd[]`:
  ```
  --mount "type=volume,src=<source>,dst=<destination>"
  ```

#### `append_cache_mounts()` (line 742)
- **Mounts** (all ephemeral):
  - `/home/agent/.npm`
  - `/home/agent/.cache/pip`
  - `/home/agent/go`
  - `/home/agent/.terraform.d/plugin-cache`

### Docker Runtime Building

#### `build_container_runtime(name, type, repos, ssh, network, memory, cpus, pids, repo_display, network_label)` (line 772)
- **Initializes** `docker_cmd=( docker run -it )`
- **Sets**:
  - Container name, hostname
  - Env vars: AGENT_TYPE, REPOS, GIT_*
  - Calls `append_runtime_hardening()`, `append_ssh_mount()`, `append_cache_mounts()`
- **Result**: `docker_cmd[]` array ready for execution

#### `append_runtime_hardening(network, memory, cpus, pids_limit, repo_display, agent_label, ssh_label, network_mode_label)` (line 575)
- **Security hardening flags**:
  - `--cap-drop=ALL` — drop all capabilities
  - `--security-opt=no-new-privileges:true`
  - `--security-opt "seccomp=/etc/safe-agentic/seccomp.json"`
  - `--read-only` — immutable rootfs
  - `--ulimit nofile=65536:65536`
  
- **tmpfs mounts** (writable, noexec):
  - `/tmp:rw,noexec,nosuid,size=512m`
  - `/var/tmp:rw,noexec,nosuid,size=256m`
  - `/run:rw,noexec,nosuid,size=16m`
  - `/dev/shm:rw,noexec,nosuid,size=64m`
  - `/home/agent/.config:rw,noexec,nosuid,uid=1000,gid=1000,size=32m` (git config)
  - `/home/agent/.ssh:rw,noexec,nosuid,uid=1000,gid=1000,size=1m` (known_hosts)

- **Resource limits**:
  - `--cpus <cpus>`
  - `--memory <memory>`
  - `--pids-limit <pids_limit>`

- **Network**:
  - `--network <network_name>`

- **Labels**: All prefixed `safe-agentic.*` (see cmd_spawn labels section)

#### `append_ssh_mount(enable_ssh, repos_joined)` (line 614)
- **If SSH enabled**:
  - Reads `$SSH_AUTH_SOCK` from VM
  - Creates relay via `socat`:
    - `/tmp/safe-agentic-ssh-relay.sh` script
    - `start-stop-daemon` to daemonize
    - Mounts relay socket into container at `/run/ssh-agent.sock`
  - Sets `SSH_AUTH_SOCK=/run/ssh-agent.sock` in container
  - **Fallback**: Direct mount if relay fails
  
- **If SSH disabled but repos are SSH**:
  - Dies with error (forces --ssh flag)

#### `append_host_docker_socket_access()` (line 60, docker-runtime.sh)
- **If --docker-socket**:
  - Reads VM docker socket path
  - Gets GID of socket group
  - Mounts: `-v /var/run/docker.sock:/run/docker-host.sock`
  - Sets: `DOCKER_HOST=unix:///run/docker-host.sock`
  - Adds: `--group-add <gid>` for access

#### `start_internal_docker_runtime(container_name, network_name)` (line 109, docker-runtime.sh)
- **If --docker (but not --docker-socket)**:
  - Creates volumes:
    - `<container>-docker-sock` (DinD socket)
    - `<container>-docker-data` (daemon data)
  - Spawns privileged daemon container:
    ```bash
    docker run -d \
      --name safe-agentic-docker-<container> \
      --privileged \
      --network <network_name> \
      --mount type=volume,src=<sock-vol>,dst=/run/safe-agentic-docker \
      --mount type=volume,src=<data-vol>,dst=/var/lib/docker \
      -e SAFE_AGENTIC_INTERNAL_DOCKERD=1 \
      -e SAFE_AGENTIC_DOCKER_SOCKET=/run/safe-agentic-docker/docker.sock \
      safe-agentic:latest
    ```
  - Waits for daemon ready (40 retries, 0.5s each = 20s timeout)

#### `inject_host_config(agent_type)` (line 662)
- **If Codex**:
  - Reads `$CODEX_HOME/config.toml`
  - Adapts paths: `/Users/... → /workspace`
  - Base64 encodes → `SAFE_AGENTIC_CODEX_CONFIG_B64` env var

- **If Claude**:
  - Reads `$CLAUDE_CONFIG_DIR/settings.json`
  - Base64 encodes → `SAFE_AGENTIC_CLAUDE_CONFIG_B64` env var
  - Also reads support files:
    - `CLAUDE.md`, `statusline-command.sh`, `hooks/`, `commands/`
    - Tar+gzip+base64 → `SAFE_AGENTIC_CLAUDE_SUPPORT_B64` env var

#### `inject_aws_credentials(profile)` (line 717)
- **Reads**: `$AWS_SHARED_CREDENTIALS_FILE` (default `~/.aws/credentials`)
- **Validates**: Profile exists in file
- **Extracts**: Profile section
- **Base64 encodes**: → `SAFE_AGENTIC_AWS_CREDS_B64` env var

### Container Execution

#### `run_container(managed_network, network_name)` (line 749)
- **Executes**: `orb run -m safe-agentic "${docker_cmd[@]}"`
- **Note**: Container persists after exit (no --rm)
- **Returns**: Exit code

#### `run_container_detached()` (line 760)
- **Converts** `-it` → `-d` (detached mode)
- **Executes**: `orb run -m safe-agentic ...`

### Audit & Event Logging

#### `audit_log(action, container, details)` (line 817)
- **File**: `$SAFE_AGENTIC_AUDIT_LOG` or `~/.config/safe-agentic/audit.jsonl`
- **Format**: One JSON object per line
  ```json
  {"timestamp":"2026-04-10T12:34:56Z","action":"spawn","container":"agent-claude-1234","details":"memory=8g,cpus=4"}
  ```
- **Append-only**: Never modifies existing lines

#### `emit_event(file, event_type, json_data)` (line 862)
- **Writes** to arbitrary JSONL file:
  ```json
  {"ts":"2026-04-10T12:34:56Z","event":"<type>","data":<json>}
  ```

#### `dispatch_event(sink_type, sink_target, event_type, json_data)` (line 872)
- **Dispatches** event to sink:
  - `command`: eval `$sink_target` with event in env
  - `webhook`: POST to `$sink_target` URL
  - `file`: call `emit_event()`

### Budget Enforcement

#### `compute_running_cost(jsonl_file)` (line 888)
- **Reads**: Session JSONL file (claude or codex)
- **Parses**: Message objects with `usage` + `model` fields
- **Pricing table**:
  ```
  claude-opus-4-6:   $15.0 / 1M input, $75.0 / 1M output
  claude-sonnet-4-6:  $3.0 / 1M input, $15.0 / 1M output
  claude-haiku-4-5:   $0.80 / 1M input, $4.0 / 1M output
  gpt-5.4:           $2.50 / 1M input, $10.0 / 1M output
  o3:               $10.0 / 1M input, $40.0 / 1M output
  o4-mini:           $1.10 / 1M input, $4.40 / 1M output
  (default: $3.0 / $15.0)
  ```
- **Returns**: Cost as decimal string (e.g., "22.50")
- **Fallback**: "0.00" if file missing or error

#### `check_budget(jsonl_file, max_cost)` (line 921)
- **Calls**: `compute_running_cost()`
- **Compares**: current_cost ≤ max_cost
- **Returns**: 0 if under, 1 if over

#### `start_budget_monitor(container_name, max_cost, events_file)` (line 930)
- **Background loop** (backgrounded subprocess):
  - Polls every 10s
  - Watches container state
  - Collects JSONL files from container:
    - `/home/agent/.claude/*.jsonl`
    - `/home/agent/.codex/*.jsonl`
    - Excludes `subagents/` subdirs
  - Calls `check_budget()`
  - If over: emits event, stops container
  - Breaks if container gone

### tmux Session Management

#### `tmux_session_name()` (line 534)
- **Returns**: `$SAFE_AGENTIC_TMUX_SESSION_NAME` or `"safe-agentic"`

#### `container_has_tmux_session(name)` (line 543)
- **Docker**: `docker exec <name> tmux has-session -t <session_name>`
- **Returns**: 0 if session exists

#### `wait_for_tmux_session(name, attempts)` (line 551)
- **Loops** up to 300 attempts (default) or provided count
- **Calls**: `container_has_tmux_session()` each iteration
- **Sleep**: 0.1s between attempts
- **Returns**: 0 when session ready

#### `attach_tmux_session(name, session_name)` (line 567)
- **Docker**: `docker exec -it <name> tmux attach-session -t <session_name>`

#### `container_terminal_mode(name)` (line 538)
- **Reads label**: `safe-agentic.terminal`
- **Returns**: `"tmux"` or `"direct"` or empty

### Notification System

#### `send_notification(type, target, agent_name, status, [details])` (line 492)
- **Types**:
  - `terminal` — macOS `osascript` or Linux `notify-send`
  - `slack` — HTTP POST to webhook URL
  - `command:*` — eval custom command
  
#### `parse_notify_targets(input)` (line 518)
- **Input**: Comma-separated list (e.g., `"terminal,slack,command:logger"`)
- **Output**: Lines, one target per line

---

## 4. bin/agent-session.sh

### Session Event Logging
- **Dir**: `$SAFE_AGENTIC_SESSION_STATE_DIR` (default `/workspace/.safe-agentic`)
- **Files**:
  - `started` — marker file (touch on start/resume)
  - `session-events.jsonl` — session events (JSONL)
  - `pending-prompt` — text file with pending prompt (for Claude tmux send-keys)

### Event Format
```json
{"ts":"2026-04-10T12:34:56Z","type":"session.start","data":{"agent":"claude","repos":"..."}}
{"ts":"2026-04-10T12:34:56Z","type":"session.end","data":{"exit_code":0}}
```

### Session Resumption
- **Check**: `[ -f "$SESSION_STATE_FILE" ]` → `resuming=true`
- **Codex**:
  ```bash
  codex login --device-auth  # if first run
  codex --yolo resume --last  # if resuming
  ```
- **Claude**:
  ```bash
  claude --dangerously-skip-permissions --continue  # if resuming
  ```

### Auto-Trust Directory Support
- **File**: `trust_workspace()` function
- **Logic**: Python3 script to update `settings.json`
- **Adds** to `trustedDirectories`:
  - Current working directory
  - `/workspace`
  - All `/workspace/*` subdirectories (cloned repos)

### Agent Launch Modes

#### **Interactive (default)**
- Starts tmux session via `start_tmux_session`
- Claude/Codex run with full TTY
- Supports `attach` command

#### **Background Mode** (`SAFE_AGENTIC_BACKGROUND=1`)
- Runs agent via `script -qfc` (PTY emulation for auth refresh)
- Output goes to docker logs
- No tmux, not attachable
- Used for non-interactive/pipeline runs

#### **With Prompt** (`--prompt` or `-p`)
- Claude: Saves prompt to `pending-prompt` file
- Entrypoint sends it via `tmux send-keys` after session starts
- Keeps agent interactive for live output

---

## 5. entrypoint.sh — Container Init Sequence

### Execution Order
1. **Docker daemon check** (if `SAFE_AGENTIC_INTERNAL_DOCKERD=1`)
2. **Runtime config setup** (git, AWS, Claude/Codex config)
3. **Security preamble injection** (CLAUDE.md / AGENTS.md)
4. **Repository cloning** (from `REPOS` env var)
5. **Lifecycle script execution** (`setup` from safe-agentic.json)
6. **Agent instructions injection** (AGENT-INSTRUCTIONS.md)
7. **Agent launch** (claude/codex/shell via tmux or directly)

### Config File Restoration

#### **Git Config**
- **File**: `/home/agent/.config/git/config`
- **Sets**:
  ```bash
  user.name = $GIT_AUTHOR_NAME
  user.email = $GIT_AUTHOR_EMAIL
  core.pager = delta --dark
  init.defaultBranch = main
  ```

#### **Codex Config** (`$CODEX_HOME/config.toml`)
- **Source**: `$SAFE_AGENTIC_CODEX_CONFIG_B64` (base64 decoded)
- **Fallback**:
  ```toml
  approval_policy = "never"
  sandbox_mode = "danger-full-access"
  ```

#### **Claude Config** (`$CLAUDE_CONFIG_DIR/settings.json`)
- **Source**: `$SAFE_AGENTIC_CLAUDE_CONFIG_B64` (base64 decoded)
- **Fallback**:
  ```json
  {
    "permissions": {
      "defaultMode": "bypassPermissions"
    }
  }
  ```

#### **Claude Support Files**
- **Source**: `$SAFE_AGENTIC_CLAUDE_SUPPORT_B64` (tar+gzip+base64 decoded)
- **Files**: `CLAUDE.md`, `hooks/`, `commands/`, `statusline-command.sh`

#### **AWS Credentials**
- **File**: `/home/agent/.aws/credentials`
- **Source**: `$SAFE_AGENTIC_AWS_CREDS_B64` (base64 decoded)
- **Permissions**: `chmod 600`

### Security Preamble Injection
- **Template**: `/usr/local/lib/safe-agentic/security-preamble.md` (or `config/` in build)
- **Placeholders** substituted:
  - `{{SSH_STATUS}}` — enabled/disabled
  - `{{AWS_STATUS}}` — injected or not
  - `{{NETWORK_STATUS}}` — managed/none/custom
  - `{{DOCKER_STATUS}}` — off/dind/host-socket
  - `{{RESOURCES}}` — memory/cpus/pids summary
- **Destination**:
  - Claude: appended to `$CLAUDE_CONFIG_DIR/CLAUDE.md`
  - Codex: appended to `$CODEX_HOME/AGENTS.md`
  - Both if type=shell
- **Idempotence**: Checks for marker `safe-agentic:security-preamble`

### Repository Cloning
- **Source**: `$REPOS` env var (comma-joined URLs)
- **Logic**:
  - Parse each URL
  - Call `repo_clone_path()` (from repo-url.sh)
  - Clone to `/workspace/<org>/<repo>`
  - Skip if dir already exists (for resume)
- **Single repo behavior**: `cd` into cloned dir before agent launch

### Lifecycle Scripts
- **File**: `safe-agentic.json` in any cloned repo
- **Script field**: `scripts.setup` (shell command)
- **Execution**: `bash -c "<script>"` from repo directory
- **Error**: Non-fatal warning only

### Agent Instructions
- **Source**: `$SAFE_AGENTIC_INSTRUCTIONS_B64` (base64 decoded)
- **Destination**: `/workspace/AGENT-INSTRUCTIONS.md`
- **Readable by**: Agent during session

### tmux Session Startup
- **Session name**: `$SAFE_AGENTIC_TMUX_SESSION_NAME` (default: `safe-agentic`)
- **History limit**: `$SAFE_AGENTIC_TMUX_HISTORY_LIMIT` (default: 500,000 lines)
- **Command**: `tmux new-session -d -s <name> /usr/local/lib/safe-agentic/agent-session.sh [args]`
- **Wait for session**: Up to 50 attempts × 0.1s = 5s timeout

### Prompt Injection (Claude)
- **File**: `$SESSION_STATE_DIR/pending-prompt`
- **Mechanism**: Background process reads file, sends via `tmux send-keys`
- **Timing**:
  - If `--auto-trust`: Sleep 4s, press Enter, sleep 3s, type prompt
  - Else: Sleep 5s, type prompt
- **Cleanup**: Delete `pending-prompt` after sending

---

## 6. Docker Labels — Complete Reference

All labels prefixed `safe-agentic.*` for containers, networks, volumes:

### Container Labels
```
safe-agentic.type=container                    # Marks as managed container
safe-agentic.agent-type=<claude|codex|shell>   # Agent type
safe-agentic.repo-display=<label>              # Repo display name (or "-")
safe-agentic.ssh=<on|off>                      # SSH forwarding enabled
safe-agentic.auth=<shared|ephemeral>           # Auth volume mode
safe-agentic.gh-auth=<shared|ephemeral>        # GitHub CLI auth mode
safe-agentic.docker=<off|dind|host-socket>     # Docker access mode
safe-agentic.network-mode=<managed|custom|none> # Network type
safe-agentic.terminal=<tmux|direct>            # Terminal mode (claude/codex: tmux, others: direct)
safe-agentic.prompt=<base64>                   # Encoded initial prompt (for retry)
safe-agentic.instructions=<1>                  # Flag: instructions injected
safe-agentic.on-exit=<1>                       # Flag: on-exit callback registered
safe-agentic.on-complete-b64=<base64>          # On-complete callback (encoded)
safe-agentic.on-fail-b64=<base64>              # On-fail callback (encoded)
safe-agentic.max-cost=<DOLLARS>                # Budget limit (e.g., "10.50")
safe-agentic.notify-b64=<base64>               # Notification targets (encoded)
safe-agentic.mcp-oauth=<true>                  # Flag: MCP OAuth ports published
safe-agentic.fleet=<name>                      # Fleet name (if part of fleet)
```

### Network Labels
```
safe-agentic.type=container-network             # Marks as managed network
app=safe-agentic                                # Image name (for cleanup filtering)
```

### Volume Labels
```
app=safe-agentic
safe-agentic.type=docker-runtime-volume         # Docker daemon data/socket volumes
safe-agentic.parent=<container-name>            # Parent container name
```

---

## 7. TUI Go Code — Package Structure

### Main App (`app.go`)
```go
type App struct {
  tapp      *tview.Application
  pages     *tview.Pages
  header    *Header
  table     *AgentTable
  footer    *Footer
  preview   *PreviewPane
  poller    *Poller
  actions   *Actions
  loaded    chan struct{}       // closed after first poll
  execAfter []string           // scheduled exec-after-exit
}

// Methods
NewApp()                        // Create & wire TUI
Run()                          // Start poller + event loop
SuspendAndRun(fn)              // Suspend TUI, run fn, resume
ExecAfterExit(args)            // Schedule command for exec-after-exit
ExecAfterArgs() []string       // Get scheduled command
```

### Model (`model.go`)
```go
type Agent struct {
  Name        string  // Container name
  Type        string  // "claude", "codex", "shell"
  Repo        string  // Repo label (or "-")
  SSH         string  // "on" or "off"
  Auth        string  // "shared" or "ephemeral"
  GHAuth      string  // "shared" or "ephemeral"
  Docker      string  // "off", "dind", "host-socket"
  NetworkMode string  // "managed", "custom", "none"
  Status      string  // Docker status (e.g., "Up 2 hours")
  Running     bool    // Container state
  Activity    string  // "Working", "Idle", "Stopped"
  CPU         string  // "2.5%" (from docker stats)
  Memory      string  // "512MiB / 8GiB"
  NetIO       string  // "5.2MB / 1.2MB"
  PIDs        string  // "42"
}

// Column definitions for table
type Column struct {
  Title    string
  Width    int      // min width; 0 = flexible
  Priority int      // lower = dropped first when narrow
}
```

### Poller (`poller.go`) [CRITICAL for Go rewrite]
```go
type Poller struct {
  mu       sync.Mutex
  agents   []Agent
  stale    bool
  stopCh   chan struct{}
  stopped  chan struct{}
  stopOnce sync.Once
  onUpdate func([]Agent, bool)  // callback
}

// Key methods
Start()               // Begin polling loop
Stop()               // Stop polling (timeout 500ms)
GetAgents()          // Thread-safe agent list snapshot
ForceRefresh()       // Trigger immediate poll
Restart()            // Stop & restart polling

// Internal
loop()               // Poll loop (ticker-based, interval=2s)
poll()               // Fetch agents via orb/docker commands
```

### Polling Implementation (`poller.go`)
- **Interval**: `pollInterval = 2` seconds
- **Runs**: `fetchAgents()` every tick
- **Callbacks**: `onUpdate(agents []Agent, stale bool)`

#### Docker Commands Executed
```bash
orb run -m safe-agentic docker ps -a \
  --filter "name=^agent-" \
  --format "{{json .}}"

orb run -m safe-agentic docker stats --no-stream \
  --format "{{json .}}" <running-container-names>...
```

#### JSON Parsing
- **Struct**: `dockerPSEntry` — maps JSON from `docker ps`
- **Struct**: `dockerStatsEntry` — maps JSON from `docker stats`
- **Labels**: Parsed from comma-separated Docker label string

#### Activity Detection (`probeProcessActivity`)
- **Logic**: 
  - Run `pgrep -x <agent-binary>` (claude or codex)
  - Read `/proc/<pid>/stat` field 14+15 (utime+stime)
  - Measure delta over 1 second window
  - If delta > 0: "Working", else "Idle"
- **Timeout**: 5s per probe (via `execOrbTimeout`)

#### orb Integration
- **Wrapper**: `execOrb(args...)` → `execOrbTimeout(5s, ...)`
- **Command**: `orb run -m safe-agentic <args>`
- **Handles**: Timeout, stderr suppression, stdout capture

### Dashboard (`dashboard.go`)
- **HTTP server** on `localhost:8420` (default)
- **Endpoints**: JSON API for agent data + web UI
- **Uses**: Same Poller as TUI

### Actions (`actions.go`)
- **User input handling** (keybindings)
- **Calls**: shell commands for attach, stop, logs, etc.

---

## 8. Config Files & Formats

### defaults.sh
**Location**: `$XDG_CONFIG_HOME/safe-agentic/defaults.sh` or `~/.config/safe-agentic/defaults.sh`

**Format**: Simple shell variable assignments (no shell code execution)
```bash
# Comments supported
SAFE_AGENTIC_DEFAULT_CPUS=8
SAFE_AGENTIC_DEFAULT_MEMORY="16g"
SAFE_AGENTIC_DEFAULT_PIDS_LIMIT=1024
SAFE_AGENTIC_DEFAULT_NETWORK="custom-net"
SAFE_AGENTIC_DEFAULT_SSH=true
SAFE_AGENTIC_DEFAULT_DOCKER=false
SAFE_AGENTIC_DEFAULT_REUSE_AUTH=true
SAFE_AGENTIC_DEFAULT_REUSE_GH_AUTH=true
GIT_AUTHOR_NAME="Agent Bot"
GIT_AUTHOR_EMAIL="bot@example.com"
GIT_COMMITTER_NAME="Agent Bot"
GIT_COMMITTER_EMAIL="bot@example.com"
```

**Parsing**:
- Whitespace trimmed
- Comments (`# ...`) ignored
- `export KEY=value` syntax supported
- Quoted strings: `"..."` or `'...'` (escaping supported: `\"`, `\\`)
- Unquoted values: must be simple (no spaces)

### audit.jsonl
**Location**: `$SAFE_AGENTIC_AUDIT_LOG` or `~/.config/safe-agentic/audit.jsonl`

**Format**: Append-only JSONL
```json
{"timestamp":"2026-04-10T12:34:56Z","action":"spawn","container":"agent-claude-1234","details":"memory=8g,cpus=4"}
{"timestamp":"2026-04-10T12:35:00Z","action":"stop","container":"agent-claude-1234","details":""}
```

**Fields**:
- `timestamp` — ISO 8601 UTC
- `action` — "spawn", "stop", "attach", "cleanup", etc.
- `container` — container name (empty string if N/A)
- `details` — action-specific metadata (optional)

### session-events.jsonl
**Location**: `/workspace/.safe-agentic/session-events.jsonl` (inside container)

**Format**: Append-only JSONL
```json
{"ts":"2026-04-10T12:34:56Z","type":"session.start","data":{"agent":"claude","repos":"git@github.com:org/repo.git"}}
{"ts":"2026-04-10T12:34:57Z","type":"session.end","data":{"exit_code":0}}
```

**Event types**:
- `session.start` — agent session initiated
- `session.end` — agent exited

### Fleet YAML Schema
**File**: `examples/fleet-*.yaml`

**Format**:
```yaml
shared_tasks: <true|false>  # optional, default false

agents:
  - name: <name>            # required, alphanumeric
    type: <claude|codex>    # required
    repo: <url>             # optional
    ssh: <true|false>       # optional
    reuse_auth: <true|false>
    reuse_gh_auth: <true|false>
    docker: <true|false>
    docker_socket: <true|false>
    auto_trust: <true|false>
    background: <true|false>
    prompt: <text>
    aws: <profile>
    network: <name>
    memory: <size>
    cpus: <number>
    pids_limit: <number>
    identity: <"Name <email>">
    max_cost: <dollars>
    notify: <targets>
```

### Pipeline YAML Schema
**File**: `examples/pipeline-*.yaml`

**Format**:
```yaml
name: <name>

steps:
  - name: <name>             # required
    type: <claude|codex>     # required
    repo: <url>              # optional
    ssh: <true|false>
    reuse_auth: <true|false>
    reuse_gh_auth: <true|false>
    docker: <true|false>
    auto_trust: <true|false>
    prompt: <text>
    depends_on: <step-name>  # optional, waits for step
    retry: <count>           # optional, retry on failure
    timeout: <seconds>       # optional
    on_failure: <stop|continue>  # stop=abort pipeline, continue=skip to next
```

### safe-agentic.json (Repository Config)
**Location**: Root of cloned repo

**Format**:
```json
{
  "scripts": {
    "setup": "npm install && npm run build",
    "teardown": "npm run clean"
  },
  "resources": {
    "memory": "16g",
    "cpus": 8
  }
}
```

**Used by**:
- Entrypoint to run `setup` script during initialization
- Future: Fleet/pipeline to override defaults

---

## 9. Test Infrastructure

### Fake orb Binary (`tests/test-*.sh`)
Located in temp `$FAKE_BIN/orb`, implements minimal orb interface:

**Sub-commands**:
- `list` — prints "safe-agentic"
- `run` — 
  - Logs all args to `$TEST_ORB_LOG`
  - Handles specific command patterns:
    - `bash -c '<ssh-check>'` → prints "/tmp/fake-ssh.sock"
    - `bash -c '<docker-socket-check>'` → prints "/var/run/docker.sock"
    - `bash -c '<docker-gid-check>'` → prints "998"
    - `bash -lc '*safe-agentic-hardening-verify*'` → creates verify marker file
    - `docker image inspect ...` → exit 0 (image exists)
    - `docker network ...` → exit 1 (inspect), exit 0 (create/rm)
    - All other docker commands → exit 0
  - All other commands → exit 0
- `push`, `start`, `stop`, `create`, `ssh` — accepted but no-op

### Test Helpers
- `run_ok <label> <cmd>` — assert command succeeds
- `run_fails <label> <cmd>` — assert command fails
- `assert_output_contains <needle> <label>` — grep output/error logs
- `last_docker_run()` — extract last `docker run` from log

### Test Environment
- `PATH="$FAKE_BIN:$PATH"` — fake binaries shadow real ones
- `TEST_ORB_LOG="$ORB_LOG"` — all `vm_exec` calls logged
- `TEST_VERIFY_STATE="$VERIFY_STATE"` — marker file location
- Cleanup: `trap 'rm -rf "$TMP_DIR"' EXIT`

---

## 10. Key Environment Variables Summary

### User-Set (from defaults.sh or CLI)
```
SAFE_AGENTIC_DEFAULT_CPUS=<N>
SAFE_AGENTIC_DEFAULT_MEMORY=<SIZE>
SAFE_AGENTIC_DEFAULT_PIDS_LIMIT=<N>
SAFE_AGENTIC_DEFAULT_NETWORK=<NAME>
SAFE_AGENTIC_DEFAULT_SSH=<true|false>
SAFE_AGENTIC_DEFAULT_DOCKER=<true|false>
SAFE_AGENTIC_DEFAULT_REUSE_AUTH=<true|false>
SAFE_AGENTIC_DEFAULT_REUSE_GH_AUTH=<true|false>
GIT_AUTHOR_NAME=<NAME>
GIT_AUTHOR_EMAIL=<EMAIL>
GIT_COMMITTER_NAME=<NAME>
GIT_COMMITTER_EMAIL=<EMAIL>
```

### Host-To-Container Injection (set by bin/agent, passed to docker run -e)
```
AGENT_TYPE=<claude|codex|shell>
REPOS=<comma-joined URLs>
GIT_AUTHOR_NAME, GIT_AUTHOR_EMAIL, GIT_COMMITTER_*
SAFE_AGENTIC_INSTRUCTIONS_B64=<base64>
SAFE_AGENTIC_ON_EXIT_B64=<base64>
SAFE_AGENTIC_ON_COMPLETE_B64=<base64>
SAFE_AGENTIC_ON_FAIL_B64=<base64>
SAFE_AGENTIC_NOTIFY_B64=<base64>
SAFE_AGENTIC_AWS_CREDS_B64=<base64>
SAFE_AGENTIC_AUTO_TRUST=<1|empty>
SAFE_AGENTIC_BACKGROUND=<1|empty>
SAFE_AGENTIC_CODEX_CONFIG_B64=<base64>
SAFE_AGENTIC_CLAUDE_CONFIG_B64=<base64>
SAFE_AGENTIC_CLAUDE_SUPPORT_B64=<tar+gzip+base64>
SAFE_AGENTIC_TMUX_SESSION_NAME=safe-agentic
SAFE_AGENTIC_SSH_ENABLED=<1|empty>
SAFE_AGENTIC_INTERNAL_DOCKERD=<1|empty>
SAFE_AGENTIC_DOCKER_SOCKET=<path>
SAFE_AGENTIC_RESOURCES=<summary>
```

### Internal (lib.sh functions)
```
docker_cmd=()           # Array of docker run args (built incrementally)
ACTIVE_CONTAINER_NAME   # Container being spawned
network_name            # Resolved network name
```

---

## 11. Critical Docker Commands & Patterns

### Network Creation (managed)
```bash
docker network create \
  --driver bridge \
  --opt "com.docker.network.bridge.name=$bridge_name" \
  --opt com.docker.network.bridge.enable_icc=false \
  --label "app=safe-agentic" \
  --label "safe-agentic.type=container-network" \
  "$network_name"
```

### Network Cleanup (managed only)
```bash
# Only removes networks with label safe-agentic.type=container-network
docker network inspect --format '{{index .Labels "safe-agentic.type"}}' <name>
docker network rm <name>
```

### Container Inspection Patterns
```bash
# List all safe-agentic containers
docker ps -a --filter "name=^agent-" --format '{{json .}}'

# Get label value
docker inspect --format '{{index .Config.Labels "safe-agentic.agent-type"}}' <name>

# Get state
docker inspect --format '{{.State.Status}}' <name>
```

### Container Resource Stats
```bash
docker stats --no-stream --format '{{json .}}' <name1> [<name2> ...]
```

Returns JSON with: `Name`, `CPUPerc`, `MemUsage`, `NetIO`, `PIDs`

### Volume Lifecycle
```bash
# Create labeled volume
docker volume create \
  --label "app=safe-agentic" \
  --label "safe-agentic.type=docker-runtime-volume" \
  --label "safe-agentic.parent=<container>" \
  "$volume_name"

# List volumes by filter
docker volume ls -q --filter "label=app=safe-agentic" --filter "label=safe-agentic.type=docker-runtime-volume"

# Remove
docker volume rm <names>...
```

---

## 12. Execution Flow: End-to-End Example

### User Command
```bash
agent spawn claude --repo https://github.com/user/repo.git --prompt "Fix the bug in main.go"
```

### Flow
1. **CLI parsing** (`bin/agent`, `cmd_spawn`)
   - Parses args: type=claude, repos=[...], prompt=...
   - Auto-detects git identity
   - Validates inputs

2. **VM & Image checks** (`cmd_spawn`)
   - `require_vm` → `orb list` (VM exists?)
   - `ensure_vm_runtime_hardening` → checks seccomp
   - `ensure_local_image_present` → checks image exists

3. **Network setup** (`prepare_network`)
   - `create_managed_network` → `docker network create`

4. **Docker command building** (`build_container_runtime`)
   - Initialize `docker_cmd=( docker run -it )`
   - Add flags: name, hostname, pull=never
   - `append_runtime_hardening` → caps, seccomp, tmpfs, limits
   - `append_ssh_mount` → SSH relay socket
   - `append_cache_mounts` → pip/npm/go volumes
   - `inject_host_config` → Claude settings.json
   - Add labels, env vars

5. **Container execution** (`run_container`)
   - `orb run -m safe-agentic "${docker_cmd[@]}"`
   - Docker creates container
   - Executes entrypoint.sh

6. **Inside container** (`entrypoint.sh`)
   - Setup git config from env vars
   - Restore Claude config (from base64)
   - Inject security preamble
   - Clone repos from `REPOS` env var
   - Run `safe-agentic.json` setup script
   - Inject AGENT-INSTRUCTIONS.md
   - Start tmux session → run `agent-session.sh`

7. **Agent launch** (`agent-session.sh`)
   - Write `session.start` event
   - Call `trust_workspace` (add dirs to Claude trustedDirectories)
   - Launch: `script -qfc "claude --dangerously-skip-permissions" /dev/null`
   - Claude reads config, starts interactive session
   - Prompt sent via `tmux send-keys` (if provided)

8. **Live monitoring** (TUI via `Poller`)
   - Every 2 seconds:
     - `docker ps -a --filter "name=^agent-"` → get containers
     - `docker stats ...` → get CPU/memory
     - `docker exec <name> pgrep -x claude` → activity probe
     - Update table display

9. **Container exit**
   - User exits Claude (`exit` or Ctrl+D)
   - `agent-session.sh` writes `session.end` event
   - Container stops
   - `cmd_cleanup` (manual) or automatic removal

---

## 13. Go Rewrite Checklist

### Core CLI (replace `bin/agent`)
- [ ] Command dispatcher (`cmd_*` functions)
- [ ] Argument parsing for each command
- [ ] Help text generation
- [ ] Error handling & die() pattern

### Docker Runtime Builder (replace `agent-lib.sh`)
- [ ] Validation functions (name, pids, network)
- [ ] Container name resolution
- [ ] Network creation/removal (managed & custom)
- [ ] Volume management
- [ ] Runtime hardening flags
- [ ] SSH relay setup (socat wrapper)
- [ ] Config injection (Claude/Codex/AWS)
- [ ] Docker-in-Docker setup
- [ ] docker_cmd array building

### Execution (replace vm_exec, run_container)
- [ ] orb integration (`orb run -m safe-agentic ...`)
- [ ] Timeout handling
- [ ] Error propagation
- [ ] Audit logging (append-only JSONL)
- [ ] Event dispatch (webhooks, commands)

### Budget & Monitoring
- [ ] Cost computation from session JSONL
- [ ] Budget check & enforcement
- [ ] Background monitor loop (10s polling)

### TUI (replace tui/ Go code — already Go!)
- [ ] Keep existing: Already in Go
- [ ] Integrate with new CLI (syscall.Exec for attach/spawn)

### Config Management
- [ ] defaults.sh parser
- [ ] Validation for allowed keys
- [ ] Git identity detection & application

### YAML Parsing (Fleet/Pipeline)
- [ ] YAML unmarshaling (stdlib or manual parse)
- [ ] Dependency resolution (topological sort)
- [ ] Parallel execution (fleet) vs sequential (pipeline)
- [ ] Retry logic
- [ ] Timeout handling

### Testing
- [ ] Fake orb binary mirrored in Go (test helper)
- [ ] Table-driven tests for validation
- [ ] Integration tests with Docker/Orb
- [ ] Audit log format verification

---

## 14. Critical Design Patterns

### Array-Based Docker Command Building
```go
// Bash: docker_cmd=(docker run -it)
// docker_cmd+=(--name container)
// docker run "${docker_cmd[@]}"

// Go equivalent:
dockerCmd := []string{"docker", "run", "-it"}
dockerCmd = append(dockerCmd, "--name", "container")
cmd := exec.Command("orb", append([]string{"run", "-m", "safe-agentic"}, dockerCmd...)...)
```

### Base64-Encoded Injection Pattern
```go
// Bash:
// prompt_b64=$(printf '%s' "$prompt" | base64 | tr -d '\n')
// docker_cmd+=(--label "safe-agentic.prompt=$prompt_b64")

// Go:
// import "encoding/base64"
promptB64 := base64.StdEncoding.EncodeToString([]byte(prompt))
dockerCmd = append(dockerCmd, "--label", fmt.Sprintf("safe-agentic.prompt=%s", promptB64))
```

### Append-Only Logging Pattern
```go
// Every write appends, never modifies
// File handle kept open or reopened in append mode
file, err := os.OpenFile(auditLogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
json.NewEncoder(file).Encode(logEntry)
file.Close()
```

### Background Loop Pattern (Budget Monitor)
```go
// Spawned via go func() or background subprocess
// Polls every 10s, exits when container gone or budget exceeded
go func() {
  ticker := time.NewTicker(10 * time.Second)
  defer ticker.Stop()
  for {
    select {
    case <-ticker.C:
      if containerGone() { return }
      if budgetExceeded() { stopContainer(); return }
    }
  }
}()
```

---

This reference covers every major function, data structure, Docker command, and flow in safe-agentic. Use this as your source of truth for the Go rewrite.
