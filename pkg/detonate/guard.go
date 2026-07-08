package detonate

import (
	"fmt"
	"net"
)

// privateBlocks are the only network ranges a softnet allow-list may name:
// RFC1918 private space plus IPv4 link-local. A softnet allow-list restricted
// to one of these can reach the operator-provisioned fakenet gateway and
// nothing routable — the whole point of code-enforced isolation.
var privateBlocks = func() []*net.IPNet {
	var out []*net.IPNet
	for _, cidr := range []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16", // link-local
	} {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			panic("detonate: bad private block constant: " + cidr) // unreachable: constants
		}
		out = append(out, n)
	}
	return out
}()

// validateSoftnetAllow is the load-bearing containment control for the
// softnet path: the `--net-softnet-allow` value MUST be a single private /
// isolated CIDR wholly contained in one privateBlock (or a /32 within one).
// A permissive allow-list (0.0.0.0/0, ::/0, any public/routable range) would
// hand the detonating sample internet egress, so anything not provably
// private is rejected — fail closed.
func validateSoftnetAllow(cidr string) error {
	if cidr == "" {
		return fmt.Errorf("containment violation: softnet allow-list is empty (must be a single private CIDR)")
	}
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return fmt.Errorf("containment violation: softnet allow-list %q is not a valid CIDR: %w", cidr, err)
	}
	// IPv6 (incl. ::/0) has no allowed private block here — reject outright.
	if ip.To4() == nil {
		return fmt.Errorf("containment violation: softnet allow-list %q is not an IPv4 private CIDR", cidr)
	}
	ones, _ := ipnet.Mask.Size()
	for _, b := range privateBlocks {
		bOnes, _ := b.Mask.Size()
		// Contained iff the network address sits inside the private block AND
		// the prefix is at least as long, so every address in the range does
		// too. This rejects 0.0.0.0/0 (prefix 0) and any public range.
		if b.Contains(ipnet.IP) && ones >= bOnes {
			return nil
		}
	}
	return fmt.Errorf("containment violation: softnet allow-list %q is not within a private range (allowed: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 169.254.0.0/16)", cidr)
}

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
