package virtualserver

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Server is the virtualserver exposed as a real HTTP upstream for vmodel
// providers. It serves
//
//	/openai/v1/{models,chat/completions,responses}
//	/anthropic/v1/{models,messages}
//
// on a private in-memory listener. There is no auth middleware: the listener
// is reachable only through DialContext, which is the trust boundary (the same
// one the previous in-process function call had). Clients reach it through
// vmodelclient.NewTransport(srv.DialContext).
type Server struct {
	listener *memListener
	http     *http.Server
}

// Serve starts an HTTP server for svc on a fresh in-memory listener. It
// returns immediately; the server runs until Close.
func Serve(svc *Service) *Server {
	// gin mode is process-global and owned by the host; do not touch it here.
	router := gin.New()
	router.Use(gin.Recovery())
	svc.SetupOpenAIRoutes(router.Group("/openai"))
	svc.SetupAnthropicRoutes(router.Group("/anthropic"))
	// Anything the SDKs can reach but the virtual models do not simulate
	// (embeddings, images, count_tokens, ...) gets a protocol-shaped 501
	// instead of a bare router 404.
	router.NoRoute(svc.handler.NotSupported)

	ln := newMemListener()
	s := &Server{listener: ln, http: &http.Server{Handler: router}}
	go func() { _ = s.http.Serve(ln) }()
	return s
}

// DialContext connects to this server; network and addr are ignored.
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
