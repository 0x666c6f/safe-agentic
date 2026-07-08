// Package detonate holds the pure decision logic for the malware-analysis
// orchestrator: which tier a sample is routed to, and the forward-only
// lifecycle a detonation run moves through. No VM or container calls live
// here — see the containment guard in guard.go for the one side-effect-free
// safety check that gates network attachment.
package detonate

import "fmt"

// Tier is where a sample gets detonated.
type Tier int

const (
	TierLocalARM Tier = iota
	TierCloudX86
	TierCommercial
	TierRefuse
)

func (t Tier) String() string {
	switch t {
	case TierLocalARM:
		return "local-arm"
	case TierCloudX86:
		return "cloud-x86"
	case TierCommercial:
		return "commercial"
	case TierRefuse:
		return "refuse"
	default:
		return "unknown"
	}
}

// StaticFindings is the pre-detonation triage output used to route a sample.
type StaticFindings struct {
	SHA256   string
	FileType string
	Arch     string
	Format   string
}

// Route decides which tier a sample goes to and why.
//
// Decision table:
//
//	arch in {arm64, aarch64}                      -> TierLocalARM (native on Apple Silicon)
//	format in {script, macro, elf-arm, macho}      -> TierLocalARM (arch-independent, runs local)
//	format == pe && arch in {x86-64, amd64, i386}  -> TierCloudX86 (no faithful local x86 detonation)
//	anything else (empty/unrecognized arch+format) -> TierRefuse (unsafe to route blind)
//
// Arch is checked before format, so an ARM64 PE still routes local.
func Route(f StaticFindings) (Tier, string) {
	switch {
	case f.Arch == "arm64" || f.Arch == "aarch64":
		return TierLocalARM, fmt.Sprintf("ARM (%s) sample — detonates natively on Apple Silicon", f.Arch)
	case f.Format == "script" || f.Format == "macro" || f.Format == "elf-arm" || f.Format == "macho":
		return TierLocalARM, fmt.Sprintf("%s format runs natively on Apple Silicon", f.Format)
	case f.Format == "pe" && (f.Arch == "x86-64" || f.Arch == "amd64" || f.Arch == "i386"):
		return TierCloudX86, "x86 Windows PE — no faithful local detonation on Apple Silicon; use a self-hosted cloud x86 sandbox"
	case f.Arch == "" && f.Format == "":
		return TierRefuse, "no static findings: arch and format are both empty"
	case f.Format == "pe":
		return TierRefuse, fmt.Sprintf("PE format with unsupported/unrecognized architecture %q", f.Arch)
	default:
		return TierRefuse, fmt.Sprintf("unrecognized arch/format combination (arch=%q, format=%q)", f.Arch, f.Format)
	}
}

// State is a point in a detonation run's lifecycle. States only move
// forward, one step at a time; Destroy is the sole exception and is always
// reachable, from any state, to guarantee cleanup can never be blocked.
// Destroyed is terminal: no further transitions, including to itself.
type State int

const (
	StateCreated State = iota
	StateInjected
	StateDetonated
	StateCollected
	StateDestroyed
)

func (s State) String() string {
	switch s {
	case StateCreated:
		return "Created"
	case StateInjected:
		return "Injected"
	case StateDetonated:
		return "Detonated"
	case StateCollected:
		return "Collected"
	case StateDestroyed:
		return "Destroyed"
	default:
		return "unknown"
	}
}

// CanTransition reports whether moving from s to "to" is a legal step.
func (s State) CanTransition(to State) bool {
	if s == StateDestroyed {
		return false // terminal: no reuse, no re-destroy
	}
	if to == StateDestroyed {
		return true // destroy is always allowed, from any live state
	}
	return to == s+1 // forward, single-step only
}
