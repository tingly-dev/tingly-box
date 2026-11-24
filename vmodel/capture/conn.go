package capture

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type connKey struct{}

// Listener wraps l so every accepted connection is teed into raw/conn-N.in
// (and raw/conn-N.out when Options.Out is set).
func (t *Tap) Listener(l net.Listener) net.Listener { return &tapListener{Listener: l, tap: t} }

type tapListener struct {
	net.Listener
	tap *Tap
}

func (l *tapListener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return l.tap.newConn(c), nil
}

// Install hooks srv so each request can be located in its connection's raw
// files. It chains any ConnContext / ConnState already set.
func (t *Tap) Install(srv *http.Server) {
	prevCtx, prevState := srv.ConnContext, srv.ConnState
	srv.ConnContext = func(ctx context.Context, c net.Conn) context.Context {
		if prevCtx != nil {
			ctx = prevCtx(ctx, c)
		}
		if tc, ok := c.(*tapConn); ok {
			ctx = context.WithValue(ctx, connKey{}, tc)
		}
		return ctx
	}
	srv.ConnState = func(c net.Conn, s http.ConnState) {
		if tc, ok := c.(*tapConn); ok {
			switch s {
			case http.StateIdle:
				tc.finish(false)
			case http.StateClosed, http.StateHijacked:
				tc.finish(true)
				if s == http.StateClosed {
					tc.closeFiles()
				}
			}
		}
		if prevState != nil {
			prevState(c, s)
		}
	}
}

// tapConn counts and tees the bytes of one connection. Offsets are into the
// two raw files, which hold exactly the bytes that crossed the connection.
type tapConn struct {
	net.Conn
	tap *Tap
	id  int64

	mu              sync.Mutex
	in, out         *os.File
	inOff, outOff   int64 // bytes seen so far
	inMark, outMark int64 // offsets where the next exchange starts
	cur             *exchange
}

func (t *Tap) newConn(c net.Conn) *tapConn {
	tc := &tapConn{Conn: c, tap: t, id: t.connSeq.Add(1)}
	open := func(ext string) *os.File {
		f, err := os.OpenFile(filepath.Join(t.dir, "raw", fmt.Sprintf("conn-%04d.%s", tc.id, ext)), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			t.warn("capture: " + err.Error())
			return nil
		}
		return f
	}
	tc.in = open("in")
	if t.opts.Out {
		tc.out = open("out")
	}
	return tc
}

func (c *tapConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		c.mu.Lock()
		c.inOff += int64(n)
		if c.in != nil {
			if _, werr := c.in.Write(p[:n]); werr != nil {
				c.tap.warn("capture: write raw input: " + werr.Error())
				c.in = nil
			}
		}
		c.mu.Unlock()
	}
	return n, err
}

func (c *tapConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if n > 0 {
		c.mu.Lock()
		c.outOff += int64(n)
		if c.out != nil {
			if _, werr := c.out.Write(p[:n]); werr != nil {
				c.tap.warn("capture: write raw output: " + werr.Error())
				c.out = nil
			}
		}
		c.mu.Unlock()
	}
	return n, err
}

// exchange is one request being served on a connection.
type exchange struct {
	seq    int64
	t      time.Time
	method string
	target string

	mu     sync.Mutex
	status int

	inStart, outStart int64
}

func (e *exchange) setStatus(code int) {
	e.mu.Lock()
	if e.status == 0 {
		e.status = code
	}
	e.mu.Unlock()
}

func (e *exchange) getStatus() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.status
}

func (c *tapConn) begin(e *exchange) {
	c.mu.Lock()
	e.inStart, e.outStart = c.inMark, c.outMark
	c.cur = e
	c.mu.Unlock()
}

// finish closes the current exchange at the connection's current offsets.
// closing is set when the connection is going away, in which case bytes that
// never became a request (a malformed request line, a client that hung up
// mid-head) are still pointed at by an "unparsed" entry.
func (c *tapConn) finish(closing bool) {
	c.mu.Lock()
	e := c.cur
	c.cur = nil
	inEnd, outEnd := c.inOff, c.outOff
	inStart, outStart := c.inMark, c.outMark
	if e != nil {
		inStart, outStart = e.inStart, e.outStart
	}
	c.inMark, c.outMark = inEnd, outEnd
	hasOut := c.out != nil
	c.mu.Unlock()

	switch {
	case e != nil:
		ent := entry{Seq: e.seq, T: e.t, Conn: c.id, In: [2]int64{inStart, inEnd},
			Method: e.method, Target: e.target, Status: e.getStatus(), Incomplete: closing && e.getStatus() == 0}
		if hasOut {
			ent.Out = &[2]int64{outStart, outEnd}
		}
		c.tap.write(ent)
	case closing && inEnd > inStart:
		ent := entry{T: time.Now().UTC(), Conn: c.id, In: [2]int64{inStart, inEnd}, Unparsed: true}
		if hasOut {
			ent.Out = &[2]int64{outStart, outEnd}
		}
		c.tap.write(ent)
	}
}

func (c *tapConn) closeFiles() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.in != nil {
		_ = c.in.Close()
		c.in = nil
	}
	if c.out != nil {
		_ = c.out.Close()
		c.out = nil
	}
}
