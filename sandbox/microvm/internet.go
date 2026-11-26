package microvm

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strings"

	"github.com/tingly-dev/tingly-box/sandbox"
	"github.com/tingly-dev/tingly-box/sandbox/netfilter"
)

// The internet level: Cloud Hypervisor runs in a network namespace that
// pasta routes out through the host (netfilter.Namespace); the guest's card
// is a tap in that namespace, which forwards and masquerades what the guest
// sends, filtered by netfilter.Router outside the guest's reach. The host's
// resolver is forwarded at netfilter.DNSAddr, which the guest's resolv.conf
// names.

// The guest's network, on a tap in the VMM's namespace.
const (
	guestTap = "tbtap0"
	guestMAC = "52:54:00:12:34:56"
)

var (
	guestSubnet  = netip.MustParsePrefix("10.0.2.0/24")
	guestGateway = netip.MustParseAddr("10.0.2.1")
	guestAddr    = netip.PrefixFrom(netip.MustParseAddr("10.0.2.15"), 24)
)

// guestNet is what the guest init sets up for the internet level.
type guestNet struct {
	Addr    string `json:"addr"`
	Gateway string `json:"gateway"`
}

// internet is one VM's internet level on the host.
type internet struct {
	via   []string // pasta and the shell that sets the filter, before the VMM
	conf  string   // the guest's resolv.conf
	guest guestNet
}

// newInternet writes the guest's resolv.conf in dir.
func newInternet(ctx context.Context, dir string, n sandbox.Network) (*internet, error) {
	grants, byName := netfilter.Grants(n.Allow)
	if len(byName) > 0 {
		return nil, fmt.Errorf("%w: microvm grants private destinations under internet by address only, not %s", sandbox.ErrUnavailable, strings.Join(byName, ", "))
	}
	if err := netfilter.Check(ctx); err != nil {
		return nil, fmt.Errorf("%w: %v", sandbox.ErrUnavailable, err)
	}
	via, err := netfilter.Namespace(netfilter.Router(netfilter.DNSAddr, grants, guestTap, guestSubnet), true)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", sandbox.ErrUnavailable, err)
	}
	in := &internet{via: via, conf: filepath.Join(dir, "resolv.conf"),
		guest: guestNet{Addr: guestAddr.String(), Gateway: guestGateway.String()}}
	if err := os.WriteFile(in.conf, netfilter.ResolvConf(), 0o644); err != nil {
		return nil, fmt.Errorf("%w: %v", sandbox.ErrUnavailable, err)
	}
	return in, nil
}

// vmmArgs give the VM its card, a tap the VMM creates in the namespace.
func (in *internet) vmmArgs() []string {
	return []string{"--net", fmt.Sprintf("tap=%s,ip=%s,mask=255.255.255.0,mac=%s", guestTap, guestGateway, guestMAC)}
}
