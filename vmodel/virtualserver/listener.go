package virtualserver

import (
	"context"
	"errors"
	"net"
	"sync"
)

// memListener is a net.Listener whose connections never touch the network.
// Dial hands one end of a net.Pipe to the caller and queues the other end for
// Accept, so an http.Server on top of it speaks real HTTP/1.1 — serialization,
// chunked streaming, status codes — over an in-process duplex stream.
//
// It exists so vmodel providers can be reached through an ordinary
// http.Transport (DialContext = memListener.Dial) with nothing bound on the
// host: no port to collide, nothing for another process to hit, nothing to
// clean up after a crash. See .design/vmodel-transport.md.
type memListener struct {
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
}

var errMemListenerClosed = errors.New("vmodel: in-memory listener closed")

func newMemListener() *memListener {
	return &memListener{
		conns: make(chan net.Conn),
		done:  make(chan struct{}),
	}
}

// Dial returns the client side of a fresh connection. network and addr are
// ignored; the listener is the only possible peer.
func (l *memListener) Dial(ctx context.Context, _, _ string) (net.Conn, error) {
	client, server := net.Pipe()
	var err error
	select {
	case l.conns <- server:
		return client, nil
	case <-l.done:
		err = errMemListenerClosed
	case <-ctx.Done():
		err = ctx.Err()
	}
	_ = client.Close()
	_ = server.Close()
	return nil, err
}

// Accept implements net.Listener.
func (l *memListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.done:
		return nil, errMemListenerClosed
	}
}

// Close implements net.Listener. It is idempotent and unblocks any pending
// Accept or Dial.
func (l *memListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

// Addr implements net.Listener.
func (l *memListener) Addr() net.Addr { return memAddr{} }

type memAddr struct{}

func (memAddr) Network() string { return "vmodel" }
func (memAddr) String() string  { return "vmodel.internal" }
