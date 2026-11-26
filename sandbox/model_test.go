package sandbox

import (
	"net/netip"
	"strings"
	"testing"
)

func confined() Policy {
	return Policy{
		Storage:     Storage{Level: StorageConfined, Home: "/h"},
		Network:     Network{Level: NetworkNone},
		Exec:        Exec{Level: ExecConfined},
		Credentials: Credentials{Level: CredentialsExplicit},
		Resources:   Resources{Level: ResourcesUnlimited},
	}
}

func TestWellFormed(t *testing.T) {
	if err := confined().WellFormed(); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		edit func(*Policy)
		want string
	}{
		"no level":                 {func(p *Policy) { p.Exec.Level = "" }, "exec: no level"},
		"another axis's level":     {func(p *Policy) { p.Network.Level = Level("confined") }, "network: no level"},
		"relative home":            {func(p *Policy) { p.Storage.Home = "h" }, "home"},
		"relative mount":           {func(p *Policy) { p.Storage.Mounts = []Mount{{Path: "w"}} }, "mount"},
		"expose with open":         {func(p *Policy) { p.Network = Network{Level: NetworkOpen, Expose: []Endpoint{{Port: 80}}} }, "expose"},
		"repeated expose":          {func(p *Policy) { p.Network.Expose = []Endpoint{{Port: 80}, {Port: 80}} }, "repeated"},
		"expose out of range":      {func(p *Policy) { p.Network.Expose = []Endpoint{{Port: 70000}} }, "out of range"},
		"allow with none":          {func(p *Policy) { p.Network.Allow = []string{"example.com"} }, "allow means nothing"},
		"allow names the loopback": {func(p *Policy) { p.Network = Network{Level: NetworkAllowlist, Allow: []string{"127.0.0.1:8080"}} }, "is the host"},
		"allow names localhost":    {func(p *Policy) { p.Network = Network{Level: NetworkAllowlist, Allow: []string{"localhost"}} }, "is the host"},
		"bad allow":                {func(p *Policy) { p.Network = Network{Level: NetworkAllowlist, Allow: []string{"a b"}} }, "not a host name"},
		"limited without a limit":  {func(p *Policy) { p.Resources = Resources{Level: ResourcesLimited} }, "sets no limit"},
		"unlimited with a limit":   {func(p *Policy) { p.Resources = Resources{Level: ResourcesUnlimited, Tasks: 5} }, "sets a limit"},
		"negative limit":           {func(p *Policy) { p.Resources = Resources{Level: ResourcesLimited, CPU: -1} }, "negative"},
	} {
		p := confined()
		tc.edit(&p)
		err := p.WellFormed()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
	}
	p := confined()
	p.Network = Network{Level: NetworkInternet, Allow: []string{"registry.corp.lan", "10.0.0.0/8"}, Expose: []Endpoint{{Port: 12580}}}
	p.Resources = Resources{Level: ResourcesLimited, CPU: 500}
	if err := p.WellFormed(); err != nil {
		t.Errorf("a well-formed internet policy: %v", err)
	}
}

func withHostAddrs(t *testing.T, addrs ...string) {
	old := hostAddrs
	hostAddrs = func() []netip.Addr {
		var out []netip.Addr
		for _, a := range addrs {
			out = append(out, netip.MustParseAddr(a))
		}
		return out
	}
	t.Cleanup(func() { hostAddrs = old })
}

func TestClassify(t *testing.T) {
	withHostAddrs(t, "192.168.1.5", "203.0.113.7")
	for ip, want := range map[string]Surface{
		"127.0.0.1": SurfaceHost, "::1": SurfaceHost, "0.0.0.0": SurfaceHost,
		"192.168.1.5": SurfaceHost, "203.0.113.7": SurfaceHost, // the host's own, a public one too
		"::ffff:127.0.0.1": SurfaceHost,
		"192.168.1.9":      SurfacePrivate, "10.1.2.3": SurfacePrivate, "172.17.0.2": SurfacePrivate,
		"169.254.169.254": SurfacePrivate, "100.64.0.1": SurfacePrivate, "fd00::1": SurfacePrivate, "fe80::1": SurfacePrivate,
		"::ffff:10.0.0.1": SurfacePrivate,
		"93.184.216.34":   SurfacePublic, "2606:2800:220:1::": SurfacePublic,
	} {
		if got := Classify(netip.MustParseAddr(ip)); got != want {
			t.Errorf("Classify(%s) = %d, want %d", ip, got, want)
		}
	}
}

func TestGrantsFollowTheModel(t *testing.T) {
	withHostAddrs(t, "192.168.1.5")
	public, private, host := netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("10.0.0.7"), netip.MustParseAddr("192.168.1.5")
	grant := func(s string) Grant {
		g, err := ParseGrant(s)
		if err != nil {
			t.Fatalf("%s: %v", s, err)
		}
		return g
	}
	for _, tc := range []struct {
		grant, name string
		ip          netip.Addr
		port        int
		want        bool
	}{
		{"example.com", "example.com", public, 443, true},
		{"example.com", "EXAMPLE.com.", public, 80, true},
		{"example.com", "example.com", public, 22, false}, // 80 and 443 without a port
		{"example.com:22", "example.com", public, 22, true},
		{"example.com", "evil.com", public, 443, false},
		{"example.com", "example.com", private, 443, true}, // an exact name may grant a private address
		{"example.com", "example.com", host, 443, false},   // never the host
		{"*.example.com", "api.example.com", public, 443, true},
		{"*.example.com", "example.com", public, 443, false},      // subdomains only
		{"*.example.com", "api.example.com", private, 443, false}, // a wildcard never grants private
		{"*.example.com", "notexample.com", public, 443, false},
		{"10.0.0.0/8", "", private, 443, true},
		{"10.0.0.0/8", "anything.lan", private, 443, true}, // a prefix grants the address however it was named
		{"192.168.0.0/16", "", host, 443, false},           // a prefix never covers the host
		{"93.184.216.34:8443", "", public, 8443, true},
		{"[2606:2800:220:1::]:443", "", netip.MustParseAddr("2606:2800:220:1::"), 443, true},
	} {
		if got := grant(tc.grant).Permits(tc.name, tc.ip, tc.port); got != tc.want {
			t.Errorf("%s permits %s/%s:%d = %v, want %v", tc.grant, tc.name, tc.ip, tc.port, got, tc.want)
		}
	}
}

func TestExitError(t *testing.T) {
	e := &ExitError{Code: 137, Reason: EndOOM}
	if e.Error() != "out of memory" || e.ExitCode() != 137 {
		t.Errorf("%v %d", e, e.ExitCode())
	}
	if (&ExitError{Code: 2, Reason: EndExited}).Error() != "exit status 2" {
		t.Error("exit status")
	}
}
