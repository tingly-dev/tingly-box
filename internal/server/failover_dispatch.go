// Package server — failover_dispatch.go
//
// Mid-request failover for the priority routing tactic, built as a
// layered hand-off rather than a smart shim fused into the main path:
//
//   - Producer (the protocol handler) emits chunks normally, unaware of
//     failover. On its first real stream chunk it raises one signal,
//     CommitFirstChunk, and otherwise does nothing differently.
//   - firstChunkGate is a passive, protocol-agnostic byte buffer. It
//     makes no decisions: it records writes until CommitFirstChunk (or
//     the orchestrator's terminal flush) lets them through.
//   - Orchestrator (dispatchWithPriorityFailover) owns the retry
//     decision. It reads the gate's committed/status state after each
//     attempt and either commits, discards + retries the next tier, or
//     flushes the buffered terminal error.
//
// Single-service requests skip the gate entirely, so the common case
// never touches the buffer.

package server

import (
	"bytes"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// preStreamErrorRecorder narrows the recorder contract to what
// pre-stream failure sites need — both *ProtocolRecorder and
// *streamRecorder fit.
type preStreamErrorRecorder interface {
	RecordError(error)
}

// handlePreStreamFailure emits the canonical "upstream failed
// pre-stream" response: status 500 + JSON error, captured by the
// buffered failover writer so the orchestrator can retry the next
// priority tier.
func (s *Server) handlePreStreamFailure(c *gin.Context, err error, recorder preStreamErrorRecorder) {
	s.trackUsageFromContext(c, 0, 0, err)
	stream.SendStreamingError(c, err)
	if recorder != nil {
		recorder.RecordError(err)
	}
}

// retryableUpstreamStatuses are the HTTP status codes treated as
// "upstream transiently sick, try the next priority tier".
//
// 500 is included because in-process error helpers (SendStreamingError,
// SendErrorResponse on forwarding failure) wrap upstream pre-stream
// errors as 500. 502 covers the explicit "upstream stream failed" path.
// Keeping both means refactors that change one helper's status code
// don't silently break failover.
var retryableUpstreamStatuses = map[int]bool{
	http.StatusTooManyRequests:     true,
	http.StatusBadGateway:          true,
	http.StatusServiceUnavailable:  true,
	http.StatusGatewayTimeout:      true,
	http.StatusInternalServerError: true,
}

// isRetryableStatus reports whether a buffered status code from a
// dispatch attempt should trigger failover. Status 0 means the writer
// was never touched — treat as terminal (the handler ran to completion
// without writing, retrying would just repeat the no-op).
func isRetryableStatus(status int) bool {
	return status != 0 && retryableUpstreamStatuses[status]
}

// firstChunkGate is a passive, protocol-agnostic byte buffer placed
// between a dispatch attempt and the real response writer. It makes no
// decisions of its own: it records writes into a buffer until an
// explicit signal commits them. The retry decision lives in the
// orchestrator; the "first real chunk arrived" signal comes from the
// streaming producer via CommitFirstChunk.
//
// Lifecycle:
//   - buffered  — Write/WriteHeader capture into buf/hdr/status.
//   - committed — after CommitFirstChunk or CommitIfBuffered, all I/O
//     passes straight through to the real writer.
//   - discarded — Discard resets the buffer for the next retry tier
//     (valid only while uncommitted).
type firstChunkGate struct {
	gin.ResponseWriter
	real      gin.ResponseWriter
	buf       bytes.Buffer
	hdr       http.Header
	status    int
	committed bool
}

func newFirstChunkGate(w gin.ResponseWriter) *firstChunkGate {
	return &firstChunkGate{
		ResponseWriter: w,
		real:           w,
		hdr:            http.Header{},
	}
}

func (g *firstChunkGate) Header() http.Header {
	if g.committed {
		return g.real.Header()
	}
	return g.hdr
}

func (g *firstChunkGate) Write(p []byte) (int, error) {
	if g.committed {
		return g.real.Write(p)
	}
	if g.status == 0 {
		g.status = http.StatusOK
	}
	return g.buf.Write(p)
}

func (g *firstChunkGate) WriteString(s string) (int, error) {
	return g.Write([]byte(s))
}

func (g *firstChunkGate) WriteHeader(code int) {
	if g.committed {
		g.real.WriteHeader(code)
		return
	}
	g.status = code
}

// WriteHeaderNow is swallowed while buffered, delegated once committed.
func (g *firstChunkGate) WriteHeaderNow() {
	if g.committed {
		g.real.WriteHeaderNow()
	}
}

// Flush is a no-op while buffered (so partial SSE can't leak past the
// gate before commit) and delegates once committed (so streaming clients
// keep getting incremental delivery). ProcessStream type-asserts the
// writer for http.Flusher, so this method must exist concretely.
func (g *firstChunkGate) Flush() {
	if !g.committed {
		return
	}
	if f, ok := g.real.(http.Flusher); ok {
		f.Flush()
	}
}

// Status returns the captured status: 0 while untouched, the recorded
// code otherwise (Write defaults it to 200). Once committed it reflects
// the real writer. The orchestrator treats 0 as terminal (non-retryable).
func (g *firstChunkGate) Status() int {
	if g.committed {
		return g.real.Status()
	}
	return g.status
}

func (g *firstChunkGate) Size() int {
	if g.committed {
		return g.real.Size()
	}
	return g.buf.Len()
}

func (g *firstChunkGate) Written() bool {
	if g.committed {
		return g.real.Written()
	}
	return g.buf.Len() > 0 || g.status != 0
}

// Committed reports whether the gate has flushed to the wire. Once
// committed, retry is impossible — bytes have left the process.
func (g *firstChunkGate) Committed() bool {
	return g.committed
}

// CommitFirstChunk is the producer's "first real chunk arrived" signal.
// It flushes captured headers + status + buffered body to the real
// writer and switches to pass-through. Idempotent.
func (g *firstChunkGate) CommitFirstChunk() {
	g.commit()
}

// CommitIfBuffered is the orchestrator's deferred terminal flush of an
// uncommitted buffered response (the last error after retries are
// exhausted). No-op when already committed or never touched.
func (g *firstChunkGate) CommitIfBuffered() {
	if g.committed || (g.buf.Len() == 0 && g.status == 0) {
		return
	}
	g.commit()
}

// commit copies captured headers, status, and body to the real writer
// and enters pass-through mode. gin defers WriteHeader to the first
// Write or WriteHeaderNow, so for body-less responses WriteHeaderNow is
// forced explicitly.
func (g *firstChunkGate) commit() {
	if g.committed {
		return
	}
	g.committed = true
	dst := g.real.Header()
	for k, vs := range g.hdr {
		dst[k] = vs
	}
	status := g.status
	if status == 0 {
		status = http.StatusOK
	}
	g.real.WriteHeader(status)
	if g.buf.Len() > 0 {
		_, _ = g.real.Write(g.buf.Bytes())
		g.buf.Reset()
	} else {
		g.real.WriteHeaderNow()
	}
}

// Discard resets captured state for the next retry tier. No-op once committed.
func (g *firstChunkGate) Discard() {
	if g.committed {
		return
	}
	g.buf.Reset()
	g.status = 0
	for k := range g.hdr {
		delete(g.hdr, k)
	}
}

// selectFallbackService picks the next priority tier excluding services
// already tried in this request. requireAPIStyle keeps candidates
// compatible with the already-transformed request body — heterogeneous
// fallback would need re-transformation, out of scope here.
//
// Returns (nil, nil, nil) when no compatible candidate remains.
func (s *Server) selectFallbackService(
	rule *typ.Rule,
	excluded map[string]bool,
	requireAPIStyle protocol.APIStyle,
) (*typ.Provider, *loadbalance.Service, error) {
	available := make([]*loadbalance.Service, 0)
	candidateProviders := make(map[string]*typ.Provider)
	for _, svc := range rule.GetActiveServices() {
		if excluded[svc.ServiceID()] {
			continue
		}
		p, err := s.config.GetProviderByUUID(svc.Provider)
		if err != nil || p == nil {
			continue
		}
		if requireAPIStyle != "" && p.APIStyle != requireAPIStyle {
			continue
		}
		available = append(available, svc)
		candidateProviders[svc.ServiceID()] = p
	}
	if len(available) == 0 {
		return nil, nil, nil
	}

	tempRule := *rule
	tempRule.Services = available
	tempRule.CurrentServiceID = ""

	// Use tier-aware selection when the rule uses tier-based routing.
	// This ensures fallback respects tier ordering (T0 → T1 → T2...)
	// instead of using the generic load balancer which might select
	// any available service regardless of tier priority.
	tactic := rule.LBTactic.Instantiate()
	if tactic.GetType() == loadbalance.TacticTier {
		// TierTactic already handles tier-ordered selection with breaker awareness
		svc := tactic.SelectService(&tempRule)
		if svc == nil {
			return nil, nil, nil
		}
		return candidateProviders[svc.ServiceID()], svc, nil
	}

	// For non-tier tactics, use the load balancer
	svc, err := s.loadBalancer.SelectService(&tempRule)
	if err != nil {
		return nil, nil, err
	}
	if svc == nil {
		return nil, nil, nil
	}
	return candidateProviders[svc.ServiceID()], svc, nil
}

// dispatchAttempt is the per-attempt callback. It writes through
// c.Writer, which is the gate during the failover loop.
type dispatchAttempt func(provider *typ.Provider, model string)

// dispatchWithPriorityFailover runs `attempt` repeatedly, retrying on
// retryable buffered failures until either the gate commits (the
// stream's first real chunk reached the wire, retry impossible) or the
// candidate pool is exhausted (the last buffered error flushes on the
// deferred return).
//
// Single-service requests bypass the gate entirely: with no fallback
// tier, failover is impossible and there is no reason to buffer.
func (s *Server) dispatchWithPriorityFailover(
	c *gin.Context,
	rule *typ.Rule,
	initialProvider *typ.Provider,
	initialModel string,
	attempt dispatchAttempt,
) {
	activeServices := rule.GetActiveServices()
	if len(activeServices) <= 1 {
		attempt(initialProvider, initialModel)
		return
	}

	realWriter := c.Writer
	gate := newFirstChunkGate(realWriter)
	c.Writer = gate
	defer func() {
		c.Writer = realWriter
		gate.CommitIfBuffered()
	}()

	tried := map[string]bool{}
	provider := initialProvider
	model := initialModel

	// Keep the recorder's bound service in sync per attempt so the
	// breaker store gets fed the right serviceID on failure.
	rec, _ := getRecorderFromContext(c)

	for i := 0; i < len(activeServices); i++ {
		serviceID := loadbalance.FormatServiceID(provider.UUID, model)
		tried[serviceID] = true
		if rec != nil {
			rec.SetActiveService(provider, model)
		}

		attempt(provider, model)

		// A committed gate means the stream's first real chunk reached
		// the wire — bytes have left the process, retry is impossible.
		if gate.Committed() {
			return
		}
		status := gate.Status()
		if !isRetryableStatus(status) {
			return
		}

		nextProvider, nextService, err := s.selectFallbackService(rule, tried, initialProvider.APIStyle)
		if err != nil {
			logrus.WithContext(c.Request.Context()).WithFields(logrus.Fields{
				"stage":   "failover",
				"attempt": i + 1,
				"status":  status,
				"error":   err.Error(),
			}).Warnf("[failover] load balancer failed selecting fallback after %d attempt(s) status=%d: %v", i+1, status, err)
			return
		}
		if nextProvider == nil || nextService == nil {
			logrus.WithContext(c.Request.Context()).WithFields(logrus.Fields{
				"stage":   "failover",
				"attempt": i + 1,
				"status":  status,
			}).Warnf("[failover] giving up after %d attempt(s) status=%d (no more services)", i+1, status)
			return
		}
		logrus.WithContext(c.Request.Context()).WithFields(logrus.Fields{
			"stage":        "failover",
			"attempt":      i + 1,
			"status":       status,
			"from_service": serviceID,
			"to_provider":  nextProvider.Name,
			"to_model":     nextService.Model,
		}).Infof("[failover] status=%d → retrying with %s/%s", status, nextProvider.Name, nextService.Model)

		gate.Discard()
		provider = nextProvider
		model = nextService.Model
	}
}
