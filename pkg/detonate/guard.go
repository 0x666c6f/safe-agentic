package detonate

import "fmt"

// isolatedModes are the only network attachment modes considered safe for
// detonating an untrusted sample: no path to a real uplink.
var isolatedModes = map[string]bool{
	"isolated":  true,
	"host-none": true,
	"fakenet":   true,
}

// NetAttachment describes how a detonation VM/container is attached to the
// network.
type NetAttachment struct {
	Mode      string
	HasUplink bool
}

// ValidateIsolated is the load-bearing containment guard: it must reject any
// attachment that could let a detonating sample reach a real network. It
// passes only when Mode is one of the isolated modes AND HasUplink is false;
// either condition failing is named explicitly in the returned error.
func ValidateIsolated(n NetAttachment) error {
	if !isolatedModes[n.Mode] {
		return fmt.Errorf("containment violation: network mode %q is not isolated (must be isolated, host-none, or fakenet)", n.Mode)
	}
	if n.HasUplink {
		return fmt.Errorf("containment violation: network attachment has an uplink despite mode %q — uplink must be false for isolation", n.Mode)
	}
	return nil
}
