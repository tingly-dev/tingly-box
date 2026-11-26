// Package netfilter writes the nftables rules that give a sandbox's own
// network the internet level (.design/sandbox.md §2.2): its own loopback and
// the public internet; never the host (its loopback and every address of
// its interfaces) nor private ranges, except the private destinations the
// policy grants by address and the forwarded DNS resolver. A runtime installs
// them in the sandbox's network namespace (or a VM's kernel) before the
// sandbox starts, with privileges the sandbox does not have.
package netfilter

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/tingly-dev/tingly-box/sandbox"
)

// Rules is an nft script for a sandbox whose DNS resolver is dns, granting
// the private destinations of grants (address or prefix grants, on their
// ports). In order: the sandbox's own loopback and the resolver pass; the
// host is refused, whatever a grant says; granted destinations pass; the
// private ranges are refused; the rest (public) passes. Refused connections
// fail at once (a reset or an unreachable), not by timing out.
func Rules(dns netip.Addr, grants []sandbox.Grant) string {
	var b strings.Builder
	b.WriteString("table inet tb_sandbox {\n  chain output {\n    type filter hook output priority 0; policy accept;\n")
	b.WriteString("    oifname \"lo\" accept\n")
	if dns.IsValid() {
		fmt.Fprintf(&b, "    %s daddr %s meta l4proto { tcp, udp } th dport 53 accept\n", family(dns), dns)
	}
	reject := func(p netip.Prefix) {
		fmt.Fprintf(&b, "    %s daddr %s meta l4proto tcp reject with tcp reset\n", family(p.Addr()), p)
		fmt.Fprintf(&b, "    %s daddr %s reject\n", family(p.Addr()), p)
	}
	for _, p := range sandbox.LoopbackPrefixes {
		reject(p)
	}
	for _, a := range sandbox.HostAddrs() {
		reject(netip.PrefixFrom(a, a.BitLen()))
	}
	for _, g := range grants {
		ports := make([]string, len(g.Ports))
		for i, p := range g.Ports {
			ports[i] = fmt.Sprint(p)
		}
		fmt.Fprintf(&b, "    %s daddr %s meta l4proto { tcp, udp } th dport { %s } accept\n", family(g.Prefix.Addr()), g.Prefix, strings.Join(ports, ", "))
	}
	for _, p := range sandbox.PrivatePrefixes {
		reject(p)
	}
	b.WriteString("  }\n}\n")
	return b.String()
}

func family(a netip.Addr) string {
	if a.Is4() {
		return "ip"
	}
	return "ip6"
}

// Grants are allow's address and prefix grants; a grant by name cannot be
// judged by a packet filter, so it is returned as unsupported.
func Grants(allow []string) (grants []sandbox.Grant, unsupported []string) {
	for _, a := range allow {
		g, err := sandbox.ParseGrant(a)
		if err != nil || !g.Prefix.IsValid() {
			unsupported = append(unsupported, a)
			continue
		}
		grants = append(grants, g)
	}
	return grants, unsupported
}
