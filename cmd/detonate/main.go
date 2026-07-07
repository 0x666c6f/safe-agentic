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
  - No reuse. Each run clones a fresh VM from an immutable golden and is
    destroyed after collection, or automatically on failure. There is no
    restore, revert, or reuse verb: a clone that has touched a live sample
    is never detonated again.
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
