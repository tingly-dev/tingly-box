package benchmark

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/vmodel"
	"github.com/tingly-dev/tingly-box/vmodel/benchmark/scenario"
	"github.com/tingly-dev/tingly-box/vmodel/virtualserver"
)

// Server is the observable reference mock-provider at the heart of the vmodel
// benchmark. It wraps any inner provider http.Handler with a capture middleware
// that records request counts, per-endpoint hits, and the last forwarded request
// — observability that is independent of how responses are generated.
//
// Response generation is pluggable via the constructor used:
//
//   - NewModelServer() serves real vmodel models (protocol-correct bytes),
//     the same registries that back the production /virtual/v1/* endpoint.
//   - NewScenarioServer() serves registered scenario fixtures across all four
//     provider formats (OpenAI chat / responses, Anthropic, Google).
//   - NewServer(inner) wraps an arbitrary provider handler.
//
// Transport is also pluggable on the same instance: InProcess() starts an
// httptest server (in-process), Listen() binds a real TCP port (for subprocess
// or external clients). The type carries no *testing.T dependency, so it can be
// imported by external Go projects as well as test packages.
type Server struct {
	handler   http.Handler
	rec       *recorder
	scenarios *vmodel.GenericRegistry[scenario.Scenario] // non-nil only for scenario servers

	ts      *httptest.Server
	httpSrv *http.Server
	ln      net.Listener
	url     string
}

// NewServer wraps an arbitrary provider handler with the capture middleware.
func NewServer(inner http.Handler) *Server {
	rec := newRecorder()
	return &Server{handler: captureMiddleware(rec, inner), rec: rec}
}

// ServeHTTP lets a Server be used directly as an http.Handler while preserving
// the same capture and endpoint accounting as its managed transports.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

// NewModelServer builds a Server whose inner handler is the production
// virtualserver.Service mounted under /v1, /openai/v1, and /anthropic/v1 — the
// same wiring (and default model registries) as the production endpoint, so the
// responses are wire-format-correct. Use this for servertest, load tests, and
// external projects that want a realistic provider.
func NewModelServer() *Server {
	router, _ := modelRouter()
	return NewServer(router)
}

// modelRouter builds a gin engine serving the production
// virtualserver.Service (the same default vmodel registries as /virtual/v1/*)
// under /v1, /openai/v1, and /anthropic/v1, and returns the engine plus the
// service. Shared by NewModelServer (which wraps it with capture) and
// LocalServer (the capture-free load target), so the route wiring lives in
// exactly one place. A server that needs *custom* models builds its own router
// over a registered virtualserver.Service and wraps it with NewServer — no
// dedicated accessor needed.
func modelRouter() (*gin.Engine, *virtualserver.Service) {
	svc := virtualserver.NewService()

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	for _, prefix := range []string{"/v1", "/openai/v1", "/anthropic/v1"} {
		svc.SetupRoutes(router.Group(prefix))
	}
	return router, svc
}

// NewScenarioServer builds a Server whose inner handler serves registered
// scenario fixtures (scenario.MockResponseBuilder) across all four provider
// formats. Register scenarios with RegisterScenario. Use this for the protocol
// transform matrix and for byte-exact mocks where you control the exact response.
func NewScenarioServer() *Server {
	reg := vmodel.NewGenericRegistry[scenario.Scenario]()
	s := NewServer(newScenarioResponder(reg))
	s.scenarios = reg
	return s
}

// RegisterScenario registers (or replaces) a scenario on a scenario server. It
// is a no-op on a model server. The registry ordinarily errors on duplicate
// IDs, so a prior entry with the same name is cleared first.
func (s *Server) RegisterScenario(sc scenario.Scenario) {
	if s.scenarios == nil {
		return
	}
	s.scenarios.Unregister(sc.Name)
	_ = s.scenarios.Register(sc)
}

// InProcess starts the server on an in-process httptest listener and returns its
// base URL. Prefer this for in-process Go tests.
func (s *Server) InProcess() string {
	s.ts = httptest.NewServer(s.handler)
	s.url = s.ts.URL
	return s.url
}

// Listen binds a real TCP listener (addr "" or ":0" picks an ephemeral port) and
// returns the base URL. Prefer this when a subprocess or external client must
// reach the server over loopback.
func (s *Server) Listen(addr string) (string, error) {
	if addr == "" {
		addr = ":0"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", fmt.Errorf("benchmark: listen %s: %w", addr, err)
	}
	s.ln = ln
	s.httpSrv = &http.Server{Handler: s.handler}
	go func() { _ = s.httpSrv.Serve(ln) }()
	s.url = fmt.Sprintf("http://localhost:%d", ln.Addr().(*net.TCPAddr).Port)
	return s.url, nil
}

// URL returns the base URL the server is reachable at (empty until InProcess or
// Listen has been called).
func (s *Server) URL() string { return s.url }

// Port returns the TCP port for a Listen()-started server, or 0 otherwise.
func (s *Server) Port() int {
	if s.ln == nil {
		return 0
	}
	return s.ln.Addr().(*net.TCPAddr).Port
}

// Close shuts the server down with a short grace period.
func (s *Server) Close() error {
	if s.ts != nil {
		s.ts.Close()
	}
	if s.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return s.httpSrv.Shutdown(ctx)
	}
	return nil
}

// CallCount returns the total number of requests received.
func (s *Server) CallCount() int { return s.rec.totalCalls() }

// EndpointHits returns how many requests hit a specific provider endpoint.
func (s *Server) EndpointHits(kind EndpointKind) int { return s.rec.hits(kind) }

// PathHits returns how many requests were captured for one exact URL path.
func (s *Server) PathHits(path string) int { return s.rec.hitsForPath(path) }

// LastRequest returns the most recent request forwarded to the given provider
// endpoint, or nil if that endpoint was never hit.
func (s *Server) LastRequest(kind EndpointKind) *CapturedRequest { return s.rec.lastRequest(kind) }

// LastRequestForPath returns the most recent request captured for one exact URL
// path, or nil if that path was never hit.
func (s *Server) LastRequestForPath(path string) *CapturedRequest {
	return s.rec.lastRequestForPath(path)
}

// Reset clears all recorded counts and captured requests.
func (s *Server) Reset() { s.rec.reset() }

// captureMiddleware tees the request body, records the hit, restores the body
// for the inner handler, then delegates.
func captureMiddleware(rec *recorder, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		rec.record(classify(r.URL.Path), r, body)
		next.ServeHTTP(w, r)
	})
}
