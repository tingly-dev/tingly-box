//go:build unix

package netproxy

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// A process started through Wrap is the bridge: it runs before anything else
// of this binary would.
func init() {
	if dir, ok := os.LookupEnv(BridgeEnv); ok {
		os.Exit(bridge(dir, os.Getenv(PlanEnv), os.Args[1:]))
	}
}

// bridge offers the endpoints in dir, runs args with the environment that
// points at them, and exits as it does.
func bridge(dir, plan string, args []string) int {
	_ = os.Unsetenv(BridgeEnv)
	_ = os.Unsetenv(PlanEnv)
	fail := func(err error) int {
		fmt.Fprintf(os.Stderr, "sandbox network bridge: %v\n", err)
		return 127
	}
	if len(args) == 0 {
		return fail(errors.New("no command"))
	}
	pl, err := parsePlan(plan)
	if err != nil {
		return fail(err)
	}
	dial := func(name string) (io.ReadWriteCloser, error) {
		return net.Dial("unix", filepath.Join(dir, name+".sock"))
	}
	env, err := ServeBridge(pl, dial, os.Environ())
	if err != nil {
		return fail(err)
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return fail(err)
	}
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP, syscall.SIGQUIT)
	go func() {
		for s := range sigs {
			_ = cmd.Process.Signal(s)
		}
	}()
	err = cmd.Wait()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &exit):
		if ws, ok := exit.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal())
		}
		return exit.ExitCode()
	default:
		return 127
	}
}

// ServeBridge offers pl's endpoints, which dial reaches by name, on the
// sandbox's loopback for as long as this process lives: each exposed port at
// localhost on the same port, the proxy on a free one. It returns env with the
// proxy variables pointing there when pl has the proxy.
func ServeBridge(pl Plan, dial func(name string) (io.ReadWriteCloser, error), env []string) ([]string, error) {
	for _, port := range pl.Expose {
		ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err != nil {
			return nil, fmt.Errorf("expose %d: %w", port, err)
		}
		forward(ln, ExposeName(port), dial)
		// localhost may resolve to ::1 first.
		if ln6, err := net.Listen("tcp", net.JoinHostPort("::1", strconv.Itoa(port))); err == nil {
			forward(ln6, ExposeName(port), dial)
		}
	}
	if !pl.Proxy {
		return env, nil
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	forward(ln, ProxyName, dial)
	return proxyEnv(env, "http://"+ln.Addr().String()), nil
}

// forward splices every connection on ln to the endpoint name.
func forward(ln net.Listener, name string, dial func(string) (io.ReadWriteCloser, error)) {
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				up, err := dial(name)
				if err != nil {
					_ = c.Close()
					return
				}
				splice(c, c, up)
			}()
		}
	}()
}

// proxyEnv is env with every proxy variable replaced by url, and loopback
// left direct (the exposed endpoints are there).
func proxyEnv(env []string, url string) []string {
	out := make([]string, 0, len(env)+6)
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(name) {
		case "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY":
			continue
		}
		out = append(out, kv)
	}
	for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
		out = append(out, name+"="+url)
	}
	return append(out, "NO_PROXY=localhost,127.0.0.1,::1", "no_proxy=localhost,127.0.0.1,::1")
}
