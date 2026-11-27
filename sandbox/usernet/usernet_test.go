package usernet

import (
	"context"
	"io"
	"net"
	"net/netip"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/link/ethernet"
	"gvisor.dev/gvisor/pkg/tcpip/network/arp"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"

	"github.com/tingly-dev/tingly-box/sandbox"
	"github.com/tingly-dev/tingly-box/sandbox/netfilter"
)

// datagramPair is the network's pair, with the guest end non-blocking too,
// so the test's guest stops reading when it is closed.
func datagramPair(t *testing.T) (host, guest *os.File) {
	t.Helper()
	host, guest, err := pair(true)
	if err != nil {
		t.Fatal(err)
	}
	return host, guest
}

// guestStack is a guest: its own stack, its card on link, set up as the
// guest init sets up a VM's.
func guestStack(t *testing.T, link *os.File) *stack.Stack {
	t.Helper()
	s := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol, arp.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol},
	})
	ep := channel.New(512, MTU, tcpip.LinkAddress("\x52\x54\x00\x12\x34\x56"))
	if err := s.CreateNIC(1, ethernet.New(ep)); err != nil {
		t.Fatal(err)
	}
	if err := s.AddProtocolAddress(1, tcpip.ProtocolAddress{Protocol: ipv4.ProtocolNumber,
		AddressWithPrefix: tcpip.AddressWithPrefix{Address: tcpip.AddrFrom4(Guest.Addr().As4()), PrefixLen: Guest.Bits()}}, stack.AddressProperties{}); err != nil {
		t.Fatal(err)
	}
	sub, _ := tcpip.NewSubnet(tcpip.AddrFrom4([4]byte{}), tcpip.MaskFromBytes(make([]byte, 4)))
	s.SetRouteTable([]tcpip.Route{{Destination: sub, Gateway: tcpip.AddrFrom4(Gateway.As4()), NIC: 1}})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for {
			pkt := ep.ReadContext(ctx)
			if pkt == nil {
				return
			}
			_, _ = link.Write(pkt.ToView().AsSlice())
			pkt.DecRef()
		}
	}()
	go func() {
		buf := make([]byte, 64<<10)
		for {
			n, err := link.Read(buf)
			if err != nil {
				return
			}
			pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(append([]byte(nil), buf[:n]...))})
			ep.InjectInbound(0, pkt)
			pkt.DecRef()
		}
	}()
	t.Cleanup(func() { cancel(); ep.Close(); s.Close() })
	return s
}

// world stands for the rest of the internet: every connection the network
// makes from the host lands on a local echo server, and is recorded.
type world struct {
	mu     sync.Mutex
	dialed []string
	tcp    net.Listener
	udp    net.PacketConn
}

func newWorld(t *testing.T) *world {
	w := &world{}
	var err error
	if w.tcp, err = net.Listen("tcp", "127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	if w.udp, err = net.ListenPacket("udp", "127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.tcp.Close(); _ = w.udp.Close() })
	go func() {
		for {
			c, err := w.tcp.Accept()
			if err != nil {
				return
			}
			go func() { _, _ = io.Copy(c, c); _ = c.Close() }()
		}
	}()
	go func() {
		buf := make([]byte, 2048)
		for {
			n, from, err := w.udp.ReadFrom(buf)
			if err != nil {
				return
			}
			_, _ = w.udp.WriteTo(append([]byte("re:"), buf[:n]...), from)
		}
	}()
	return w
}

func (w *world) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	w.mu.Lock()
	w.dialed = append(w.dialed, network+" "+addr)
	w.mu.Unlock()
	to := w.tcp.Addr().String()
	if network == "udp" {
		to = w.udp.LocalAddr().String()
	}
	var d net.Dialer
	return d.DialContext(ctx, network, to)
}

func (w *world) reached() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.dialed)
}

func serve(t *testing.T, grants ...string) (*stack.Stack, *world) {
	t.Helper()
	host, guest := datagramPair(t)
	w := newWorld(t)
	var gs []sandbox.Grant
	for _, a := range grants {
		g, err := sandbox.ParseGrant(a)
		if err != nil {
			t.Fatal(err)
		}
		gs = append(gs, g)
	}
	n, err := Serve(host, Config{Grants: gs, Resolver: "192.0.2.53", Dial: w.dial})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = n.Close(); _ = guest.Close() })
	return guestStack(t, guest), w
}

// connects reports whether the guest gets a working TCP connection to addr.
func connects(t *testing.T, s *stack.Stack, addr string) bool {
	t.Helper()
	ap := netip.MustParseAddrPort(addr)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := gonet.DialContextTCP(ctx, s, tcpip.FullAddress{NIC: 1, Addr: tcpip.AddrFrom4(ap.Addr().As4()), Port: ap.Port()}, ipv4.ProtocolNumber)
	if err != nil {
		return false
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Write([]byte("ping")); err != nil {
		return false
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(c, buf); err != nil || string(buf) != "ping" {
		t.Fatalf("%s: echo %q, %v", addr, buf, err)
	}
	return true
}

func TestInternetReachesThePublicInternetOnly(t *testing.T) {
	s, w := serve(t)
	if !connects(t, s, "93.184.216.34:443") {
		t.Fatal("a public address is not reachable")
	}
	if !connects(t, s, "93.184.216.34:22") {
		t.Fatal("a public address is not reachable on another port")
	}
	refused := []string{
		"127.0.0.1:80",       // the host's loopback
		"10.0.2.1:22",        // the gateway, the host side
		"10.0.2.99:80",       // the guest's own network
		"192.168.1.10:80",    // private, not granted
		"169.254.169.254:80", // cloud metadata
		netfilter.DNSAddr.String() + ":80",
	}
	for _, a := range sandbox.HostAddrs() {
		if a.Is4() && !a.IsLoopback() {
			refused = append(refused, a.String()+":80") // the host's own interfaces
		}
	}
	for _, a := range refused {
		if connects(t, s, a) {
			t.Errorf("%s is reachable", a)
		}
	}
	want := []string{"tcp 93.184.216.34:443", "tcp 93.184.216.34:22"}
	if got := w.reached(); !slices.Equal(got, want) {
		t.Fatalf("the host connected to %q, want only %q", got, want)
	}
}

func TestInternetGrantsPrivateAddressesByAddress(t *testing.T) {
	s, w := serve(t, "192.168.1.0/24:8080", "localhost", "127.0.0.1")
	if !connects(t, s, "192.168.1.10:8080") {
		t.Fatal("a granted private address is not reachable")
	}
	for _, a := range []string{"192.168.1.10:80", "192.168.2.10:8080", "127.0.0.1:80"} {
		if connects(t, s, a) {
			t.Errorf("%s is reachable", a)
		}
	}
	if got := w.reached(); !slices.Equal(got, []string{"tcp 192.168.1.10:8080"}) {
		t.Fatalf("the host connected to %q", got)
	}
}

func TestInternetForwardsDNSAndUDP(t *testing.T) {
	s, w := serve(t)
	exchange := func(addr string) (string, bool) {
		ap := netip.MustParseAddrPort(addr)
		c, err := gonet.DialUDP(s, nil, &tcpip.FullAddress{NIC: 1, Addr: tcpip.AddrFrom4(ap.Addr().As4()), Port: ap.Port()}, ipv4.ProtocolNumber)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		_ = c.SetDeadline(time.Now().Add(2 * time.Second))
		if _, err := c.Write([]byte("q")); err != nil {
			return "", false
		}
		buf := make([]byte, 64)
		n, err := c.Read(buf)
		if err != nil {
			return "", false
		}
		return string(buf[:n]), true
	}
	if got, ok := exchange(netfilter.DNSAddr.String() + ":53"); !ok || got != "re:q" {
		t.Fatalf("DNS: %q, %v", got, ok)
	}
	if got, ok := exchange("93.184.216.34:443"); !ok || got != "re:q" {
		t.Fatalf("public UDP: %q, %v", got, ok)
	}
	if _, ok := exchange("127.0.0.1:53"); ok {
		t.Fatal("the host's loopback answers UDP")
	}
	want := []string{"udp 192.0.2.53:53", "udp 93.184.216.34:443"}
	if got := w.reached(); !slices.Equal(got, want) {
		t.Fatalf("the host sent to %q, want %q", got, want)
	}
}

// Close returns while the network is blocked reading the guest's card (a
// blocking descriptor in an *os.File would keep the read, and Close, waiting).
func TestCloseReturnsWhileReading(t *testing.T) {
	host, guest, err := Pair()
	if err != nil {
		t.Fatal(err)
	}
	defer guest.Close()
	n, err := Serve(host, Config{})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	closed := make(chan struct{})
	go func() { _ = n.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return")
	}
}
