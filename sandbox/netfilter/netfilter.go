// Package netfilter writes the nftables rules that give a sandbox's own
// network the internet level (.design/sandbox.md §2.2): its own loopback and
// the public internet; never the host (its loopback and every address of
// its interfaces) nor private ranges, except the private destinations the
// policy grants by address and the forwarded DNS resolver. A runtime installs
// them in the sandbox's network namespace (or a VM's kernel) before the
// sandbox starts, with privileges the sandbox does not have.
package netfilter

import (
	"bufio"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"

	"github.com/tingly-dev/tingly-box/sandbox"
)

// DNSAddr is where a sandbox with the internet level reaches the host's
// resolver, forwarded by passt or pasta; its resolv.conf names it. It is
// link-local, so the rules let it through by name.
var DNSAddr = netip.MustParseAddr("169.254.1.53")

// ResolvConf is the sandbox's resolv.conf.
func ResolvConf() []byte { return []byte("nameserver " + DNSAddr.String() + "\noptions edns0\n") }

// ResolvConfTarget is where the sandbox's resolv.conf goes: /etc/resolv.conf,
// or the file it links to.
func ResolvConfTarget() string {
	if t, err := filepath.EvalSymlinks("/etc/resolv.conf"); err == nil {
		return t
	}
	return "/etc/resolv.conf"
}

// HostResolver is the host's first nameserver, which the forwarder sends the
// sandbox's queries to.
func HostResolver() (string, error) {
	f, err := os.Open("/etc/resolv.conf")
	if err != nil {
		return "", fmt.Errorf("no resolver for the sandbox: %v", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if fields := strings.Fields(sc.Text()); len(fields) >= 2 && fields[0] == "nameserver" {
			return fields[1], nil
		}
	}
	return "", errors.New("no resolver for the sandbox: /etc/resolv.conf names no nameserver")
}

// Rules is an nft script for a sandbox whose DNS resolver is dns, granting
// the private destinations of grants (address or prefix grants, on their
// ports), on what the sandbox's own network namespace sends.
func Rules(dns netip.Addr, grants []sandbox.Grant) string {
	var b strings.Builder
	b.WriteString("table inet tb_sandbox {\n  chain output {\n    type filter hook output priority 0; policy accept;\n")
	b.WriteString("    oifname \"lo\" accept\n")
	filter(&b, dns, grants)
	b.WriteString("  }\n}\n")
	return b.String()
}

// Router is an nft script for a namespace that routes a VM's card iface,
// with the addresses of subnet, out of its own: what the VM sends is filtered
// as Rules filters, and masqueraded; the namespace itself refuses it.
func Router(dns netip.Addr, grants []sandbox.Grant, iface string, subnet netip.Prefix) string {
	var b strings.Builder
	fmt.Fprintf(&b, "table inet tb_sandbox {\n  chain input {\n    type filter hook input priority 0; policy accept;\n    iifname %q reject\n  }\n", iface)
	fmt.Fprintf(&b, "  chain forward {\n    type filter hook forward priority 0; policy accept;\n    iifname != %q accept\n", iface)
	filter(&b, dns, grants)
	b.WriteString("  }\n  chain prerouting {\n    type nat hook prerouting priority -100;\n  }\n")
	fmt.Fprintf(&b, "  chain postrouting {\n    type nat hook postrouting priority 100;\n    %s saddr %s masquerade\n  }\n}\n", family(subnet.Addr()), subnet)
	return b.String()
}

// filter writes the rules both share. In order: the resolver passes; the
// host is refused, whatever a grant says; granted destinations pass; the
// private ranges are refused; the rest (public) passes. Refused connections
// fail at once (a reset or an unreachable), not by timing out.
func filter(b *strings.Builder, dns netip.Addr, grants []sandbox.Grant) {
	if dns.IsValid() {
		fmt.Fprintf(b, "    %s daddr %s meta l4proto { tcp, udp } th dport 53 accept\n", family(dns), dns)
	}
	reject := func(p netip.Prefix) {
		fmt.Fprintf(b, "    %s daddr %s meta l4proto tcp reject with tcp reset\n", family(p.Addr()), p)
		fmt.Fprintf(b, "    %s daddr %s reject\n", family(p.Addr()), p)
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
		fmt.Fprintf(b, "    %s daddr %s meta l4proto { tcp, udp } th dport { %s } accept\n", family(g.Prefix.Addr()), g.Prefix, strings.Join(ports, ", "))
	}
	for _, p := range sandbox.PrivatePrefixes {
		reject(p)
	}
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
