// Package netproxy gives a runtime the allowlist network level: a proxy on the
// host that connects only to the hosts a policy allows, served on a unix
// socket, and a bridge inside the sandbox that offers it on loopback and runs
// the process with the proxy variables pointing at it. A runtime that has no
// network of its own (an unshared network namespace, a VM without a NIC)
// lets the sandbox see the socket and starts its command through Wrap.
//
// The bridge is this same binary: Wrap puts the executable in front of the
// command, and init turns the process into the bridge when it finds
// BridgeEnv. A runtime that uses Wrap must let the sandbox see Executable().
package netproxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/sandbox"
)

// SocketName is the proxy's socket in the directory Listen makes.
const SocketName = "proxy.sock"

// Allowed reports whether host:port is one of allow (see sandbox.Network).
func Allowed(allow []string, host string, port int) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, a := range allow {
		h, p := a, ""
		if hh, pp, err := net.SplitHostPort(a); err == nil {
			h, p = hh, pp
		}
		h = strings.ToLower(strings.TrimSuffix(h, "."))
		if p == "" {
			if port != 80 && port != 443 {
				continue
			}
		} else if p != strconv.Itoa(port) {
			continue
		}
		if suffix, ok := strings.CutPrefix(h, "*."); ok {
			if strings.HasSuffix(host, "."+suffix) {
				return true
			}
		} else if host == h {
			return true
		}
	}
	return false
}

// Proxy is an HTTP proxy (CONNECT and plain requests) that reaches only the
// allowed hosts, listening on a unix socket.
type Proxy struct {
	allow []string
	dir   string
	ln    net.Listener
	srv   *http.Server
	fwd   *httputil.ReverseProxy
	once  sync.Once
}

var dialer = &net.Dialer{Timeout: 30 * time.Second}

// Listen starts a proxy for allow on a socket in a new private directory (Dir).
func Listen(allow []string) (*Proxy, error) {
	dir, err := os.MkdirTemp(socketRoot(), "tb-net-")
	if err != nil {
		return nil, fmt.Errorf("proxy socket directory: %w", err)
	}
	ln, err := net.Listen("unix", filepath.Join(dir, SocketName))
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("proxy socket: %w", err)
	}
	p := &Proxy{allow: append([]string(nil), allow...), dir: dir, ln: ln}
	tr := &http.Transport{Proxy: nil, DialContext: dialer.DialContext, ForceAttemptHTTP2: true}
	p.fwd = &httputil.ReverseProxy{Rewrite: func(*httputil.ProxyRequest) {}, Transport: tr}
	p.srv = &http.Server{Handler: p, ReadHeaderTimeout: 30 * time.Second}
	go func() { _ = p.srv.Serve(ln) }()
	return p, nil
}

// socketRoot is where the socket's directory goes: the user's runtime
// directory, else the temporary one, else /tmp, whichever keeps the socket's
// path within the length a unix socket allows.
func socketRoot() string {
	for _, d := range []string{os.Getenv("XDG_RUNTIME_DIR"), os.TempDir(), "/tmp"} {
		if d != "" && len(d)+len("/tb-net-0123456789/"+SocketName) < 100 {
			if fi, err := os.Stat(d); err == nil && fi.IsDir() {
				return d
			}
		}
	}
	return os.TempDir()
}

// Dir is the directory holding the socket, for the runtime to let the
// sandbox see.
func (p *Proxy) Dir() string { return p.dir }

// Close stops the proxy, ends its tunnels and removes its directory.
func (p *Proxy) Close() error {
	p.once.Do(func() {
		_ = p.srv.Close()
		_ = os.RemoveAll(p.dir)
	})
	return nil
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host, port, ok := target(r)
	if !ok {
		http.Error(w, "sandbox proxy: not a proxy request", http.StatusBadRequest)
		return
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	if !Allowed(p.allow, host, port) {
		http.Error(w, "sandbox: "+addr+" is not in the network allowlist", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodConnect {
		p.fwd.ServeHTTP(w, r)
		return
	}
	up, err := dialer.DialContext(r.Context(), "tcp", addr)
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
func splice(a net.Conn, aIn io.Reader, b net.Conn) {
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(b, aIn); done <- struct{}{} }()
	go func() { _, _ = io.Copy(a, b); done <- struct{}{} }()
	<-done
	_ = a.Close()
	_ = b.Close()
	<-done
}

// BridgeEnv names the socket, as the sandbox sees it, to the bridge.
const BridgeEnv = "TB_SANDBOX_NET_BRIDGE"

// Executable is this binary, which Wrap runs as the bridge; the sandbox must
// see it at this path.
func Executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// Wrap returns cmd started through the bridge, which reaches the proxy at
// sock (the socket's path inside the sandbox).
func Wrap(cmd sandbox.Cmd, sock string) (sandbox.Cmd, error) {
	exe, err := Executable()
	if err != nil {
		return cmd, fmt.Errorf("%w: the network bridge needs this binary: %v", sandbox.ErrUnavailable, err)
	}
	cmd.Args = append([]string{exe}, cmd.Args...)
	cmd.Env = append(append([]string(nil), cmd.Env...), BridgeEnv+"="+sock)
	return cmd, nil
}

// Serve closes p once proc has exited, and returns proc.
func Serve(p *Proxy, proc sandbox.Proc) sandbox.Proc {
	go func() {
		<-proc.Done()
		_ = p.Close()
	}()
	return proc
}
