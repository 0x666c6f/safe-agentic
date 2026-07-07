package detonate

import (
	"strings"
	"testing"
)

func TestTierString(t *testing.T) {
	cases := map[Tier]string{
		TierLocalARM:   "local-arm",
		TierCloudX86:   "cloud-x86",
		TierCommercial: "commercial",
		TierRefuse:     "refuse",
	}
	seen := map[string]bool{}
	for tier, want := range cases {
		got := tier.String()
		if got != want {
			t.Errorf("Tier(%d).String() = %q, want %q", tier, got, want)
		}
		if seen[got] {
			t.Errorf("Tier.String() value %q reused across tiers", got)
		}
		seen[got] = true
	}
}

func TestRoute(t *testing.T) {
	tests := []struct {
		name      string
		findings  StaticFindings
		wantTier  Tier
		reasonHas string // substring the reason must contain
	}{
		{
			name:      "arm64 arch routes local regardless of format",
			findings:  StaticFindings{Arch: "arm64", Format: "elf"},
			wantTier:  TierLocalARM,
			reasonHas: "ARM",
		},
		{
			name:      "aarch64 arch routes local",
			findings:  StaticFindings{Arch: "aarch64", Format: "elf"},
			wantTier:  TierLocalARM,
			reasonHas: "ARM",
		},
		{
			name:      "script format routes local regardless of arch",
			findings:  StaticFindings{Arch: "", Format: "script"},
			wantTier:  TierLocalARM,
			reasonHas: "script",
		},
		{
			name:      "macro format routes local",
			findings:  StaticFindings{Arch: "unknown", Format: "macro"},
			wantTier:  TierLocalARM,
			reasonHas: "macro",
		},
		{
			name:      "elf-arm format routes local",
			findings:  StaticFindings{Format: "elf-arm"},
			wantTier:  TierLocalARM,
			reasonHas: "elf-arm",
		},
		{
			name:      "macho format routes local",
			findings:  StaticFindings{Format: "macho"},
			wantTier:  TierLocalARM,
			reasonHas: "macho",
		},
		{
			name:      "arm64 arch wins over pe format",
			findings:  StaticFindings{Arch: "arm64", Format: "pe"},
			wantTier:  TierLocalARM,
			reasonHas: "ARM",
		},
		{
			name:      "x86-64 PE routes cloud",
			findings:  StaticFindings{Arch: "x86-64", Format: "pe"},
			wantTier:  TierCloudX86,
			reasonHas: "no faithful local detonation on Apple Silicon",
		},
		{
			name:      "amd64 PE routes cloud",
			findings:  StaticFindings{Arch: "amd64", Format: "pe"},
			wantTier:  TierCloudX86,
			reasonHas: "self-hosted cloud x86 sandbox",
		},
		{
			name:      "i386 PE routes cloud",
			findings:  StaticFindings{Arch: "i386", Format: "pe"},
			wantTier:  TierCloudX86,
			reasonHas: "cloud x86",
		},
		{
			name:      "completely empty findings refused",
			findings:  StaticFindings{},
			wantTier:  TierRefuse,
			reasonHas: "empty",
		},
		{
			name:      "PE with unsupported arch refused",
			findings:  StaticFindings{Arch: "sparc", Format: "pe"},
			wantTier:  TierRefuse,
			reasonHas: "sparc",
		},
		{
			name:      "unrecognized arch/format combo refused",
			findings:  StaticFindings{Arch: "mips", Format: "elf"},
			wantTier:  TierRefuse,
			reasonHas: "mips",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotTier, gotReason := Route(tc.findings)
			if gotTier != tc.wantTier {
				t.Errorf("Route(%+v) tier = %v, want %v (reason: %q)", tc.findings, gotTier, tc.wantTier, gotReason)
			}
			if !strings.Contains(gotReason, tc.reasonHas) {
				t.Errorf("Route(%+v) reason = %q, want substring %q", tc.findings, gotReason, tc.reasonHas)
			}
		})
	}
}

// TestCanTransition exhaustively checks every (from, to) pair across all five
// states: forward single-step is allowed, backward/skip/same-state is
// rejected, Destroy is allowed from any non-terminal state, and Destroyed
// itself is a terminal dead end (no transitions out, including to itself).
func TestCanTransition(t *testing.T) {
	states := []State{StateCreated, StateInjected, StateDetonated, StateCollected, StateDestroyed}

	want := map[[2]State]bool{
		{StateCreated, StateCreated}:   false,
		{StateCreated, StateInjected}:  true,
		{StateCreated, StateDetonated}: false, // skip
		{StateCreated, StateCollected}: false, // skip
		{StateCreated, StateDestroyed}: true,  // destroy always allowed

		{StateInjected, StateCreated}:   false, // backward
		{StateInjected, StateInjected}:  false,
		{StateInjected, StateDetonated}: true,
		{StateInjected, StateCollected}: false, // skip
		{StateInjected, StateDestroyed}: true,

		{StateDetonated, StateCreated}:   false, // backward
		{StateDetonated, StateInjected}:  false, // backward
		{StateDetonated, StateDetonated}: false,
		{StateDetonated, StateCollected}: true,
		{StateDetonated, StateDestroyed}: true,

		{StateCollected, StateCreated}:   false, // backward
		{StateCollected, StateInjected}:  false, // backward
		{StateCollected, StateDetonated}: false, // backward
		{StateCollected, StateCollected}: false,
		{StateCollected, StateDestroyed}: true,

		{StateDestroyed, StateCreated}:   false, // terminal
		{StateDestroyed, StateInjected}:  false, // terminal
		{StateDestroyed, StateDetonated}: false, // terminal
		{StateDestroyed, StateCollected}: false, // terminal
		{StateDestroyed, StateDestroyed}: false, // terminal, no reuse
	}

	for _, from := range states {
		for _, to := range states {
			key := [2]State{from, to}
			expected, ok := want[key]
			if !ok {
				t.Fatalf("missing expectation for %v -> %v", from, to)
			}
			if got := from.CanTransition(to); got != expected {
				t.Errorf("State(%d).CanTransition(%d) = %v, want %v", from, to, got, expected)
			}
		}
	}
}

func TestValidateIsolated_Allowed(t *testing.T) {
	allowedModes := []string{"isolated", "host-none", "fakenet"}
	for _, mode := range allowedModes {
		n := NetAttachment{Mode: mode, HasUplink: false}
		if err := ValidateIsolated(n); err != nil {
			t.Errorf("ValidateIsolated(%+v) unexpected error: %v", n, err)
		}
	}
}

func TestValidateIsolated_RejectsUnsafeModes(t *testing.T) {
	unsafeModes := []string{"nat", "bridge", "shared", "host"}
	for _, mode := range unsafeModes {
		n := NetAttachment{Mode: mode, HasUplink: false}
		err := ValidateIsolated(n)
		if err == nil {
			t.Fatalf("ValidateIsolated(%+v) = nil error, want rejection", n)
		}
		if !strings.Contains(err.Error(), mode) {
			t.Errorf("ValidateIsolated(%+v) error = %q, want it to name the mode %q", n, err.Error(), mode)
		}
	}
}

func TestValidateIsolated_RejectsUplinkEvenWhenModeIsolated(t *testing.T) {
	n := NetAttachment{Mode: "isolated", HasUplink: true}
	err := ValidateIsolated(n)
	if err == nil {
		t.Fatal("ValidateIsolated with HasUplink=true on isolated mode = nil error, want rejection")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "uplink") {
		t.Errorf("ValidateIsolated error = %q, want it to name the uplink violation", err.Error())
	}
}

func TestValidateIsolated_RejectsEmptyMode(t *testing.T) {
	n := NetAttachment{Mode: "", HasUplink: false}
	if err := ValidateIsolated(n); err == nil {
		t.Fatal("ValidateIsolated with empty mode = nil error, want rejection")
	}
}

func TestValidateIsolated_RejectsUnsafeModeWithUplink(t *testing.T) {
	n := NetAttachment{Mode: "nat", HasUplink: true}
	if err := ValidateIsolated(n); err == nil {
		t.Fatal("ValidateIsolated(nat + uplink) = nil error, want rejection")
	}
}

// TestValidateIsolated_RejectsNovelUnrecognizedModes pins the allowlist
// (fail-closed) property: ValidateIsolated must reject any mode string it
// doesn't explicitly recognize as isolated, not just the four named unsafe
// modes exercised above. This guards against a future refactor turning the
// check into a denylist (which would silently admit new unsafe modes).
func TestValidateIsolated_RejectsNovelUnrecognizedModes(t *testing.T) {
	novelModes := []string{"NAT", "internet", "vpn", "tailscale", "proxy"}
	for _, mode := range novelModes {
		n := NetAttachment{Mode: mode, HasUplink: false}
		err := ValidateIsolated(n)
		if err == nil {
			t.Fatalf("ValidateIsolated(%+v) = nil error, want rejection of unrecognized mode %q", n, mode)
		}
		if !strings.Contains(err.Error(), mode) {
			t.Errorf("ValidateIsolated(%+v) error = %q, want it to name the mode %q", n, err.Error(), mode)
		}
	}
}
