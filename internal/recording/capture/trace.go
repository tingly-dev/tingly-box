// Package capture records gateway traffic at its two HTTP boundaries: the
// inbound client request (handed over by the model handlers) and every
// outbound wire exchange with a provider (WrapTransport). Protocol code never
// touches it — see .design/recording.md §2.
//
// One inbound request is one Trace; each upstream HTTP round trip is one
// Exchange. A Trace rides the request context from the recording middleware
// down to the client transport. It is created disabled for every request and
// only starts holding data once a handler calls Enable with the effective
// recording selection, so the disabled cost is one allocation and a few
// atomic loads.
package capture

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// maxBodyBytes caps how much of one body a Trace holds. It is a safety valve
// against pathological payloads, not a size control: long agent sessions are
// kept small by dedup at storage time (.design/recording.md §3.2). A var only
// so tests can lower it.
var maxBodyBytes = 32 << 20

type ctxKey struct{}

// WithTrace returns ctx carrying t.
func WithTrace(ctx context.Context, t *Trace) context.Context {
	return context.WithValue(ctx, ctxKey{}, t)
}

// FromContext returns the Trace carried by ctx, or nil. Every Trace method is
// nil-safe, so callers use the result unconditionally.
func FromContext(ctx context.Context) *Trace {
	if ctx == nil {
		return nil
	}
	t, _ := ctx.Value(ctxKey{}).(*Trace)
	return t
}

// EnableOptions carries what a handler knows once rule resolution is done.
type EnableOptions struct {
	Mode     typ.RecordingMode
	Scenario string
	Rule     string // rule UUID
	Session  typ.SessionID
}

// Trace is the record of one inbound request.
type Trace struct {
	enabled atomic.Bool
	done    atomic.Bool

	mu        sync.Mutex
	started   time.Time
	mode      typ.RecordingMode
	scenario  string
	rule      string
	session   typ.SessionID
	inbound   *Message
	exchanges []*Exchange
	err       string
}

// New returns a disabled Trace.
func New() *Trace {
	return &Trace{started: time.Now()}
}

// Enabled reports whether the Trace is recording.
func (t *Trace) Enabled() bool {
	return t != nil && t.enabled.Load() && !t.done.Load()
}

// Enable turns recording on with the request's effective selection. A
// disabled mode leaves the Trace off. Enabling twice keeps the first call's
// settings — failover attempts re-run the handler prologue.
func (t *Trace) Enable(opts EnableOptions) {
	if t == nil || t.done.Load() || !opts.Mode.Enabled() {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.enabled.Load() {
		return
	}
	t.mode = typ.ParseRecordingMode(string(opts.Mode))
	t.scenario = opts.Scenario
	t.rule = opts.Rule
	t.session = opts.Session
	t.enabled.Store(true)
}

// SetInboundRequest stores the client request exactly as received. Called by
// the model handlers right after reading the body, before Enable is known;
// it only keeps a reference, so it is free when recording stays off.
func (t *Trace) SetInboundRequest(method, url string, header map[string][]string, body []byte) {
	if t == nil || t.done.Load() {
		return
	}
	m := &Message{
		Method:  method,
		URL:     url,
		Headers: redactHeaders(header),
		Size:    int64(len(body)),
	}
	if len(body) > maxBodyBytes {
		body = body[:maxBodyBytes]
		m.Truncated = true
	}
	m.Body = body
	m.Complete = !m.Truncated
	t.mu.Lock()
	t.inbound = m
	t.mu.Unlock()
}

// SetError records the request-level error, if any.
func (t *Trace) SetError(err error) {
	if t == nil || err == nil {
		return
	}
	t.mu.Lock()
	t.err = err.Error()
	t.mu.Unlock()
}

// beginExchange appends a new Exchange when the Trace is recording.
func (t *Trace) beginExchange(p ProviderInfo) *Exchange {
	if !t.Enabled() {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	ex := &Exchange{
		Seq:       len(t.exchanges) + 1,
		Provider:  p,
		StartedAt: time.Now(),
		withBody: bodyPoints{
			request:  t.mode.Has(typ.RecordUpstreamRequest),
			response: t.mode.Has(typ.RecordUpstreamResponse),
		},
	}
	t.exchanges = append(t.exchanges, ex)
	return ex
}

// Finish freezes the Trace and returns its snapshot, or nil when it never
// recorded. Exchanges whose response is still open are snapshotted as
// incomplete; later writes to them are dropped.
func (t *Trace) Finish(requestID string) *Snapshot {
	if t == nil || !t.enabled.Load() || !t.done.CompareAndSwap(false, true) {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	s := &Snapshot{
		RequestID: requestID,
		Timestamp: t.started.UTC(),
		Duration:  time.Since(t.started),
		Scenario:  t.scenario,
		Rule:      t.rule,
		Session:   t.session,
		Mode:      t.mode,
		Error:     t.err,
	}
	if t.mode.Has(typ.RecordClientRequest) && t.inbound != nil {
		s.Inbound = t.inbound
	}
	for _, ex := range t.exchanges {
		s.Exchanges = append(s.Exchanges, ex.snapshot())
	}
	t.inbound = nil
	t.exchanges = nil
	return s
}

// Snapshot is the frozen, storage-ready form of a Trace.
type Snapshot struct {
	RequestID string
	Timestamp time.Time
	Duration  time.Duration
	Scenario  string
	Rule      string
	Session   typ.SessionID
	Mode      typ.RecordingMode
	Error     string

	Inbound   *Message // client_request; nil when not selected
	Exchanges []*ExchangeSnapshot
}

// Message is one captured HTTP request or response.
type Message struct {
	Method      string
	URL         string
	Status      int
	Headers     map[string]string
	ContentType string
	Stream      bool // response is text/event-stream
	Body        []byte
	Size        int64 // bytes seen, including any truncated tail
	Truncated   bool
	Complete    bool // body was read to EOF
}
