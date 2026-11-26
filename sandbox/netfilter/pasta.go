package netfilter

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
)

// A runtime gives a sandbox the internet level in a network namespace of its
// own that pasta (from passt) routes out through the host's sockets, without
// mapping the host's loopback; the rules are set in that namespace before
// the sandbox starts, by a shell pasta runs (pasta's AppArmor profile lets it
// run only commands under /usr/bin), which then becomes the sandbox's first
// process. The host's resolver is forwarded at DNSAddr.

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
	checkMu sync.Mutex
	checked string
)

// Check reports whether a filtered namespace can be given here: pasta and
// nft installed, a resolver, and pasta able to give a namespace whose filter
// nft can set. A success is remembered.
func Check(ctx context.Context) error {
	checkMu.Lock()
	defer checkMu.Unlock()
	pasta, err := lookPasta()
	if err != nil {
		return errors.New("pasta is not installed: the internet level needs the passt package")
	}
	nft, err := lookNft()
	if err != nil {
		return errors.New("nft is not installed: the internet level needs the nftables package")
	}
	if checked == pasta+nft {
		return nil
	}
	if _, err := HostResolver(); err != nil {
		return err
	}
	probe := `printf 'table inet tb_probe { chain o { type filter hook output priority 0; policy accept; }; }' | "$1" -f -`
	out, err := exec.CommandContext(ctx, pasta, append(slices.Clone(pastaArgs), "--", "/usr/bin/sh", "-c", probe, "sh", nft)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("pasta cannot give a filtered network here (%v: %s)", err, strings.TrimSpace(string(out)))
	}
	checked = pasta + nft
	return nil
}

// Namespace is the command prefix that runs what follows in a new network
// namespace with the nft script set; with forward, the namespace also
// routes (a VM's card in it reaches out through it). What follows runs as
// root of a new user namespace that maps the caller; it exits 125 when the
// script cannot be set.
func Namespace(script string, forward bool) ([]string, error) {
	pasta, err1 := lookPasta()
	nft, err2 := lookNft()
	resolver, err3 := HostResolver()
	if err := errors.Join(err1, err2, err3); err != nil {
		return nil, err
	}
	sh := `printf '%s' "$1" | "$2" -f - || exit 125; shift 2; exec "$@"`
	if forward {
		sh = `echo 1 > /proc/sys/net/ipv4/ip_forward || exit 125; ` + sh
	}
	return append(append([]string{pasta}, pastaArgs...), "--dns-forward", DNSAddr.String(), "--dns-host", resolver,
		"--", "/usr/bin/sh", "-c", sh, "sh", script, nft), nil
}
