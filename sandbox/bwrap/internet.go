package bwrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tingly-dev/tingly-box/sandbox"
	"github.com/tingly-dev/tingly-box/sandbox/netfilter"
)

// The internet level: bubblewrap runs in a namespace pasta routes out
// through the host (netfilter.Namespace), whose nftables rules, set before
// bubblewrap starts and out of the sandbox's reach (it has no capabilities
// there), refuse the host and private ranges.

// CheckInternet reports whether the internet level can be given here.
func CheckInternet(ctx context.Context) error { return netfilter.Check(ctx) }

// internet is one sandbox's internet level.
type internet struct {
	via   []string // pasta and the shell that sets the filter, before bwrap
	extra []string // bubblewrap arguments
	dir   string   // the sandbox's resolv.conf
}

func newInternet(ctx context.Context, n sandbox.Network) (*internet, error) {
	grants, byName := netfilter.Grants(n.Allow)
	if len(byName) > 0 {
		return nil, fmt.Errorf("%w: bwrap grants private destinations under internet by address only, not %s", sandbox.ErrUnavailable, strings.Join(byName, ", "))
	}
	if err := CheckInternet(ctx); err != nil {
		return nil, fmt.Errorf("%w: %v", sandbox.ErrUnavailable, err)
	}
	via, err := netfilter.Namespace(netfilter.Rules(netfilter.DNSAddr, grants), false)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", sandbox.ErrUnavailable, err)
	}
	dir, err := os.MkdirTemp("", "tb-dns-")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", sandbox.ErrUnavailable, err)
	}
	conf := filepath.Join(dir, "resolv.conf")
	if err := os.WriteFile(conf, netfilter.ResolvConf(), 0o644); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("%w: %v", sandbox.ErrUnavailable, err)
	}
	in := &internet{dir: dir, via: via,
		// pasta maps the user to root in its namespace: bubblewrap maps it
		// back, so the sandbox runs as the host user, as without pasta.
		extra: []string{"--ro-bind", conf, netfilter.ResolvConfTarget(), "--uid", strconv.Itoa(os.Getuid()), "--gid", strconv.Itoa(os.Getgid())},
	}
	return in, nil
}

func (in *internet) Close() error { return os.RemoveAll(in.dir) }
