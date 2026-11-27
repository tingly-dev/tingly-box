// Package usernet gives a virtual machine the internet level
// (.design/sandbox.md §2.2) without any privilege on the host: a user-space
// TCP/IP stack (gVisor's) terminates the guest's TCP and UDP on the host
// side of its network card, judges each destination address, and connects
// from the host only to what the level reaches: the public internet, and the
// private addresses granted. The host itself (its loopback and every address
// of its interfaces) is never reached; DNS at netfilter.DNSAddr is forwarded
// to the host's resolver. The filter is outside the guest, so nothing the
// guest's kernel does gets around it.
//
// The guest's card is a stream of Ethernet frames: one per datagram of a
// connected datagram socket (Virtualization.framework's file handle
// attachment), or any Link. Only IPv4; no ICMP.
package usernet

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/link/ethernet"
	"gvisor.dev/gvisor/pkg/tcpip/network/arp"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
	"gvisor.dev/gvisor/pkg/waiter"

	"github.com/tingly-dev/tingly-box/sandbox"
	"github.com/tingly-dev/tingly-box/sandbox/netfilter"
)

// The guest's network: the guest init sets its card up with these.
var (
	Subnet  = netip.MustParsePrefix("10.0.2.0/24")
	Gateway = netip.MustParseAddr("10.0.2.1")
	Guest   = netip.PrefixFrom(netip.MustParseAddr("10.0.2.15"), 24)
)

// GatewayMAC is the host side's link address.
const GatewayMAC = "\x52\x54\x00\x12\x34\x01"

// MTU is the card's MTU.
const MTU = 1500

// Link is the guest's card: each Read returns one Ethernet frame, each Write
// sends one.
type Link interface {
	io.ReadWriteCloser
}

// Config is what the network gives.
type Config struct {
	// Grants are the private destinations granted, by address (sandbox.Grant
	// with a prefix); a grant by name grants nothing here.
	Grants []sandbox.Grant
	// Resolver is the host's resolver (an address, with or without a port),
	// where queries to netfilter.DNSAddr go.
	Resolver string
	// Dial connects from the host to an address the network has granted;
	// nil is a plain dialer. A variable for tests.
	Dial func(ctx context.Context, network, addr string) (net.Conn, error)
}

// Network is a guest's network, served until Close.
type Network struct {
	cfg    Config
	link   Link
	ep     *channel.Endpoint
	stack  *stack.Stack
	ctx    context.Context
	cancel context.CancelFunc
	once   sync.Once
	wg     sync.WaitGroup
}

const nicID = 1

// Serve serves the guest's network on link.
func Serve(link Link, cfg Config) (*Network, error) {
	if cfg.Dial == nil {
		d := &net.Dialer{Timeout: 30 * time.Second}
		cfg.Dial = d.DialContext
	}
	if cfg.Resolver != "" {
		if _, _, err := net.SplitHostPort(cfg.Resolver); err != nil {
			cfg.Resolver = net.JoinHostPort(cfg.Resolver, "53")
		}
	}
	s := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol, arp.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol},
		HandleLocal:        false,
	})
	ep := channel.New(512, MTU, tcpip.LinkAddress(GatewayMAC))
	if err := s.CreateNIC(nicID, ethernet.New(ep)); err != nil {
		return nil, fmt.Errorf("usernet: %v", err)
	}
	gw := tcpip.AddrFrom4(Gateway.As4())
	if err := s.AddProtocolAddress(nicID, tcpip.ProtocolAddress{Protocol: ipv4.ProtocolNumber,
		AddressWithPrefix: gw.WithPrefix()}, stack.AddressProperties{}); err != nil {
		return nil, fmt.Errorf("usernet: %v", err)
	}
	// Every destination is the host side's to judge.
	_ = s.SetPromiscuousMode(nicID, true)
	_ = s.SetSpoofing(nicID, true)
	sub, _ := tcpip.NewSubnet(tcpip.AddrFrom4([4]byte{}), tcpip.MaskFromBytes(make([]byte, 4)))
	s.SetRouteTable([]tcpip.Route{{Destination: sub, NIC: nicID}})

	ctx, cancel := context.WithCancel(context.Background())
	n := &Network{cfg: cfg, link: link, ep: ep, stack: s, ctx: ctx, cancel: cancel}
	s.SetTransportProtocolHandler(tcp.ProtocolNumber, tcp.NewForwarder(s, 0, 256, n.tcp).HandlePacket)
	s.SetTransportProtocolHandler(udp.ProtocolNumber, udp.NewForwarder(s, n.udp).HandlePacket)
	n.wg.Add(2)
	go n.rx()
	go n.tx()
	return n, nil
}

// Close stops the network and every connection through it.
func (n *Network) Close() error {
	n.once.Do(func() {
		n.cancel()
		_ = n.link.Close()
		n.ep.Close()
		n.stack.Close()
		n.wg.Wait()
	})
	return nil
}

// rx hands the guest's frames to the stack.
func (n *Network) rx() {
	defer n.wg.Done()
	buf := make([]byte, 64<<10)
	for {
		size, err := n.link.Read(buf)
		if err != nil {
			n.cancel()
			return
		}
		if size < header.EthernetMinimumSize {
			continue
		}
		pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(append([]byte(nil), buf[:size]...))})
		n.ep.InjectInbound(0, pkt)
		pkt.DecRef()
	}
}

// tx sends the stack's frames to the guest.
func (n *Network) tx() {
	defer n.wg.Done()
	for {
		pkt := n.ep.ReadContext(n.ctx)
		if pkt == nil {
			return
		}
		frame := pkt.ToView().AsSlice()
		_, err := n.link.Write(frame)
		pkt.DecRef()
		if err != nil {
			n.cancel()
			return
		}
	}
}

// Permits reports whether the network connects the guest to ip:port: a
// public address on any port; a private one when a grant by address grants
// it; never the host, nor the guest's own network.
func Permits(grants []sandbox.Grant, ip netip.Addr, port int) bool {
	ip = ip.Unmap()
	if Subnet.Contains(ip) {
		return false
	}
	switch sandbox.Classify(ip) {
	case sandbox.SurfacePublic:
		return true
	case sandbox.SurfacePrivate:
		for _, g := range grants {
			if g.Prefix.IsValid() && g.Permits("", ip, port) {
				return true
			}
		}
	}
	return false
}

// target is where a guest's connection to id goes on the host: the resolver
// for DNS, the destination itself when permitted, else nowhere.
func (n *Network) target(id stack.TransportEndpointID) (string, bool) {
	ip := netip.AddrFrom4(id.LocalAddress.As4())
	port := int(id.LocalPort)
	if ip == netfilter.DNSAddr {
		return n.cfg.Resolver, port == 53 && n.cfg.Resolver != ""
	}
	if !Permits(n.cfg.Grants, ip, port) {
		return "", false
	}
	return net.JoinHostPort(ip.String(), strconv.Itoa(port)), true
}

func (n *Network) tcp(r *tcp.ForwarderRequest) {
	to, ok := n.target(r.ID())
	if !ok {
		r.Complete(true) // refused: a reset
		return
	}
	up, err := n.cfg.Dial(n.ctx, "tcp", to)
	if err != nil {
		r.Complete(true)
		return
	}
	var wq waiter.Queue
	ep, terr := r.CreateEndpoint(&wq)
	r.Complete(false)
	if terr != nil {
		_ = up.Close()
		return
	}
	go splice(gonet.NewTCPConn(&wq, ep), up)
}

// udpIdle is how long a UDP flow lives without a datagram.
var udpIdle = 60 * time.Second

func (n *Network) udp(r *udp.ForwarderRequest) bool {
	id := r.ID()
	if ip := netip.AddrFrom4(id.LocalAddress.As4()); ip == netip.AddrFrom4([4]byte{255, 255, 255, 255}) {
		return true
	}
	to, ok := n.target(id)
	if !ok {
		return true // dropped
	}
	var wq waiter.Queue
	ep, terr := r.CreateEndpoint(&wq)
	if terr != nil {
		return true
	}
	guest := gonet.NewUDPConn(&wq, ep)
	up, err := n.cfg.Dial(n.ctx, "udp", to)
	if err != nil {
		_ = guest.Close()
		return true
	}
	go func() {
		defer guest.Close()
		defer up.Close()
		done := make(chan struct{}, 2)
		copyFlow := func(dst, src net.Conn) {
			buf := make([]byte, 64<<10)
			for {
				_ = src.SetReadDeadline(time.Now().Add(udpIdle))
				k, err := src.Read(buf)
				if err != nil {
					break
				}
				if _, err := dst.Write(buf[:k]); err != nil {
					break
				}
			}
			done <- struct{}{}
		}
		go copyFlow(up, guest)
		go copyFlow(guest, up)
		select {
		case <-done:
		case <-n.ctx.Done():
		}
	}()
	return true
}

// splice copies both ways until either side is done.
func splice(a, b net.Conn) {
	var wg sync.WaitGroup
	half := func(dst, src net.Conn) {
		defer wg.Done()
		_, _ = io.Copy(dst, src)
		if c, ok := dst.(interface{ CloseWrite() error }); ok {
			_ = c.CloseWrite()
		} else {
			_ = dst.Close()
		}
	}
	wg.Add(2)
	go half(a, b)
	go half(b, a)
	wg.Wait()
	_ = a.Close()
	_ = b.Close()
}
