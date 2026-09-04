package virtualserver

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync/atomic"
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
// is reachable only through Transport, which is the trust boundary (the same
// one the previous in-process function call had).
type Server struct {
	svc       *Service
	listener  *memListener
	http      *http.Server
	transport *http.Transport
}

// current is the Server the client layer dials for vmodel:// providers — the
// most recent Serve call. Cleared again by that Server's Close.
var current atomic.Pointer[Server]

var errNotServing = errors.New("vmodel: virtual-model server is not running in this process")

// Serve starts an HTTP server for svc on a fresh in-memory listener and makes
// it the target of Transport. It returns immediately; the server runs until
// Close.
func Serve(svc *Service) *Server {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	svc.SetupOpenAIRoutes(router.Group("/openai"))
	svc.SetupAnthropicRoutes(router.Group("/anthropic"))

	ln := newMemListener()
	s := &Server{
		svc:      svc,
		listener: ln,
		http:     &http.Server{Handler: router},
	}
	s.transport = &http.Transport{
		DialContext:         ln.Dial,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
	}
	go func() { _ = s.http.Serve(ln) }()
	current.Store(s)
	return s
}

// Service returns the served Service so callers can register models.
func (s *Server) Service() *Service { return s.svc }

// DialContext connects to this server; network and addr are ignored.
func (s *Server) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return s.listener.Dial(ctx, network, addr)
}

// Close stops accepting connections and shuts the server down with a short
// grace period. If this server is the current Transport target, Transport
// falls back to failing fast.
func (s *Server) Close() error {
	current.CompareAndSwap(s, nil)
	s.transport.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return s.http.Shutdown(ctx)
}

// Transport returns the RoundTripper the client layer uses as the base of a
// vmodel provider's transport chain. It dials the current Server; with no
// Server running every request fails with a clear error instead of leaking a
// DNS lookup for the placeholder host.
func Transport() http.RoundTripper {
	if s := current.Load(); s != nil {
		return s.transport
	}
	return notServingTransport{}
}

type notServingTransport struct{}

func (notServingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, &net.OpError{Op: "dial", Net: Scheme, Err: errNotServing}
}
