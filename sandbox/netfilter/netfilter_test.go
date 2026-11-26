package netfilter

import (
	"net/netip"
	"os/exec"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/sandbox"
)

func TestRulesRefuseTheHostBeforeAnyGrant(t *testing.T) {
	grants, unsupported := Grants([]string{"10.1.0.0/16", "registry.lan", "192.168.0.0/16:5000"})
	if len(grants) != 2 || len(unsupported) != 1 || unsupported[0] != "registry.lan" {
		t.Fatalf("grants %v unsupported %v", grants, unsupported)
	}
	r := Rules(netip.MustParseAddr("169.254.1.53"), grants)
	host := strings.Index(r, "ip daddr 127.0.0.0/8")
	grant := strings.Index(r, "ip daddr 10.1.0.0/16")
	private := strings.Index(r, "ip daddr 10.0.0.0/8")
	if !(0 < host && host < grant && grant < private) {
		t.Errorf("order: host %d, grant %d, private %d:\n%s", host, grant, private, r)
	}
	for _, want := range []string{"dport { 80, 443 } accept", "dport { 5000 } accept", "169.254.1.53 meta l4proto { tcp, udp } th dport 53 accept"} {
		if !strings.Contains(r, want) {
			t.Errorf("missing %q:\n%s", want, r)
		}
	}
	for _, a := range sandbox.HostAddrs() {
		if !strings.Contains(r, " "+netip.PrefixFrom(a, a.BitLen()).String()+" ") {
			t.Errorf("the host's address %s is not refused", a)
		}
	}
	// nft accepts the script (checked without applying it).
	if nft, err := exec.LookPath("nft"); err == nil {
		cmd := exec.Command(nft, "-c", "-f", "-")
		cmd.Stdin = strings.NewReader(r)
		if out, err := cmd.CombinedOutput(); err != nil && !strings.Contains(string(out), "Operation not permitted") {
			t.Errorf("nft -c: %v: %s", err, out)
		}
	}
}
