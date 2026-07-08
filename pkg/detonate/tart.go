package detonate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var _ Runner = (*TartRunner)(nil)

// CmdRunner is a thin exec.Command wrapper so TartRunner's command building
// is swappable in tests without ever shelling out. execCmdRunner is the
// real implementation; tests use a spy. Exported so external-package tests
// (notably the cmd/detonate end-to-end test) can drive a real TartRunner
// against a fake tart/hdiutil via SetCmdRunner.
type CmdRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type execCmdRunner struct{}

func (execCmdRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return out.Bytes(), fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, out.String())
	}
	return out.Bytes(), nil
}

// TartRunner is the real Runner backed by the `tart` CLI (cirruslabs/tart,
// Apple Virtualization.framework) plus `hdiutil` for sample staging.
//
// Network isolation is CODE-ENFORCED here: ConfigureIsolatedNet runs the
// softnet allow-list through ValidateSoftnetAllow (must be a single private
// CIDR — the operator's fakenet gateway subnet — never a public/0.0.0.0/0
// range), Run re-validates it plus the resulting NetAttachment through
// ValidateIsolated, and buildTartRunArgs emits --net-softnet, never
// --net-bridged. The operator still provisions the golden image and the
// fakenet gateway; this code guarantees the guest is pinned to it.
type TartRunner struct {
	cmd CmdRunner

	// WorkDir roots this runner's per-run host-side staging: sample
	// images and the artifacts directory shared into the guest.
	WorkDir string

	// AllowedIsolatedGateway is an OPTIONAL operator pin: when non-empty,
	// ConfigureIsolatedNet additionally requires the softnet allow-CIDR to
	// equal it, so the operator can nail detonations to one exact fakenet
	// gateway subnet. When empty, any CIDR that passes ValidateSoftnetAllow
	// (i.e. any private range) is accepted — the private-CIDR check is the
	// load-bearing containment control either way.
	AllowedIsolatedGateway string

	mu   sync.Mutex
	nets map[string]NetAttachment
	// gateways captures, per run, the exact softnet allow-CIDR
	// ConfigureIsolatedNet validated. Run must build its command from this
	// captured value — never from the mutable AllowedIsolatedGateway field —
	// so the value the guard checked and the value the command uses can never
	// diverge, even if AllowedIsolatedGateway changes afterward.
	gateways map[string]string
	// samples captures, per run, the path to the read-only sample image
	// InjectOffline built. Run refuses to boot a run with no injected sample,
	// and attaches this exact image via --disk=<path>:ro.
	samples map[string]string
}

func NewTartRunner(workDir string) *TartRunner {
	return &TartRunner{
		cmd:      execCmdRunner{},
		WorkDir:  workDir,
		nets:     make(map[string]NetAttachment),
		gateways: make(map[string]string),
		samples:  make(map[string]string),
	}
}

// SetCmdRunner swaps the exec seam. Test-only: production installs the real
// execCmdRunner via NewTartRunner. Exists so external-package tests can drive
// a real TartRunner against a fake tart/hdiutil without shelling out.
func (r *TartRunner) SetCmdRunner(c CmdRunner) { r.cmd = c }

// --- pure arg builders (unit-tested without exec) ---

func buildTartListArgs() []string {
	return []string{"list", "--format", "json"}
}

func buildTartCloneArgs(golden, run string) []string {
	return []string{"clone", golden, run}
}

func buildTartDeleteArgs(run string) []string {
	return []string{"delete", run}
}

func buildTartStopArgs(run string) []string {
	return []string{"stop", run}
}

// buildTartRunArgs builds the `tart run` argv for an isolated detonation.
// Documented cirruslabs/tart flags only — a wrong flag fails CLOSED because
// the guest won't boot:
//   - --no-graphics: headless.
//   - --net-softnet + --net-softnet-allow=<CIDR>: Tart's isolated user-space
//     network (softnet), pinned by an allow-list to the single private CIDR
//     of the operator's fakenet gateway. NEVER --net-bridged: a bridge can
//     reach the host LAN/internet, which is the exact egress this forbids.
//   - --disk=<sample>:ro: the injected sample, attached READ-ONLY. The :ro is
//     mandatory — the sample image must never be writable by the guest.
//   - --dir=out:<artifactsDir>: host artifacts share so the guest drops
//     results with no extraction step.
func buildTartRunArgs(run, sampleImagePath, softnetAllowCIDR, artifactsDir string) []string {
	return []string{
		"run", run,
		"--no-graphics",
		"--net-softnet",
		"--net-softnet-allow=" + softnetAllowCIDR,
		"--disk=" + sampleImagePath + ":ro",
		// ponytail: this writable artifact share is a known escape-surface
		// trade-off — a guest can plant symlinks/large files here (Collect is
		// already symlink-safe). The stronger fix is collecting artifacts from
		// an offline disk after poweroff, but that can't be implemented or
		// verified without a real Tart guest, so it's the deferred hardening.
		"--dir=out:" + artifactsDir,
	}
}

// buildHdiutilCreateArgs builds a read-only ISO 9660 image containing only the
// staged sample. An ISO (not a UDRO .dmg) is required: Tart's `--disk` attaches
// raw images and ISOs, but rejects an Apple UDIF/UDRO .dmg ("failed to lock …:
// Bad file descriptor"). ISO is inherently read-only, so the sample can never
// be written back to. srcDir is the trailing positional for makehybrid.
func buildHdiutilCreateArgs(srcDir, volName, outPath string) []string {
	return []string{"makehybrid", "-iso", "-joliet", "-default-volume-name", volName, "-ov", "-o", outPath, srcDir}
}

// tartListEntry is the subset of `tart list --format json` fields this
// package needs.
type tartListEntry struct {
	Name  string `json:"Name"`
	State string `json:"State"`
}

func parseTartList(output []byte) ([]tartListEntry, error) {
	var entries []tartListEntry
	if err := json.Unmarshal(output, &entries); err != nil {
		return nil, fmt.Errorf("parsing tart list output: %w", err)
	}
	return entries, nil
}

// sampleImagePath is the deterministic on-disk location of a run's injected
// sample image. InjectOffline writes it here and Run reads it back — the two
// run in SEPARATE CLI processes, so the file path (not the in-memory samples
// map) is the cross-invocation source of truth for what to attach.
func (r *TartRunner) sampleImagePath(run string) string {
	return filepath.Join(r.WorkDir, run, "sample.iso")
}

// --- Runner impl ---

func (r *TartRunner) GoldenExists(ctx context.Context, golden string) (bool, error) {
	out, err := r.cmd.Run(ctx, "tart", buildTartListArgs()...)
	if err != nil {
		return false, err
	}
	entries, err := parseTartList(out)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if e.Name == golden {
			return true, nil
		}
	}
	return false, nil
}

func (r *TartRunner) Clone(ctx context.Context, golden, run string) error {
	_, err := r.cmd.Run(ctx, "tart", buildTartCloneArgs(golden, run)...)
	return err
}

// ConfigureIsolatedNet interprets gw as the softnet allow-CIDR (the
// operator's fakenet gateway subnet/host). It fails closed unless gw is a
// single private CIDR (ValidateSoftnetAllow); if AllowedIsolatedGateway is
// set it must additionally match it exactly. On success it records the
// isolated NetAttachment and the validated CIDR per-run so Run consumes the
// exact value guarded here.
func (r *TartRunner) ConfigureIsolatedNet(_ context.Context, run, gw string) (NetAttachment, error) {
	if r.AllowedIsolatedGateway != "" && gw != r.AllowedIsolatedGateway {
		return NetAttachment{}, fmt.Errorf("containment violation: softnet allow-CIDR %q for run %q does not match the operator pin %q", gw, run, r.AllowedIsolatedGateway)
	}
	// Load-bearing control: the allow-list must be a private CIDR or the
	// sample gets internet egress. Reject anything public/0.0.0.0/0.
	if err := ValidateSoftnetAllow(gw); err != nil {
		return NetAttachment{}, err
	}
	n := NetAttachment{Mode: "isolated", HasUplink: false}
	r.mu.Lock()
	r.nets[run] = n
	r.gateways[run] = gw
	r.mu.Unlock()
	return n, nil
}

func (r *TartRunner) Run(ctx context.Context, run string, timeout time.Duration) error {
	r.mu.Lock()
	n, ok := r.nets[run]
	gw := r.gateways[run]
	sampleImage := r.samples[run]
	r.mu.Unlock()
	if !ok {
		return fmt.Errorf("containment violation: no network attachment configured for run %q; call ConfigureIsolatedNet first", run)
	}
	// CRITICAL: re-validate here, not just at ConfigureIsolatedNet time —
	// this is the last gate before an egress-capable command runs, and it
	// must hold even if ConfigureIsolatedNet is refactored later.
	if err := ValidateIsolated(n); err != nil {
		return err
	}
	// Fail closed if no sample was injected. The samples map is per-process;
	// inject and run are separate CLI invocations, so fall back to the
	// deterministic on-disk image path and require it to actually exist —
	// booting a clone with no sample disk is a no-op at best and a
	// state-confusion bug at worst.
	if sampleImage == "" {
		sampleImage = r.sampleImagePath(run)
	}
	if _, err := os.Stat(sampleImage); err != nil {
		return fmt.Errorf("containment violation: no sample image for run %q (%s); call inject first", run, err)
	}
	// Defense in depth: the captured CIDR must still be a private allow-list.
	// The value re-validated here is the exact value buildTartRunArgs uses.
	if err := ValidateSoftnetAllow(gw); err != nil {
		return err
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	artifactsDir := filepath.Join(r.WorkDir, run, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0o700); err != nil {
		return fmt.Errorf("preparing artifacts dir: %w", err)
	}
	// Use the CIDR and sample captured at inject/configure time — never the
	// mutable r.AllowedIsolatedGateway field — so the values just guarded are
	// the exact values that reach the command.
	_, err := r.cmd.Run(runCtx, "tart", buildTartRunArgs(run, sampleImage, gw, artifactsDir)...)

	// The timeout is the detonation WINDOW, not an error: a generic golden
	// never powers itself off, so the expected end of a run is our deadline
	// firing and tart run being killed. Treat DeadlineExceeded as the normal
	// end of the window — but only after CONFIRMING the guest is actually
	// powered off. A live-malware VM must never be reported as a completed
	// run: killing tart run should tear the guest down, but if the stop can't
	// be confirmed we fail closed so the caller auto-destroys the clone.
	//
	// runCtx.Err() distinguishes our timeout (DeadlineExceeded → window end)
	// from a parent-ctx cancel (Canceled → propagate as failure), and from a
	// genuine early tart failure (deadline not reached → err propagates).
	if runCtx.Err() == context.DeadlineExceeded {
		// Use the parent ctx, not the expired runCtx, for the teardown checks.
		_, _ = r.cmd.Run(ctx, "tart", buildTartStopArgs(run)...)
		off, offErr := r.PoweredOff(ctx, run)
		if offErr != nil {
			return fmt.Errorf("detonation window for run %q elapsed but VM power state could not be confirmed (treating as containment failure): %w", run, offErr)
		}
		if !off {
			return fmt.Errorf("detonation window for run %q elapsed but the VM is still running after stop — treating as containment failure", run)
		}
		return nil
	}
	return err
}

// InjectOffline builds a read-only disk image of the sample with hdiutil
// (a real, nameable command), refuses to proceed unless the guest is
// confirmed powered off, and records the built image path so Run can attach
// it read-only via `tart run --disk=<path>:ro`. Attaching an extra disk at
// `tart run` time (not to a live guest) is Tart's documented mechanism, so no
// separate live-attach step is needed.
func (r *TartRunner) InjectOffline(ctx context.Context, run, samplePath string) error {
	off, err := r.PoweredOff(ctx, run)
	if err != nil {
		return err
	}
	if !off {
		return fmt.Errorf("inject refused: run %q is not powered off", run)
	}

	runDir := filepath.Join(r.WorkDir, run)
	stageDir := filepath.Join(runDir, "sample-src")
	if err := os.MkdirAll(stageDir, 0o700); err != nil {
		return fmt.Errorf("preparing sample staging dir: %w", err)
	}
	data, err := os.ReadFile(samplePath)
	if err != nil {
		return fmt.Errorf("reading sample: %w", err)
	}
	staged := filepath.Join(stageDir, filepath.Base(samplePath))
	if err := os.WriteFile(staged, data, 0o400); err != nil {
		return fmt.Errorf("staging sample: %w", err)
	}

	outISO := r.sampleImagePath(run)
	if _, err := r.cmd.Run(ctx, "hdiutil", buildHdiutilCreateArgs(stageDir, run+"-sample", outISO)...); err != nil {
		return fmt.Errorf("building read-only sample image: %w", err)
	}

	r.mu.Lock()
	r.samples[run] = outISO
	r.mu.Unlock()
	return nil
}

func (r *TartRunner) Collect(ctx context.Context, run, destDir string) ([]string, error) {
	off, err := r.PoweredOff(ctx, run)
	if err != nil {
		return nil, err
	}
	if !off {
		return nil, fmt.Errorf("collect refused: run %q is not powered off", run)
	}

	srcDir := filepath.Join(r.WorkDir, run, "artifacts")
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return nil, fmt.Errorf("reading artifacts dir: %w", err)
	}
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return nil, fmt.Errorf("preparing dest dir: %w", err)
	}

	var collected []string
	for _, e := range entries {
		// srcDir is the guest-writable --dir=out: mount: a sample can plant
		// a symlink here (e.g. named report.json) pointing at any host path.
		// e.Type() reflects the raw directory entry, not a followed stat, so
		// it flags a symlink without ever resolving it. Skip anything that
		// isn't a plain regular file — symlinks, dirs, devices, sockets,
		// pipes — so the os.ReadFile below can never become a guest-to-host
		// file-read primitive. e.Name() is always a bare filename (no path
		// separators), so the Join below can't escape srcDir either.
		if !e.Type().IsRegular() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(srcDir, e.Name()))
		if err != nil {
			return collected, fmt.Errorf("reading artifact %s: %w", e.Name(), err)
		}
		dst := filepath.Join(destDir, e.Name())
		if err := os.WriteFile(dst, data, 0o600); err != nil {
			return collected, fmt.Errorf("writing artifact %s: %w", e.Name(), err)
		}
		collected = append(collected, dst)
	}
	return collected, nil
}

func (r *TartRunner) PoweredOff(ctx context.Context, run string) (bool, error) {
	out, err := r.cmd.Run(ctx, "tart", buildTartListArgs()...)
	if err != nil {
		return false, err
	}
	entries, err := parseTartList(out)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if e.Name == run {
			return strings.EqualFold(e.State, "stopped"), nil
		}
	}
	return false, fmt.Errorf("run %q not found in tart list", run)
}

func (r *TartRunner) Destroy(ctx context.Context, run string) error {
	_, err := r.cmd.Run(ctx, "tart", buildTartDeleteArgs(run)...)
	r.mu.Lock()
	delete(r.nets, run)
	delete(r.gateways, run)
	delete(r.samples, run)
	r.mu.Unlock()
	return err
}
