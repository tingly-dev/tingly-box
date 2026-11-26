package sandbox

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// AxisLevels are the levels each axis has, from strict to wide.
var AxisLevels = map[Axis][]Level{
	AxisStorage:     {StorageConfined, StorageHost},
	AxisNetwork:     {NetworkNone, NetworkAllowlist, NetworkInternet, NetworkOpen},
	AxisExec:        {ExecConfined, ExecHost},
	AxisCredentials: {CredentialsExplicit, CredentialsInherit},
	AxisResources:   {ResourcesLimited, ResourcesUnlimited},
}

// WellFormed reports whether p is a policy at all (.design/sandbox.md §2.6):
// every axis has a level of its own, given explicitly (there are no
// defaults), and its parameters make sense for that level.
func (p Policy) WellFormed() error {
	var errs []error
	for _, a := range Axes {
		if l := p.Level(a); !slices.Contains(AxisLevels[a], l) {
			errs = append(errs, fmt.Errorf("%s: no level %q", a, l))
		}
	}
	if p.Storage.Level == StorageConfined {
		if !filepath.IsAbs(p.Storage.Home) {
			errs = append(errs, fmt.Errorf("storage: home %q is not an absolute path", p.Storage.Home))
		}
		for _, m := range p.Storage.Mounts {
			if !filepath.IsAbs(m.Path) {
				errs = append(errs, fmt.Errorf("storage: mount %q is not an absolute path", m.Path))
			}
		}
	}
	n := p.Network
	if n.Level == NetworkOpen && len(n.Expose) > 0 {
		errs = append(errs, errors.New("network: expose means nothing with open, which reaches the host anyway"))
	}
	seen := map[int]bool{}
	for _, e := range n.Expose {
		if e.Port < 1 || e.Port > 65535 || seen[e.Port] {
			errs = append(errs, fmt.Errorf("network: expose port %d is out of range or repeated", e.Port))
		}
		seen[e.Port] = true
	}
	if len(n.Allow) > 0 && n.Level != NetworkAllowlist && n.Level != NetworkInternet {
		errs = append(errs, fmt.Errorf("network: allow means nothing with %s", n.Level))
	}
	for _, a := range n.Allow {
		g, err := ParseGrant(a)
		if err == nil && g.namesHost() {
			err = fmt.Errorf("%q is the host: reach the host's services with expose", a)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("network: allow: %v", err))
		}
	}
	r := p.Resources
	if r.MemoryBytes < 0 || r.Tasks < 0 || r.CPU < 0 {
		errs = append(errs, errors.New("resources: a limit is negative"))
	}
	set := r.MemoryBytes > 0 || r.Tasks > 0 || r.CPU > 0
	switch {
	case r.Level == ResourcesLimited && !set:
		errs = append(errs, errors.New("resources: limited sets no limit"))
	case r.Level == ResourcesUnlimited && set:
		errs = append(errs, errors.New("resources: unlimited sets a limit"))
	}
	return errors.Join(errs...)
}

// Surface is where an address lands, seen from the host (§2.2).
type Surface int

const (
	// SurfaceHost is the host itself: its loopback and every address of its
	// own interfaces, public ones included.
	SurfaceHost Surface = iota
	// SurfacePrivate is private and special ranges that are not the host.
	SurfacePrivate
	// SurfacePublic is everything else.
	SurfacePublic
)

// cgnat is the shared address space of RFC 6598.
var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// hostAddrs are the host's own interface addresses; a variable for tests.
var hostAddrs = func() []netip.Addr {
	addrs, _ := net.InterfaceAddrs()
	out := make([]netip.Addr, 0, len(addrs))
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok {
			if ip, ok := netip.AddrFromSlice(n.IP); ok {
				out = append(out, ip.Unmap())
			}
		}
	}
	return out
}

// Classify is the surface ip lands on.
func Classify(ip netip.Addr) Surface {
	ip = ip.Unmap()
	if ip.IsLoopback() || ip.IsUnspecified() || slices.Contains(hostAddrs(), ip) {
		return SurfaceHost
	}
	if ip.IsPrivate() || ip.IsLinkLocalUnicast() || cgnat.Contains(ip) ||
		ip.IsMulticast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() || !ip.IsGlobalUnicast() {
		return SurfacePrivate
	}
	return SurfacePublic
}

// Grant is one Network.Allow entry: a wildcard "*.example.com" (its
// subdomains), an exact host name, or an IP address or CIDR prefix; each with
// an optional ":port" (80 and 443 without one).
type Grant struct {
	Wildcard bool         // Name is the domain whose subdomains it grants
	Name     string       // lower case, no trailing dot; empty for an address
	Prefix   netip.Prefix // valid for an address or a prefix
	Ports    []int
}

// ParseGrant parses one Network.Allow entry.
func ParseGrant(s string) (Grant, error) {
	host, ports := s, []int{80, 443}
	if h, p, err := net.SplitHostPort(s); err == nil {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return Grant{}, fmt.Errorf("%q: bad port", s)
		}
		host, ports = h, []int{n}
	} else if strings.HasPrefix(s, "[") {
		host = strings.Trim(s, "[]")
	}
	g := Grant{Ports: ports}
	if pfx, err := netip.ParsePrefix(host); err == nil {
		g.Prefix = pfx.Masked()
		return g, nil
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		g.Prefix = netip.PrefixFrom(ip.Unmap(), ip.Unmap().BitLen())
		return g, nil
	}
	name := strings.ToLower(strings.TrimSuffix(host, "."))
	if d, ok := strings.CutPrefix(name, "*."); ok {
		g.Wildcard, name = true, d
	}
	if name == "" || strings.ContainsAny(name, "*/ ") {
		return Grant{}, fmt.Errorf("%q is not a host name, address or prefix", s)
	}
	g.Name = name
	return g, nil
}

// namesHost reports whether g names the host literally: a loopback or host
// address, a prefix within loopback, or localhost.
func (g Grant) namesHost() bool {
	if g.Prefix.IsValid() {
		a := g.Prefix.Addr()
		return a.IsLoopback() || (g.Prefix.IsSingleIP() && Classify(a) == SurfaceHost)
	}
	return g.Name == "localhost" || strings.HasSuffix(g.Name, ".localhost")
}

// Permits reports whether g grants connecting to ip:port, reached by name
// ("" when the connection names the address itself). It never grants the
// host; a wildcard grants only public addresses.
func (g Grant) Permits(name string, ip netip.Addr, port int) bool {
	if !slices.Contains(g.Ports, port) {
		return false
	}
	ip = ip.Unmap()
	surface := Classify(ip)
	if surface == SurfaceHost {
		return false
	}
	name = strings.ToLower(strings.TrimSuffix(name, "."))
	switch {
	case g.Prefix.IsValid():
		return g.Prefix.Contains(ip)
	case g.Wildcard:
		return surface == SurfacePublic && strings.HasSuffix(name, "."+g.Name)
	default:
		return name != "" && name == g.Name
	}
}

// MayName reports whether g could grant something reached by name: a prefix
// could, whatever the name; a name grant only for its own name.
func (g Grant) MayName(name string) bool {
	name = strings.ToLower(strings.TrimSuffix(name, "."))
	switch {
	case g.Prefix.IsValid():
		return true
	case g.Wildcard:
		return strings.HasSuffix(name, "."+g.Name)
	default:
		return name == g.Name
	}
}

// End is why a sandboxed process ended.
type End string

const (
	EndExited   End = "exited"   // it exited by itself
	EndSignaled End = "signaled" // a signal ended it
	EndOOM      End = "oom"      // the sandbox ran out of memory
	EndKilled   End = "killed"   // the host ended it (Kill)
	EndFailed   End = "failed"   // the sandbox itself failed
)

// ExitError is what Proc.Wait returns for any end but a zero exit.
type ExitError struct {
	Code   int // the exit code; 128 plus the signal when a signal ended it
	Reason End
	Detail string // for people: what the sandbox saw, when it failed
}

func (e *ExitError) Error() string {
	msg := string(e.Reason)
	switch e.Reason {
	case EndExited, EndSignaled:
		msg = "exit status " + strconv.Itoa(e.Code)
	case EndOOM:
		msg = "out of memory"
	}
	if e.Detail != "" {
		msg += ": " + e.Detail
	}
	return msg
}

// ExitCode is the exit code, as exec.ExitError has it.
func (e *ExitError) ExitCode() int { return e.Code }

// Usage is what a runtime that accounts for its sandboxes can report; a Proc
// implements it when its runtime can.
type Usage interface {
	Usage() (Stats, error)
}

// Stats is a sandbox's use of resources so far.
type Stats struct {
	MemoryBytes     int64 // now
	PeakMemoryBytes int64
	Tasks           int // now
	CPU             time.Duration
}
