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

// auditLog appends one chain-of-custody entry. The error is intentionally
// unchecked, matching every other audit call site in cmd/berth.
func auditLog(run, action string, details map[string]string) {
	(&audit.Logger{Path: audit.DefaultPath()}).Log(action, run, details)
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
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return detonate.StaticFindings{}, fmt.Errorf("parsing static findings JSON: %w", err)
	}
	pick := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := raw[k]; ok {
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
	if golden == "" {
		return fmt.Errorf("--golden is required")
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

	details := map[string]string{"gateway": gateway, "timeout": timeout.String()}
	runErr := r.Run(ctx, run, timeout)
	if runErr != nil {
		details["error"] = runErr.Error()
		// Auto-destroy on any failure — best-effort, never masks runErr.
		// Destroy failing here means a live-malware VM may still be up, so
		// that outcome must never be reported as "(auto-destroyed)".
		if dErr := r.Destroy(ctx, run); dErr != nil {
			details["destroy_error"] = dErr.Error()
			auditLog(run, "detonate-run", details)
			return fmt.Errorf("detonation failed AND auto-destroy FAILED — VM %q may still be running; destroy it manually with 'detonate destroy %s': runErr=%w destroyErr=%w", run, run, runErr, dErr)
		}
		details["auto_destroyed"] = "true"
		auditLog(run, "detonate-run", details)
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
	off, err := r.PoweredOff(ctx, run)
	if err != nil {
		return fmt.Errorf("checking power state: %w", err)
	}
	if !off {
		return fmt.Errorf("collect refused: run %q is not powered off", run)
	}
	files, err := r.Collect(ctx, run, outDir)
	if err != nil {
		return fmt.Errorf("collect: %w", err)
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
	err := r.Destroy(ctx, run)
	details := map[string]string{}
	if err != nil {
		details["error"] = err.Error()
	}
	auditLog(run, "detonate-destroy", details)
	if err != nil {
		return fmt.Errorf("destroy: %w", err)
	}
	fmt.Printf("destroyed run %q\n", run)
	return nil
}
