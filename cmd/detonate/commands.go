package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/0x666c6f/berth/pkg/audit"
	"github.com/0x666c6f/berth/pkg/config"
	"github.com/0x666c6f/berth/pkg/detonate"
	"github.com/0x666c6f/berth/pkg/validate"

	"github.com/spf13/cobra"
)

// newRunner builds the Runner used by every command. Tests override this var
// with a func returning a *detonate.FakeRunner — same injection pattern as
// cmd/berth's newExecutor.
var newRunner = func() detonate.Runner {
	workDir := os.Getenv("DETONATE_WORKDIR")
	if workDir == "" {
		workDir = filepath.Join(config.StateDir(), "detonate")
	}
	r := detonate.NewTartRunner(workDir)
	r.AllowedIsolatedGateway = os.Getenv("DETONATE_ISOLATED_GATEWAY")
	return r
}

// auditLog appends one chain-of-custody entry. The write error never fails
// the operation — audit is best-effort — but for a chain-of-custody tool a
// silently-lost entry is worth a warning, so it's surfaced on stderr.
func auditLog(run, action string, details map[string]string) {
	if err := (&audit.Logger{Path: audit.DefaultPath()}).Log(action, run, details); err != nil {
		fmt.Fprintf(os.Stderr, "detonate: warning: audit write failed: %v\n", err)
	}
}

// loadRunState reads a run's persisted state, translating "no such file"
// into a clear create-first error so every gated verb reports the same
// message for a run that was never created.
func loadRunState(run string) (*detonate.Run, error) {
	st, err := detonate.LoadRun(run)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("run %q does not exist — create it first", run)
		}
		return nil, fmt.Errorf("loading state for run %q: %w", run, err)
	}
	return st, nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// parseStaticFindings reads berth forensic JSON into StaticFindings,
// tolerating either snake_case or camelCase field names.
func parseStaticFindings(data []byte) (detonate.StaticFindings, error) {
	// map[string]any (not map[string]string): berth's forensic JSON carries
	// numeric fields too (e.g. "size", "entropy"). Unmarshaling into
	// map[string]string would fail wholesale the moment any field isn't a
	// string. Only the string-typed fields below are actually used; anything
	// else, numeric or not, is ignored.
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return detonate.StaticFindings{}, fmt.Errorf("parsing static findings JSON: %w", err)
	}
	pick := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := raw[k].(string); ok {
				return v
			}
		}
		return ""
	}
	return detonate.StaticFindings{
		SHA256:   pick("sha256", "SHA256"),
		FileType: pick("file_type", "fileType", "FileType"),
		Arch:     pick("arch", "Arch"),
		Format:   pick("format", "Format"),
	}, nil
}

// ─── route ──────────────────────────────────────────────────────────────

var routeCmd = &cobra.Command{
	Use:   "route <static.json>",
	Short: "Decide which detonation tier a sample belongs to, from its static findings",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRoute(args[0], os.Stdout)
	},
}

func init() { rootCmd.AddCommand(routeCmd) }

func runRoute(staticPath string, out io.Writer) error {
	data, err := os.ReadFile(staticPath)
	if err != nil {
		return fmt.Errorf("reading static findings: %w", err)
	}
	findings, err := parseStaticFindings(data)
	if err != nil {
		return err
	}
	tier, reason := detonate.Route(findings)
	fmt.Fprintf(out, "tier: %s\nreason: %s\n", tier, reason)
	switch tier {
	case detonate.TierRefuse:
		return fmt.Errorf("refused: %s", reason)
	case detonate.TierCloudX86, detonate.TierCommercial:
		fmt.Fprintf(out, "guidance: %s tier is not orchestrated locally by detonate — provision that sandbox out of band.\n", tier)
	}
	return nil
}

// ─── create ─────────────────────────────────────────────────────────────

var createGolden string

var createCmd = &cobra.Command{
	Use:   "create <run> --golden <name>",
	Short: "Clone a fresh detonation run from an immutable golden image",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCreate(context.Background(), newRunner(), args[0], createGolden)
	},
}

func init() {
	createCmd.Flags().StringVar(&createGolden, "golden", "", "name of the immutable golden VM image to clone from (required)")
	rootCmd.AddCommand(createCmd)
}

func runCreate(ctx context.Context, r detonate.Runner, run, golden string) error {
	if err := validate.NameComponent(run, "run name"); err != nil {
		return err
	}
	// Hold the run lock across the whole existence-check-then-create section:
	// without it, two concurrent `create` calls could both pass the
	// existence check before either persists state=Created.
	unlock, err := detonate.LockRun(run)
	if err != nil {
		return err
	}
	defer unlock()
	// Fail closed on reuse: a state file already existing means some earlier
	// run by this name is still live (or at least undestroyed). destroy is
	// the only way to clear it.
	if _, err := detonate.LoadRun(run); err == nil {
		return fmt.Errorf("run %q already exists; destroy it first", run)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking existing state for run %q: %w", run, err)
	}
	if golden == "" {
		return fmt.Errorf("--golden is required")
	}
	if err := validate.NameComponent(golden, "--golden"); err != nil {
		return err
	}
	exists, err := r.GoldenExists(ctx, golden)
	if err != nil {
		return fmt.Errorf("checking golden %q: %w", golden, err)
	}
	if !exists {
		return fmt.Errorf("golden image %q not found — provision it before create", golden)
	}
	if err := r.Clone(ctx, golden, run); err != nil {
		return fmt.Errorf("clone: %w", err)
	}
	now := time.Now()
	if err := detonate.SaveRun(&detonate.Run{Name: run, Golden: golden, State: detonate.StateCreated, Nonce: detonate.NewNonce(), CreatedAt: now}); err != nil {
		return fmt.Errorf("persisting state for run %q: %w", run, err)
	}
	auditLog(run, "detonate-create", map[string]string{"golden": golden})
	fmt.Printf("created run %q from golden %q\n", run, golden)
	return nil
}

// ─── inject ─────────────────────────────────────────────────────────────

var injectCmd = &cobra.Command{
	Use:   "inject <run> <sample>",
	Short: "Hash and attach a sample to a run's disk, offline, before boot",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInject(context.Background(), newRunner(), args[0], args[1])
	},
}

func init() { rootCmd.AddCommand(injectCmd) }

func runInject(ctx context.Context, r detonate.Runner, run, samplePath string) error {
	if err := validate.NameComponent(run, "run name"); err != nil {
		return err
	}
	unlock, err := detonate.LockRun(run)
	if err != nil {
		return err
	}
	defer unlock()
	st, err := loadRunState(run)
	if err != nil {
		return err
	}
	nonce := st.Nonce
	if !st.State.CanTransition(detonate.StateInjected) {
		return fmt.Errorf("run %q is not freshly created (state=%s) — inject requires state=Created", run, st.State)
	}
	hash, err := sha256File(samplePath)
	if err != nil {
		return fmt.Errorf("hashing sample: %w", err)
	}
	// Chain of custody: record the hash BEFORE any runner/VM work, so the
	// audit trail captures the attempt even if InjectOffline fails closed.
	auditLog(run, "detonate-inject", map[string]string{"sha256": hash, "sample": samplePath})
	if err := r.InjectOffline(ctx, run, samplePath); err != nil {
		return fmt.Errorf("inject offline: %w", err)
	}
	st.State = detonate.StateInjected
	if err := detonate.SaveRunIfNonce(st, nonce); err != nil {
		return fmt.Errorf("persisting state for run %q: %w", run, err)
	}
	fmt.Printf("injected %s (sha256 %s) into run %q\n", samplePath, hash, run)
	return nil
}

// ─── run ────────────────────────────────────────────────────────────────

var (
	runTimeout time.Duration
	runYes     bool
	runGateway string
)

var runCmd = &cobra.Command{
	Use:   "run <run> --timeout 180s [--yes]",
	Short: "Boot a run's cloned VM against the injected sample, inside an isolated network only",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRun(context.Background(), newRunner(), args[0], runGateway, runTimeout, runYes, os.Stdin)
	},
}

func init() {
	runCmd.Flags().DurationVar(&runTimeout, "timeout", 180*time.Second, "hard wall-clock limit for the detonation window")
	runCmd.Flags().BoolVar(&runYes, "yes", false, "skip the typed confirmation prompt (also requires DETONATE_I_UNDERSTAND=1)")
	runCmd.Flags().StringVar(&runGateway, "gateway", "", "name of the pre-provisioned, operator-verified isolated (no-uplink) network gateway (required)")
	rootCmd.AddCommand(runCmd)
}

func runRun(ctx context.Context, r detonate.Runner, run, gateway string, timeout time.Duration, yes bool, stdin io.Reader) error {
	if err := validate.NameComponent(run, "run name"); err != nil {
		return err
	}
	if gateway == "" {
		return fmt.Errorf("--gateway is required: name the pre-provisioned, operator-verified isolated (no-uplink) network gateway")
	}
	if err := validate.NameComponent(gateway, "--gateway"); err != nil {
		return err
	}

	// Held for the entire gate-check-then-boot section (including the
	// confirmation prompt and the boot itself): this is what stops two
	// concurrent `run` invocations from both loading state=Injected and
	// both reaching Runner.Run before either persists Detonated.
	unlock, err := detonate.LockRun(run)
	if err != nil {
		return err
	}
	defer unlock()

	// This is the no-reuse enforcement: a run can only be booted once, from
	// state=Injected. Already-detonated (or collected) state means a prior
	// clone was already exposed to the live sample — never re-run it, even
	// if it was never destroyed.
	st, err := loadRunState(run)
	if err != nil {
		return err
	}
	nonce := st.Nonce
	if st.State == detonate.StateCreated {
		return fmt.Errorf("run %q has not been injected yet (state=Created) — inject a sample before run", run)
	}
	// CanTransition(Injected->Detonated) is the no-reuse policy: only a run
	// still at Injected may boot. Any later state (already Detonated/Collected,
	// or Destroyed) means the clone was exposed to the live sample once — never
	// re-run it.
	if !st.State.CanTransition(detonate.StateDetonated) {
		return fmt.Errorf("run %q already detonated (state=%s): destroy and re-create for a fresh run — clones are never reused", run, st.State)
	}

	net, err := r.ConfigureIsolatedNet(ctx, run, gateway)
	if err != nil {
		return fmt.Errorf("configure isolated net: %w", err)
	}
	// Fail closed: never call Run on an attachment that isn't isolated.
	if err := detonate.ValidateIsolated(net); err != nil {
		return err
	}

	if err := confirmDetonation(run, yes, stdin); err != nil {
		return err
	}

	// Mark the clone consumed BEFORE booting, not after. If this were saved
	// only on success, a boot that errors AND then fails to auto-destroy
	// would leave state at Injected — which still permits `run` — even
	// though the clone has already been exposed to the live sample once.
	// Saving Detonated up front means every later outcome (success, or
	// failure with auto-destroy failing too) leaves the run correctly
	// blocked from reuse; only a confirmed destroy clears it.
	st.State = detonate.StateDetonated
	st.Gateway = gateway
	// SaveRunIfNonce, not SaveRun: confirmDetonation above can block on operator
	// input for a long time, during which a lockless `destroy` + fresh `create`
	// could recreate this run name. Refuse to stamp Detonated (and then boot)
	// over a brand-new run's state.
	if err := detonate.SaveRunIfNonce(st, nonce); err != nil {
		return fmt.Errorf("persisting state for run %q: %w — aborting before boot", run, err)
	}

	details := map[string]string{"gateway": gateway, "timeout": timeout.String()}
	runErr := r.Run(ctx, run, timeout)
	if runErr != nil {
		details["error"] = runErr.Error()
		// r.Run can block for the whole detonation window. If a lockless
		// `destroy` + fresh `create` recreated this run name while we were
		// booting, our cleanup must NOT run: destroying now would kill the new
		// run's clone and deleting its state would wipe it. Re-read the nonce
		// and bail out of cleanup entirely on a mismatch (or if the run is gone).
		if cur, cerr := detonate.LoadRun(run); cerr != nil || cur.Nonce != nonce {
			details["skipped_cleanup"] = "recreated concurrently (nonce mismatch)"
			auditLog(run, "detonate-run", details)
			return fmt.Errorf("detonation failed AND run %q was recreated concurrently (nonce mismatch) — not touching the new run; original error: %w", run, runErr)
		}
		// Auto-destroy on any failure — best-effort, never masks runErr.
		// Destroy failing here means a live-malware VM may still be up, so
		// that outcome must never be reported as "(auto-destroyed)".
		if dErr := r.Destroy(ctx, run); dErr != nil {
			details["destroy_error"] = dErr.Error()
			auditLog(run, "detonate-run", details)
			// State stays Detonated (already saved above): the clone may
			// still be running, so this run name stays blocked from reuse
			// until an explicit destroy (always allowed) clears it.
			return fmt.Errorf("detonation failed AND auto-destroy FAILED — VM %q may still be running; destroy it manually with 'detonate destroy %s': runErr=%w destroyErr=%w", run, run, runErr, dErr)
		}
		details["auto_destroyed"] = "true"
		auditLog(run, "detonate-run", details)
		// The clone is gone (auto-destroy succeeded), so clear its state too —
		// best-effort, same as the destroy verb — freeing the run name for a
		// fresh create. IfNonce re-checks the guard so a recreate that slipped
		// in between the check above and here still can't wipe the new run.
		_ = detonate.DeleteRunIfNonce(run, nonce)
		return fmt.Errorf("detonation failed (auto-destroyed): %w", runErr)
	}
	details["status"] = "completed"
	auditLog(run, "detonate-run", details)
	fmt.Printf("run %q completed within %s — collect artifacts next\n", run, timeout)
	return nil
}

// confirmDetonation gates the one command that actually boots a live sample.
// --yes alone is not enough: automation must also opt in via
// DETONATE_I_UNDERSTAND=1, so a bare --yes in a script can't silently
// detonate. Interactively, the operator must type the run name back exactly.
func confirmDetonation(run string, yes bool, stdin io.Reader) error {
	if yes && os.Getenv("DETONATE_I_UNDERSTAND") == "1" {
		return nil
	}
	phrase := "detonate " + run
	fmt.Printf("About to boot run %q with a live sample in an isolated sandbox.\n", run)
	fmt.Println("Benign behavior inside the sandbox does NOT mean the sample is safe.")
	fmt.Printf("Type: %s\n> ", phrase)

	reader := bufio.NewReader(stdin)
	answer, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("reading confirmation: %w", err)
	}
	if strings.TrimSpace(answer) != phrase {
		return fmt.Errorf("confirmation phrase did not match — aborting")
	}
	return nil
}

// ─── collect ────────────────────────────────────────────────────────────

var collectOut string

var collectCmd = &cobra.Command{
	Use:   "collect <run> --out <dir>",
	Short: "Copy artifacts out of a powered-off run and hash them",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCollect(context.Background(), newRunner(), args[0], collectOut)
	},
}

func init() {
	collectCmd.Flags().StringVar(&collectOut, "out", "", "host directory to copy collected artifacts into (required)")
	rootCmd.AddCommand(collectCmd)
}

func runCollect(ctx context.Context, r detonate.Runner, run, outDir string) error {
	if err := validate.NameComponent(run, "run name"); err != nil {
		return err
	}
	if outDir == "" {
		return fmt.Errorf("--out is required")
	}
	unlock, err := detonate.LockRun(run)
	if err != nil {
		return err
	}
	defer unlock()
	st, err := loadRunState(run)
	if err != nil {
		return err
	}
	nonce := st.Nonce
	if !st.State.CanTransition(detonate.StateCollected) {
		return fmt.Errorf("run %q is not in Detonated state (state=%s) — collect requires a completed run", run, st.State)
	}
	off, err := r.PoweredOff(ctx, run)
	if err != nil {
		return fmt.Errorf("checking power state: %w", err)
	}
	if !off {
		return fmt.Errorf("collect refused: run %q is not powered off", run)
	}
	// files may be non-empty even when collectErr != nil: TartRunner.Collect
	// copies artifacts one-by-one and returns (alreadyCopied, err) on a
	// mid-loop failure. Only skip straight to the error when nothing was
	// actually collected — otherwise those already-copied artifacts would
	// sit in outDir with zero audit trail.
	files, collectErr := r.Collect(ctx, run, outDir)
	if collectErr != nil && len(files) == 0 {
		return fmt.Errorf("collect: %w", collectErr)
	}
	// Hash every artifact even if one fails: Runner.Collect already copied
	// them all to outDir, so an early return here would leave collected
	// artifacts on disk with zero audit trail. Record what we could hash
	// and surface the hash error after the audit entry is written.
	lines := make([]string, 0, len(files))
	var hashErr error
	for _, f := range files {
		hash, err := sha256File(f)
		if err != nil {
			hashErr = fmt.Errorf("hashing artifact %s: %w", f, err)
			lines = append(lines, "<error: "+err.Error()+">  "+f)
			continue
		}
		lines = append(lines, hash+"  "+f)
		fmt.Printf("%s  %s\n", hash, f)
	}
	auditLog(run, "detonate-collect", map[string]string{
		"count":     strconv.Itoa(len(files)),
		"artifacts": strings.Join(lines, "\n"),
	})
	if collectErr == nil {
		// Advance even if some artifacts failed to hash (hashErr): the copy
		// off the VM fully succeeded, so the run itself is Collected. A
		// mid-loop collectErr, by contrast, leaves state at Detonated so
		// collect can be retried.
		st.State = detonate.StateCollected
		if err := detonate.SaveRunIfNonce(st, nonce); err != nil {
			return fmt.Errorf("persisting state for run %q: %w", run, err)
		}
	}
	if collectErr != nil {
		return fmt.Errorf("collect: %w", collectErr)
	}
	return hashErr
}

// ─── destroy ────────────────────────────────────────────────────────────

var destroyCmd = &cobra.Command{
	Use:   "destroy <run>",
	Short: "Delete a run's VM clone (best-effort, idempotent)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDestroy(context.Background(), newRunner(), args[0])
	},
}

func init() { rootCmd.AddCommand(destroyCmd) }

func runDestroy(ctx context.Context, r detonate.Runner, run string) error {
	if err := validate.NameComponent(run, "run name"); err != nil {
		return err
	}
	// Capture the nonce of the run this destroy invocation is acting on, so the
	// state delete below only clears THIS run — not a fresh run that raced in
	// under the same name. A missing state file (never created, or already
	// destroyed) leaves nonce empty, matching legacy/ghost runs.
	var nonce string
	if st, lerr := detonate.LoadRun(run); lerr == nil {
		nonce = st.Nonce
	}
	err := r.Destroy(ctx, run)
	details := map[string]string{}
	if err != nil {
		details["error"] = err.Error()
	}
	auditLog(run, "detonate-destroy", details)
	// Always allowed, always clears state: destroy is the sole way to free a
	// run name for reuse, regardless of whether the runner destroy above
	// succeeded (best-effort, matching Destroy's always-reachable contract
	// in pkg/detonate.State.CanTransition). IfNonce guards the one case where
	// clearing would be wrong: the run was recreated between our load and here.
	if delErr := detonate.DeleteRunIfNonce(run, nonce); delErr != nil {
		fmt.Fprintf(os.Stderr, "detonate: warning: clearing state for run %q failed: %v\n", run, delErr)
	}
	if err != nil {
		return fmt.Errorf("destroy: %w", err)
	}
	fmt.Printf("destroyed run %q\n", run)
	return nil
}
