package virtualserver

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Server is the virtualserver exposed as a real HTTP upstream for vmodel
// providers. It serves the per-protocol routes
//
//	/openai/v1/{models,chat/completions,responses}
//	/anthropic/v1/{models,messages}
//
// on a private in-memory listener. There is no auth middleware: the listener
// is reachable only through DialContext, which is the trust boundary (the same
// one the previous in-process function call had). Callers wire DialContext
// into the transport used for providers whose APIBase carries the vmodel://
// scheme (ai.VModelScheme).
type Server struct {
	svc      *Service
	listener *memListener
	http     *http.Server
}

// Serve starts an HTTP server for svc on a fresh in-memory listener. It
// returns immediately; the server runs until Close.
func Serve(svc *Service) *Server {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	svc.SetupOpenAIRoutes(router.Group("/openai"))
	svc.SetupAnthropicRoutes(router.Group("/anthropic"))

	ln := newMemListener()
	srv := &http.Server{Handler: router}
	go func() { _ = srv.Serve(ln) }()

	return &Server{svc: svc, listener: ln, http: srv}
}

// Service returns the served Service so callers can register models.
func (s *Server) Service() *Service { return s.svc }

// DialContext connects to the server. Plug it into http.Transport.DialContext;
// the network and address arguments are ignored.
func (s *Server) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return s.listener.Dial(ctx, network, addr)
}

// Close stops accepting connections and shuts the server down with a short
// grace period.
func (s *Server) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return s.http.Shutdown(ctx)
}
