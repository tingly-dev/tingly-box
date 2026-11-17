package limits

import "sync"

// Lease owns the permits of one client request. It lives in the request
// context; a middleware calls ReleaseAll when the handler returns, which covers
// streaming completion, client cancellation and panics. Failover swaps the
// permit of the failed attempt for the next one.
type Lease struct {
	mu      sync.Mutex
	permits map[string][]*Permit
	closed  bool
}

// NewLease creates an empty lease.
func NewLease() *Lease { return &Lease{permits: map[string][]*Permit{}} }

// Hold registers an attempt's permit under its service ID. If the lease is
// already closed the permit is released immediately (a late arrival must not leak).
func (l *Lease) Hold(serviceID string, p *Permit) {
	if p == nil {
		return
	}
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		p.Release()
		return
	}
	l.permits[serviceID] = append(l.permits[serviceID], p)
	l.mu.Unlock()
}

// Release frees the permits held for one service (a failed failover attempt).
func (l *Lease) Release(serviceID string) {
	l.mu.Lock()
	ps := l.permits[serviceID]
	delete(l.permits, serviceID)
	l.mu.Unlock()
	for _, p := range ps {
		p.Release()
	}
}

// ReleaseAll frees everything and closes the lease.
func (l *Lease) ReleaseAll() {
	l.mu.Lock()
	l.closed = true
	all := l.permits
	l.permits = map[string][]*Permit{}
	l.mu.Unlock()
	for _, ps := range all {
		for _, p := range ps {
			p.Release()
		}
	}
}
