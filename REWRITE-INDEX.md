# Safe-Agentic Go Rewrite — Complete Documentation Index

## Quick Links

Three comprehensive documents have been created for your Go rewrite:

### 1. **REWRITE-REFERENCE.md** (51 KB)
**The complete source of truth for the Go rewrite.**

Contains detailed specifications for every function, flag, label, and environment variable used in the safe-agentic CLI.

**Structure:**
- §1: Architecture overview
- §2: All 42+ cmd_* functions (each with line number, purpose, Docker ops, env vars, files)
- §3: All 70+ exported functions from agent-lib.sh (validation, Docker runtime, SSH, budget, audit)
- §4-5: Container session & entrypoint logic
- §6: Complete Docker label namespace (safe-agentic.*)
- §7: TUI Go code structures
- §8: Config file formats and schemas
- §9-14: Tests, env vars, commands, examples, design patterns

**Use this for:**
- ✅ Understanding what each command does
- ✅ Finding exact Docker flags and order
- ✅ Label generation requirements
- ✅ Environment variable injection
- ✅ File I/O patterns
- ✅ Config file formats
- ✅ Test infrastructure details

**Most Important Sections:**
- §2 cmd_spawn (944 lines) — THE critical command
- §3 append_runtime_hardening — Docker security flags
- §3 build_container_runtime — Docker command orchestration
- §6 Docker labels — Label namespace for cleanup & filtering
- §11 Docker commands & patterns — Actual docker ps/inspect syntax

---

### 2. **GO-REWRITE-CHECKLIST.md** (7.6 KB)
**Actionable implementation guide.**

Breaks down the entire rewrite into 9 phases with checkboxes and tasks.

**Phases:**
1. Core CLI Framework (dispatcher, validation, config)
2. Docker Runtime Builder (name resolution, networks, volumes, hardening, SSH, injection)
3. Container Execution (orb integration, lifecycle, tmux sessions)
4. Audit, Events & Budget (logging, dispatch, cost tracking, notifications)
5. Complex Commands (cmd_spawn, cmd_fleet, cmd_pipeline, cmd_docker, ~30 others)
6. Docker-in-Docker (DinD setup, cleanup)
7. Testing (test helpers, unit tests, integration tests)
8. TUI Integration (keep existing, add integration points)
9. Container Entrypoint (no action — keep as bash)

**Recommended Reading Order:**
1. skim through for overview
2. Reference during implementation
3. Check off completed tasks

**Use this for:**
- ✅ Implementation roadmap
- ✅ Task breakdown
- ✅ Priority ordering
- ✅ Progress tracking
- ✅ Data structure definitions
- ✅ Design pattern notes

---

### 3. **EXPLORATION-SUMMARY.md** (10 KB)
**Quick reference summary with key discoveries.**

Distills the most important findings into digestible sections.

**Sections:**
- 10 Key Discoveries (Docker patterns, labels, SSH, base64, budget, fleet/pipeline, lifecycle, TUI poller, tests)
- Critical files for implementation
- Files to keep as-is (no rewrite needed)
- Bash-to-Go translation patterns (with code examples)
- Line count summary
- Success criteria
- Recommended Go dependencies

**Use this for:**
- ✅ Quick reference during coding
- ✅ Bash-to-Go translation examples
- ✅ Understanding 10 key design patterns
- ✅ Dependencies to add
- ✅ File management decisions

---

## Implementation Workflow

### Step 1: Planning
1. Read EXPLORATION-SUMMARY.md completely (10 min)
2. Skim REWRITE-REFERENCE.md §1-3 (20 min)
3. Review GO-REWRITE-CHECKLIST.md phases (10 min)

### Step 2: Implementation
1. Start Phase 1 (CLI + validation) in parallel with Phase 2 (Docker builder)
2. Frequently reference REWRITE-REFERENCE.md for exact specifications
3. Use EXPLORATION-SUMMARY.md for translation patterns
4. Check off tasks in GO-REWRITE-CHECKLIST.md

### Step 3: Critical Implementations
- **High priority first**:
  - CLI dispatcher (Phase 1)
  - cmd_spawn (Phase 5) — requires phases 1-3
  - Validation functions (Phase 1)
  - Docker command building (Phase 2)
  - Budget monitoring (Phase 4)
  
- **Medium priority**:
  - Other commands (Phase 5) — 30+ cmd_* functions
  - Fleet/Pipeline (Phase 5) — requires YAML parsing, topological sort

- **Lower priority**:
  - Docker-in-Docker (Phase 6) — advanced feature
  - Testing (Phase 7) — after core works
  - TUI integration (Phase 8) — near end

### Step 4: Testing
- Use existing test infrastructure (tests/test-*.sh)
- Fake orb binary provided
- Test with `agent spawn` command first
- Verify Docker labels match exactly
- Audit log format verification

---

## Key Takeaways

### 1. Docker Command Building is Central
Every CLI command ultimately builds a Docker command. The bash pattern:
```bash
docker_cmd=(docker run -it)
append_runtime_hardening ...
append_ssh_mount ...
run_container  # executes: orb run -m safe-agentic "${docker_cmd[@]}"
```

Maps to Go as `[]string` with careful append ordering.

### 2. Labels Are Critical
The `safe-agentic.*` label namespace is used for:
- **Container classification**: agent-type, ssh status, auth mode, docker mode
- **TUI filtering**: Poller reads labels to populate table
- **Cleanup safety**: Networks/volumes only deleted if labeled correctly

Must match exactly (see REWRITE-REFERENCE.md §6).

### 3. cmd_spawn is Complex
~400 lines of bash, 25+ flags, 40+ labels, 5+ orchestrated functions. Implement last but thoroughly.

### 4. Base64 Transport
8 different fields transported via base64:
- Docker labels: prompt, on-complete-b64, on-fail-b64, notify-b64
- Env vars: instructions, on-exit, aws creds, codex config, claude config, support files

No tr -d needed in Go (encoding/base64 handles it).

### 5. Budget Monitoring is Background
Separate process loops every 10s, collects files from container, parses JSONL, stops if over budget.

### 6. Three Files Can Stay Bash
- entrypoint.sh (runs in container)
- agent-session.sh (runs in container)
- TUI Go code (already Go, just integrate)

Only rewrite ~6,000 lines of bash (bin/agent + bin/agent-lib.sh + bin/docker-runtime.sh).

### 7. No Docker SDK Needed
Use standard `exec.Command` with `orb run -m safe-agentic ...`. CLI interface is sufficient and preferred for consistency with existing code.

---

## File Status

| File | Status | Action |
|------|--------|--------|
| REWRITE-REFERENCE.md | ✅ Complete | Read & reference |
| GO-REWRITE-CHECKLIST.md | ✅ Complete | Use for planning |
| EXPLORATION-SUMMARY.md | ✅ Complete | Quick reference |
| bin/agent | Keep | Rewrite to Go |
| bin/agent-lib.sh | Keep | Rewrite to Go |
| bin/docker-runtime.sh | Keep | Rewrite to Go |
| entrypoint.sh | ✅ Keep as-is | No action |
| bin/agent-session.sh | ✅ Keep as-is | No action |
| tui/*.go | ✅ Keep as-is | Only integration points |

---

## Questions to Ask While Reading

### For REWRITE-REFERENCE.md:
- What Docker commands does this function run?
- What labels does it set?
- What env vars does it read/set?
- What files does it read/write on host vs container?

### For GO-REWRITE-CHECKLIST.md:
- Which phase am I in?
- What's the next task to implement?
- Do I have all dependencies clear?

### For EXPLORATION-SUMMARY.md:
- How do I translate this bash pattern to Go?
- What's a critical design pattern I should know?
- What dependencies do I need?

---

## Success Checklist

After implementing Go rewrite:

- [ ] All 42+ cmd_* functions work identically
- [ ] Every Docker label matches bash version exactly
- [ ] Docker command flags in correct order (see REWRITE-REFERENCE.md §2, §11)
- [ ] No functional regressions vs bash
- [ ] Unit tests pass for validation, cost, YAML parsing
- [ ] Integration tests pass with fake orb
- [ ] TUI still works (via syscall.Exec)
- [ ] Code is well-tested & documented
- [ ] Performance acceptable

---

## Need More Details?

- **On specific command**: See REWRITE-REFERENCE.md §2
- **On specific library function**: See REWRITE-REFERENCE.md §3
- **On Docker patterns**: See REWRITE-REFERENCE.md §11 or EXPLORATION-SUMMARY.md
- **On test infrastructure**: See REWRITE-REFERENCE.md §9
- **On implementation order**: See GO-REWRITE-CHECKLIST.md
- **On quick translation**: See EXPLORATION-SUMMARY.md "Bash-to-Go Translation Patterns"
- **On labels**: See REWRITE-REFERENCE.md §6
- **On config formats**: See REWRITE-REFERENCE.md §8

---

**Last updated**: 2026-04-10
**Exploration completed by**: Claude Code
**Total documentation**: 1,502 + 200 + 10 KB = 1,712 KB (comprehensive spec)

Start with EXPLORATION-SUMMARY.md, then dive deep into REWRITE-REFERENCE.md as needed. Good luck! 🚀
