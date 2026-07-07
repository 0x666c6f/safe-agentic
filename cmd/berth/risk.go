package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/0x666c6f/berth/pkg/config"
	"github.com/0x666c6f/berth/pkg/policy"
	"github.com/0x666c6f/berth/pkg/risk"
	"github.com/0x666c6f/berth/pkg/vmexec"
	"github.com/0x666c6f/berth/pkg/worktrees"
)

func spawnRiskInput(opts SpawnOpts, resolved spawnResolved) risk.SpawnInput {
	networkName := resolved.NetworkName
	if networkName == "" {
		networkName = opts.Network
	}
	return risk.SpawnInput{
		SSH:               opts.SSH,
		ReuseAuth:         opts.ReuseAuth,
		ReuseGHAuth:       opts.ReuseGHAuth,
		SeedAuth:          opts.SeedAuth,
		AWSProfile:        opts.AWSProfile,
		Docker:            opts.DockerAccess,
		DockerSocket:      opts.DockerSocket,
		AllowSetupScripts: opts.AllowSetupScripts,
		AutoTrust:         opts.AutoTrust,
		NetworkMode:       resolved.NetworkMode,
		NetworkName:       networkName,
	}
}

func printSpawnRiskSummary(w io.Writer, opts SpawnOpts, resolved spawnResolved) {
	notices := risk.SpawnNotices(spawnRiskInput(opts, resolved))
	if len(notices) == 0 && !opts.DryRun {
		return
	}
	fmt.Fprintln(w, "Security context:")
	if len(notices) == 0 {
		fmt.Fprintln(w, "  default: ephemeral auth, managed network, no SSH/AWS/Docker")
		return
	}
	for _, notice := range notices {
		fmt.Fprintf(w, "  ! %s: %s\n", notice.Flag, notice.Summary)
	}
	fmt.Fprintln(w, "  Guard: use ~/.berth/rules.toml or .berth/rules.toml to deny modes you never want.")
}

func printDiagnoseSpawnDefaults(w io.Writer, cfg config.Config, source string) {
	opts := SpawnOpts{
		SSH:          cfg.Defaults.SSH,
		ReuseAuth:    cfg.Defaults.ReuseAuth,
		ReuseGHAuth:  cfg.Defaults.ReuseGHAuth,
		SeedAuth:     cfg.Defaults.SeedAuth,
		DockerAccess: cfg.Defaults.Docker,
		DockerSocket: cfg.Defaults.DockerSocket,
	}
	resolved := spawnResolved{
		NetworkMode: policy.NetworkManaged,
		NetworkName: policy.NetworkManaged,
	}
	if cfg.Defaults.Network == policy.NetworkNone {
		resolved.NetworkMode = policy.NetworkNone
		resolved.NetworkName = policy.NetworkNone
	} else if cfg.Defaults.Network != "" && cfg.Defaults.Network != policy.NetworkManaged {
		resolved.NetworkMode = "custom"
		resolved.NetworkName = cfg.Defaults.Network
	}
	notices := risk.SpawnNotices(spawnRiskInput(opts, resolved))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Spawn defaults")
	if source != "" {
		fmt.Fprintf(w, "  config: %s\n", source)
	}
	if len(notices) == 0 {
		fmt.Fprintln(w, "  OK: default spawns use ephemeral auth, managed network, no SSH/AWS/Docker")
		return
	}
	for _, notice := range notices {
		fmt.Fprintf(w, "  ! %s enabled by default: %s\n", notice.Flag, notice.Summary)
	}
	fmt.Fprintln(w, "  Tip: override once with --no-* flags, or enforce hard policy in ~/.berth/rules.toml.")
}

func worktreeCandidatePath(containerName, requestedPath string) (string, error) {
	path := requestedPath
	if path == "" {
		path = worktrees.DefaultPath(containerName)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve worktree path: %w", err)
	}
	return abs, nil
}

func validateWorktreeMountPath(vmPath string) error {
	if strings.ContainsAny(vmPath, ",\n\r\x00") {
		return fmt.Errorf("worktree mount path %q contains characters Docker --mount cannot safely encode", vmPath)
	}
	return nil
}

// ensureWorktreeMountReadyInVM verifies that the worktrees root is actually
// bind-mounted at /worktrees inside the VM AND that the bind points at the
// current worktrees root. A machine created before this feature landed (or one
// whose home-mount was never enabled) has no /worktrees mount, so the worktree
// bind would fail or, worse, write into a throwaway VM path. And if
// defaults.worktrees_dir changed after setup, the VM may still bind the OLD
// root — spawn would create the worktree under the new host root while Docker
// binds the stale one, giving a launch failure or the wrong checkout. Fail fast
// with the exact remediation instead.
func ensureWorktreeMountReadyInVM(ctx context.Context, exec vmexec.Executor, vmPath, wantRoot string, dryRun bool) error {
	if dryRun {
		return nil
	}
	if _, err := exec.Run(ctx, "sh", "-c", "test -d "+worktrees.VMMountPoint+" && mountpoint -q "+worktrees.VMMountPoint); err != nil {
		return fmt.Errorf("VM %s has no %s mount, so --worktree cannot bind %s. The worktree mount is off by default; enable it with: berth setup --enable-worktrees (switches the VM to home-mount=rw, which weakens VM isolation — see docs). See also: berth diagnose", configuredVMName(), worktrees.VMMountPoint, vmPath)
	}
	// Confirm the live bind points at the current worktrees root (recorded by
	// vm/setup.sh in the boot-local sentinel). An empty sentinel means "unknown"
	// (older bind or a failed write) — don't block on that alone.
	srcOut, _ := exec.Run(ctx, "sh", "-c", "cat "+worktreesSentinelPath+" 2>/dev/null")
	if cur := strings.TrimSpace(string(srcOut)); cur != "" && cur != wantRoot {
		return fmt.Errorf("VM %s binds worktrees root %s but this checkout resolves to %s; the worktrees root changed after setup. Rebind with: berth setup (or: berth vm stop && berth vm start), then retry", configuredVMName(), cur, wantRoot)
	}
	return nil
}
