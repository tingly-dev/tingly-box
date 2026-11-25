// Package netproxy is the network plumbing a runtime uses when the sandbox
// has no network of its own to the host (.design/sandbox.md §2.2):
//
//   - on the host, a Host serves the policy's endpoints on unix sockets: the
//     allowlist proxy (HTTP CONNECT and plain requests, granting only what
//     Network.Allow grants), and one forwarder per exposed port to the
//     host's loopback;
//   - in the sandbox, a bridge offers them on the sandbox's loopback: each
//     exposed port at localhost on the same port, and the proxy through
//     HTTP(S)_PROXY, then runs the command.
//
// The runtime connects the two: bwrap lets the sandbox see the socket
// directory, a microVM forwards each socket over vsock. The bridge is this
// same binary: Wrap puts the executable in front of the command, and init
// turns the process into the bridge when it finds BridgeEnv. A runtime that
// uses Wrap must let the sandbox see Executable().
package netproxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/sandbox"
)

// Plan is what the bridge offers inside: the proxy, and the exposed ports.
type Plan struct {
	Proxy  bool
	Expose []int
}

// PlanFor is the plan for a network policy.
func PlanFor(n sandbox.Network) Plan {
	pl := Plan{Proxy: n.Level == sandbox.NetworkAllowlist}
	for _, e := range n.Expose {
		pl.Expose = append(pl.Expose, e.Port)
	}
	return pl
}

// Empty reports whether there is nothing to bridge.
func (pl Plan) Empty() bool { return !pl.Proxy && len(pl.Expose) == 0 }

// ProxyName and ExposeName name the endpoints, for the runtime to map to
// socket paths or vsock ports.
const ProxyName = "proxy"

func ExposeName(port int) string { return "expose-" + strconv.Itoa(port) }

// Names are the plan's endpoints.
func (pl Plan) Names() []string {
	var out []string
	if pl.Proxy {
		out = append(out, ProxyName)
	}
	for _, p := range pl.Expose {
		out = append(out, ExposeName(p))
	}
	return out
}

// Host serves a network policy's endpoints on the host.
type Host struct {
	dir     string
	closers []io.Closer
	once    sync.Once
}

// ListenDir serves n's endpoints as <name>.sock in a new private directory
// (Dir), for a runtime that lets the sandbox see it.
func ListenDir(n sandbox.Network) (*Host, error) {
	dir, err := os.MkdirTemp(socketRoot(), "tb-net-")
	if err != nil {
		return nil, fmt.Errorf("socket directory: %w", err)
	}
	h, err := Listen(n, func(name string) string { return filepath.Join(dir, name+".sock") })
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	h.dir = dir
	return h, nil
}

// Listen serves n's endpoints on the unix socket path(name) of each.
func Listen(n sandbox.Network, path func(name string) string) (*Host, error) {
	h := &Host{}
	pl := PlanFor(n)
	if pl.Proxy {
		p, err := listenProxy(n.Allow, path(ProxyName))
		if err != nil {
			return nil, err
		}
		h.closers = append(h.closers, p)
	}
	for _, port := range pl.Expose {
		f, err := listenForward(path(ExposeName(port)), port)
		if err != nil {
			_ = h.Close()
			return nil, err
		}
		h.closers = append(h.closers, f)
	}
	return h, nil
}

// Dir is the directory ListenDir made.
func (h *Host) Dir() string { return h.dir }

// Close stops serving and removes the directory ListenDir made.
func (h *Host) Close() error {
	h.once.Do(func() {
		for _, c := range h.closers {
			_ = c.Close()
		}
		if h.dir != "" {
			_ = os.RemoveAll(h.dir)
		}
	})
	return nil
}

// Serve closes h once proc has exited, and returns proc.
func Serve(h *Host, proc sandbox.Proc) sandbox.Proc {
	go func() {
		<-proc.Done()
		_ = h.Close()
	}()
	return proc
}

// socketRoot is where the socket directory goes: the user's runtime
// directory, else the temporary one, else /tmp, whichever keeps socket paths
// within the length a unix socket allows.
func socketRoot() string {
	for _, d := range []string{os.Getenv("XDG_RUNTIME_DIR"), os.TempDir(), "/tmp"} {
		if d != "" && len(d)+len("/tb-net-0123456789/expose-65535.sock") < 100 {
			if fi, err := os.Stat(d); err == nil && fi.IsDir() {
				return d
			}
		}
	}
	return os.TempDir()
}

var dialer = &net.Dialer{Timeout: 30 * time.Second}

// dialOut connects the proxy to an address it has granted; a variable for
// tests.
var dialOut = dialer.DialContext

// listenForward forwards every connection on the socket at path to the
// host's loopback on port.
func listenForward(path string, port int) (io.Closer, error) {
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("expose %d: %w", port, err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				up, err := dialer.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
				if err != nil {
					up, err = dialer.Dial("tcp", net.JoinHostPort("::1", strconv.Itoa(port)))
				}
				if err != nil {
					_ = c.Close()
					return
				}
				splice(c, c, up)
			}()
		}
	}()
	return ln, nil
}

// lookup resolves a name when a request is made; a variable for tests.
var lookup = func(ctx context.Context, name string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip", name)
}

// proxy is an HTTP proxy (CONNECT and plain requests) that connects only to
// what its grants grant, judged on the address it is about to connect to.
type proxy struct {
	grants []sandbox.Grant
	srv    *http.Server
	fwd    *httputil.ReverseProxy
}

type dialTo struct{}

func listenProxy(allow []string, path string) (*proxy, error) {
	p := &proxy{}
	for _, a := range allow {
		g, err := sandbox.ParseGrant(a)
		if err != nil {
			return nil, err
		}
		p.grants = append(p.grants, g)
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("proxy socket: %w", err)
	}
	// A plain request connects to the address the grant was checked on,
	// never to a second resolution of the name.
	tr := &http.Transport{Proxy: nil, ForceAttemptHTTP2: true,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if to, ok := ctx.Value(dialTo{}).(string); ok {
				addr = to
			}
			return dialOut(ctx, network, addr)
		}}
	p.fwd = &httputil.ReverseProxy{Rewrite: func(*httputil.ProxyRequest) {}, Transport: tr}
	p.srv = &http.Server{Handler: p, ReadHeaderTimeout: 30 * time.Second}
	go func() { _ = p.srv.Serve(ln) }()
	return p, nil
}

func (p *proxy) Close() error { return p.srv.Close() }

// resolve is the address to connect to for host:port, or why there is none.
func (p *proxy) resolve(ctx context.Context, host string, port int) (string, error) {
	name, addrs := host, []netip.Addr(nil)
	if ip, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		name, addrs = "", []netip.Addr{ip}
	} else {
		// A name no grant could take is refused without looking it up.
		if !slices.ContainsFunc(p.grants, func(g sandbox.Grant) bool { return g.MayName(host) }) {
			return "", fmt.Errorf("%s:%d is not in the network allowlist", host, port)
		}
		var err error
		if addrs, err = lookup(ctx, host); err != nil {
			return "", fmt.Errorf("%s does not resolve: %v", host, err)
		}
	}
	for _, ip := range addrs {
		for _, g := range p.grants {
			if g.Permits(name, ip, port) {
				return net.JoinHostPort(ip.Unmap().String(), strconv.Itoa(port)), nil
			}
		}
	}
	for _, ip := range addrs {
		if sandbox.Classify(ip) == sandbox.SurfaceHost {
			return "", fmt.Errorf("%s:%d is the host itself, which the allowlist never grants", host, port)
		}
	}
	return "", fmt.Errorf("%s:%d is not in the network allowlist", host, port)
}

func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host, port, ok := target(r)
	if !ok {
		http.Error(w, "sandbox proxy: not a proxy request", http.StatusBadRequest)
		return
	}
	addr, err := p.resolve(r.Context(), host, port)
	if err != nil {
		http.Error(w, "sandbox: "+err.Error(), http.StatusForbidden)
		return
	}
	if r.Method != http.MethodConnect {
		p.fwd.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), dialTo{}, addr)))
		return
	}
	up, err := dialOut(r.Context(), "tcp", addr)
	if err != nil {
		http.Error(w, "sandbox proxy: "+err.Error(), http.StatusBadGateway)
		return
	}
	conn, rw, err := http.NewResponseController(w).Hijack()
	if err != nil {
		_ = up.Close()
		return
	}
	if _, err := conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		_ = up.Close()
		_ = conn.Close()
		return
	}
	splice(conn, rw.Reader, up)
}

// target is the host and port r asks for: CONNECT's authority, or a plain
// request's absolute URL.
func target(r *http.Request) (string, int, bool) {
	hostport, scheme := r.Host, "https"
	if r.Method != http.MethodConnect {
		if !r.URL.IsAbs() {
			return "", 0, false
		}
		hostport, scheme = r.URL.Host, r.URL.Scheme
	}
	host, ps, err := net.SplitHostPort(hostport)
	if err != nil {
		host, ps = hostport, map[string]string{"http": "80", "https": "443"}[scheme]
	}
	port, err := strconv.Atoi(ps)
	if host == "" || err != nil {
		return "", 0, false
	}
	return host, port, true
}

// splice copies both ways until either side ends, then closes both. a's
// bytes are read from aIn (a itself, or a reader with some of them buffered).
func splice(a net.Conn, aIn io.Reader, b io.ReadWriteCloser) {
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(b, aIn); done <- struct{}{} }()
	go func() { _, _ = io.Copy(a, b); done <- struct{}{} }()
	<-done
	_ = a.Close()
	_ = b.Close()
	<-done
}

// BridgeEnv names, to the bridge, the directory where the sandbox sees the
// sockets; PlanEnv names the endpoints (Plan.Names) it offers.
const (
	BridgeEnv = "TB_SANDBOX_NET_BRIDGE"
	PlanEnv   = "TB_SANDBOX_NET_PLAN"
)

// Executable is this binary, which Wrap runs as the bridge; the sandbox must
// see it at this path.
func Executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// Wrap returns cmd started through the bridge, which finds the sockets of pl
// in dir (as the sandbox sees it).
func Wrap(cmd sandbox.Cmd, dir string, pl Plan) (sandbox.Cmd, error) {
	exe, err := Executable()
	if err != nil {
		return cmd, fmt.Errorf("%w: the network bridge needs this binary: %v", sandbox.ErrUnavailable, err)
	}
	cmd.Args = append([]string{exe}, cmd.Args...)
	cmd.Env = append(slices.Clone(cmd.Env), BridgeEnv+"="+dir, PlanEnv+"="+strings.Join(pl.Names(), ","))
	return cmd, nil
}

// parsePlan reads Plan.Names back.
func parsePlan(s string) (Plan, error) {
	var pl Plan
	for _, name := range strings.Split(s, ",") {
		switch {
		case name == "":
		case name == ProxyName:
			pl.Proxy = true
		case strings.HasPrefix(name, "expose-"):
			port, err := strconv.Atoi(strings.TrimPrefix(name, "expose-"))
			if err != nil {
				return pl, fmt.Errorf("bad endpoint %q", name)
			}
			pl.Expose = append(pl.Expose, port)
		default:
			return pl, errors.New("bad endpoint " + strconv.Quote(name))
		}
	}
	return pl, nil
}
