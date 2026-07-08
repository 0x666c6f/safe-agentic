// Command detonate is the enforced state machine for berth's defensive
// malware-detonation harness (Tier A: local ARM, disposable VM). Every
// safety boundary lives in code here, not in operator discipline — see the
// root Long text (detonate --help) for the safety model.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "detonate",
	Short: "Enforced state machine for defensive malware detonation (Tier A: local ARM)",
	Long: `detonate orchestrates the disposable-VM malware-detonation lifecycle for
berth's forensic workflow. Safety model (non-negotiable, enforced in code):

  - Isolated network only. Every run re-validates an isolated, no-uplink
    network attachment immediately before boot. Anything else fails closed
    and Run is never invoked.
  - No restore/reuse verb. Each run clones a fresh VM from an immutable
    golden and is meant to be destroyed after collection, or automatically
    on failure. There is no restore or revert verb, and reuse is enforced
    in code: a run's lifecycle state (Created/Injected/Detonated/Collected)
    is persisted per-run to ~/.berth/detonate/<run>.json, and every verb is
    gated on it. run refuses outright on a run that is already
    Detonated/Collected ("destroy and re-create for a fresh run — clones
    are never reused"). destroy is the sole escape hatch: always allowed,
    from any state, and it clears the state file so the run name can be
    created fresh again.
  - Offline inject. Samples are attached to the guest disk offline, before
    boot — never copied in over a live network path.
  - Benign-in-sandbox is not safe. A sample that behaves benignly inside the
    sandbox has not been proven safe outside it. Treat every processed
    sample, golden clone, and artifact as hostile indefinitely; this tool
    only bounds the blast radius.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "detonate:", err)
		os.Exit(1)
	}
}
