package capture

import (
	"sync"
	"time"
)

// ProviderInfo identifies the provider an Exchange went to. It is fixed per
// client transport, so WrapTransport takes it at construction.
type ProviderInfo struct {
	Name     string
	UUID     string
	APIStyle string
}

// bodyPoints records which bodies the selection asked for. Metadata (URL,
// status, headers, timing) is always kept for an Exchange.
type bodyPoints struct {
	request  bool
	response bool
}

// Exchange is one upstream HTTP round trip.
type Exchange struct {
	Seq       int
	Provider  ProviderInfo
	StartedAt time.Time

	withBody bodyPoints

	mu       sync.Mutex
	frozen   bool
	request  *Message
	response *Message
	respBody *cappedBuffer
	ttfb     time.Duration
	duration time.Duration
	err      string
}

func (ex *Exchange) setRequest(m *Message) {
	ex.mu.Lock()
	defer ex.mu.Unlock()
	if !ex.frozen {
		ex.request = m
	}
}

// setResponseHead records status and headers when they arrive (time to first
// byte). The body follows through appendBody / closeBody.
func (ex *Exchange) setResponseHead(m *Message) {
	ex.mu.Lock()
	defer ex.mu.Unlock()
	if ex.frozen {
		return
	}
	ex.ttfb = time.Since(ex.StartedAt)
	ex.response = m
	if ex.withBody.response {
		ex.respBody = &cappedBuffer{}
	}
}

func (ex *Exchange) appendBody(p []byte) {
	ex.mu.Lock()
	defer ex.mu.Unlock()
	if ex.frozen || ex.respBody == nil {
		return
	}
	ex.respBody.write(p)
}

// closeBody ends the response; end is one of EndEOF / EndClosed / EndError.
func (ex *Exchange) closeBody(end string) {
	ex.mu.Lock()
	defer ex.mu.Unlock()
	if ex.frozen || ex.response == nil {
		return
	}
	if ex.duration == 0 {
		ex.duration = time.Since(ex.StartedAt)
	}
	ex.response.End = end
}

func (ex *Exchange) fail(err error) {
	ex.mu.Lock()
	defer ex.mu.Unlock()
	if ex.frozen || err == nil {
		return
	}
	ex.err = err.Error()
	if ex.duration == 0 {
		ex.duration = time.Since(ex.StartedAt)
	}
}

// ExchangeSnapshot is the frozen form of an Exchange.
type ExchangeSnapshot struct {
	Seq       int
	Provider  ProviderInfo
	StartedAt time.Time
	TTFB      time.Duration
	Duration  time.Duration
	Error     string
	Request   *Message // body present only when upstream_request is selected
	Response  *Message // body present only when upstream_response is selected
}

func (ex *Exchange) snapshot() *ExchangeSnapshot {
	ex.mu.Lock()
	defer ex.mu.Unlock()
	ex.frozen = true
	s := &ExchangeSnapshot{
		Seq:       ex.Seq,
		Provider:  ex.Provider,
		StartedAt: ex.StartedAt.UTC(),
		TTFB:      ex.ttfb,
		Duration:  ex.duration,
		Error:     ex.err,
		Request:   ex.request,
	}
	if s.Duration == 0 {
		s.Duration = time.Since(ex.StartedAt)
	}
	if ex.response != nil {
		resp := *ex.response
		if ex.respBody != nil {
			resp.Body = ex.respBody.buf
			resp.Size = ex.respBody.size
			resp.Truncated = ex.respBody.truncated
		}
		s.Response = &resp
	}
	ex.respBody = nil
	return s
}

// cappedBuffer accumulates up to maxBodyBytes and counts the rest.
type cappedBuffer struct {
	buf       []byte
	size      int64
	truncated bool
}

func (b *cappedBuffer) write(p []byte) {
	b.size += int64(len(p))
	if room := maxBodyBytes - len(b.buf); room > 0 {
		if len(p) > room {
			p = p[:room]
			b.truncated = true
		}
		b.buf = append(b.buf, p...)
	} else if len(p) > 0 {
		b.truncated = true
	}
}
