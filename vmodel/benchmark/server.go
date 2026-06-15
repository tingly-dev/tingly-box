package benchmark

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/tingly-dev/tingly-box/vmodel/virtualserver"
)

// DefaultPort is the conventional benchmark server port. Callers may use
// LocalServer.Port() to discover an ephemeral port instead.
const DefaultPort = 12580

// LocalServer is the capture-free load target: a virtualserver.Service exposed
// over a real HTTP listener so the benchmark load client can hit a loopback
// target with no per-request observability overhead. It shares route wiring with
// the observable reference Server via productionRouter (see bench.go) but
// deliberately omits the capture middleware — for an observable server (request
// capture, endpoint-hit counts) use NewProductionServer().Listen() instead.
type LocalServer struct {
	svc      *virtualserver.Service
	listener net.Listener
	server   *http.Server
}

// NewLocalServer starts an in-process benchmark server bound to addr (an
// empty string or ":0" picks an ephemeral port). The returned server is
// already listening; call Port() to discover the bound port and Close() to
// shut down. The underlying virtualmodel registries come pre-populated with
// the same defaults as production via virtualserver.NewService.
func NewLocalServer(addr string) (*LocalServer, error) {
	router, svc := productionRouter()

	if addr == "" {
		addr = fmt.Sprintf(":%d", DefaultPort)
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("benchmark: listen %s: %w", addr, err)
	}

	srv := &http.Server{Handler: router}
	go func() {
		_ = srv.Serve(listener)
	}()

	return &LocalServer{svc: svc, listener: listener, server: srv}, nil
}

// Service returns the underlying virtualserver.Service so callers can
// register additional virtual models on its anthropic / openai registries.
func (s *LocalServer) Service() *virtualserver.Service { return s.svc }

// Port returns the TCP port the server is listening on.
func (s *LocalServer) Port() int { return s.listener.Addr().(*net.TCPAddr).Port }

// BaseURL returns http://localhost:<port> for use as BenchmarkOptions.BaseURL.
func (s *LocalServer) BaseURL() string {
	return fmt.Sprintf("http://localhost:%d", s.Port())
}

// Close shuts down the server with a short grace period.
func (s *LocalServer) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}
