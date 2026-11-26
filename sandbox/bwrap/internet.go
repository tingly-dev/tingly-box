package bwrap

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/tingly-dev/tingly-box/sandbox"
	"github.com/tingly-dev/tingly-box/sandbox/netfilter"
)

// The internet level: pasta (from passt) gives the sandbox a network of its
// own with a route out through the host, without mapping the host's
// loopback; nftables rules set in that network before bubblewrap starts, and
// out of the sandbox's reach (it has no capabilities there), refuse the host
// and private ranges (sandbox/netfilter); the host's resolver is forwarded
// at dnsAddr, which the sandbox's resolv.conf names. pasta's AppArmor
// profile lets it run only commands under /usr/bin, so it runs the shell
// that applies the rules and becomes bubblewrap.

// dnsAddr is where the sandbox reaches the host's resolver.
var dnsAddr = netip.MustParseAddr("169.254.1.53")

// pastaArgs give the namespace a network and nothing of the host: no mapping
// of the gateway to the host, and no port forwarding either way (by default
// pasta forwards ports the host listens on, its loopback's among them, into
// the namespace).
var pastaArgs = []string{"--config-net", "--quiet", "--no-map-gw", "-t", "none", "-u", "none", "-T", "none", "-U", "none"}

// lookPasta and lookNft find the tools; variables for tests.
var (
	lookPasta = func() (string, error) { return exec.LookPath("pasta") }
	lookNft   = func() (string, error) { return lookIn("nft", "/usr/sbin/nft", "/sbin/nft") }
)

func lookIn(name string, paths ...string) (string, error) {
	if p, err := exec.LookPath(name); err == nil {
		return p, nil
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s is not installed", name)
}

var (
	internetMu      sync.Mutex
	internetChecked string
)

// CheckInternet reports whether the internet level can be given here: pasta
// and nft installed, and pasta able to give a namespace whose filter nft can
// set. A success is remembered.
func CheckInternet(ctx context.Context) error {
	internetMu.Lock()
	defer internetMu.Unlock()
	pasta, err := lookPasta()
	if err != nil {
		return errors.New("pasta is not installed: the internet level needs the passt package")
	}
	nft, err := lookNft()
	if err != nil {
		return errors.New("nft is not installed: the internet level needs the nftables package")
	}
	if internetChecked == pasta+nft {
		return nil
	}
	if _, err := hostResolver(); err != nil {
		return err
	}
	probe := `printf 'table inet tb_probe { chain o { type filter hook output priority 0; policy accept; }; }' | "$1" -f -`
	out, err := exec.CommandContext(ctx, pasta, append(slices.Clone(pastaArgs), "--", "/usr/bin/sh", "-c", probe, "sh", nft)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("pasta cannot give a filtered network here (%v: %s)", err, strings.TrimSpace(string(out)))
	}
	internetChecked = pasta + nft
	return nil
}

// hostResolver is the host's first nameserver, which pasta forwards to.
func hostResolver() (string, error) {
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
	pasta, err1 := lookPasta()
	nft, err2 := lookNft()
	resolver, err3 := hostResolver()
	if err := errors.Join(err1, err2, err3); err != nil {
		return nil, fmt.Errorf("%w: %v", sandbox.ErrUnavailable, err)
	}
	dir, err := os.MkdirTemp("", "tb-dns-")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", sandbox.ErrUnavailable, err)
	}
	conf := filepath.Join(dir, "resolv.conf")
	if err := os.WriteFile(conf, []byte("nameserver "+dnsAddr.String()+"\noptions edns0\n"), 0o644); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("%w: %v", sandbox.ErrUnavailable, err)
	}
	// The sandbox's /etc/resolv.conf, or the file it links to.
	target := "/etc/resolv.conf"
	if t, err := filepath.EvalSymlinks(target); err == nil {
		target = t
	}
	in := &internet{dir: dir,
		via: append(append([]string{pasta}, pastaArgs...), "--dns-forward", dnsAddr.String(), "--dns-host", resolver,
			"--", "/usr/bin/sh", "-c", `printf '%s' "$1" | "$2" -f - || exit 125; shift 2; exec "$@"`, "sh",
			netfilter.Rules(dnsAddr, grants), nft),
		// pasta maps the user to root in its namespace: bubblewrap maps it
		// back, so the sandbox runs as the host user, as without pasta.
		extra: []string{"--ro-bind", conf, target, "--uid", strconv.Itoa(os.Getuid()), "--gid", strconv.Itoa(os.Getgid())},
	}
	return in, nil
}

func (in *internet) Close() error { return os.RemoveAll(in.dir) }
