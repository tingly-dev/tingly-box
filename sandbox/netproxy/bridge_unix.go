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
	"strings"
	"syscall"
)

// A process started through Wrap is the bridge: it runs before anything else
// of this binary would.
func init() {
	if sock, ok := os.LookupEnv(BridgeEnv); ok {
		os.Exit(bridge(sock, os.Args[1:]))
	}
}

// bridge serves the proxy at sock on a loopback port, runs args with the
// proxy variables pointing there, and exits as it does.
func bridge(sock string, args []string) int {
	_ = os.Unsetenv(BridgeEnv)
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "sandbox network bridge: no command")
		return 127
	}
	env, err := Bridge(func() (io.ReadWriteCloser, error) { return net.Dial("unix", sock) }, os.Environ())
	if err != nil {
		fmt.Fprintf(os.Stderr, "sandbox network bridge: %v\n", err)
		return 127
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "sandbox network bridge: %v\n", err)
		return 127
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

// Bridge offers the proxy that dial reaches on a loopback port, for as long as
// this process lives, and returns env with the proxy variables pointing there.
func Bridge(dial func() (io.ReadWriteCloser, error), env []string) ([]string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				up, err := dial()
				if err != nil {
					_ = c.Close()
					return
				}
				splice(c, c, up)
			}()
		}
	}()
	return proxyEnv(env, "http://"+ln.Addr().String()), nil
}

// proxyEnv is env with every proxy variable replaced by url, and loopback
// left direct.
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
