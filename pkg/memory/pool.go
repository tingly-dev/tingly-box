package memory

import (
	"sync"
)

// ByteBufferPool manages reusable byte buffers for request body copying.
// This prevents memory leaks from SDK gjson usage while reducing GC pressure.
type ByteBufferPool struct {
	pool sync.Pool
	// Initial buffer capacity
	initialCap int
	// Maximum buffer capacity (larger buffers are not pooled)
	maxCap int
}

// NewByteBufferPool creates a new byte buffer pool.
// initialCap: suggested initial capacity (e.g., 32KB for typical API requests)
// maxCap: maximum capacity to pool (e.g., 1MB to prevent large buffers from staying in memory)
func NewByteBufferPool(initialCap, maxCap int) *ByteBufferPool {
	return &ByteBufferPool{
		initialCap: initialCap,
		maxCap:     maxCap,
		pool: sync.Pool{
			New: func() interface{} {
				b := make([]byte, 0, initialCap)
				return &b
			},
		},
	}
}

// Get returns a buffer from the pool or creates a new one if needed.
func (p *ByteBufferPool) Get() *[]byte {
	return p.pool.Get().(*[]byte)
}

// Put returns a buffer to the pool if it's not too large.
func (p *ByteBufferPool) Put(buf *[]byte) {
	if buf == nil {
		return
	}
	// Don't pool buffers that are too large
	if cap(*buf) > p.maxCap {
		return
	}
	// Reset length to 0, keep capacity
	*buf = (*buf)[:0]
	p.pool.Put(buf)
}

// Copy copies src into a pooled buffer and returns the copy.
// The returned buffer is independent and can be used after the pool buffer is returned.
func (p *ByteBufferPool) Copy(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}

	// Get buffer from pool
	bufPtr := p.pool.Get().(*[]byte)
	buf := (*bufPtr)[:0]

	// If source is larger than capacity, allocate directly
	if len(src) > cap(*bufPtr) {
		// Allocate new memory for large requests
		result := make([]byte, len(src))
		copy(result, src)
		// Return the buffer to pool (it's still usable)
		p.Put(bufPtr)
		return result
	}

	// Copy into pooled buffer
	buf = append(buf, src...)

	// Create a copy to return (so the pool buffer can be reused)
	result := make([]byte, len(buf))
	copy(result, buf)

	// Return buffer to pool
	p.Put(bufPtr)

	return result
}

// Default pool for typical API requests
// 32KB initial capacity handles most requests
// 1MB max prevents large buffers from staying in memory
var DefaultByteBufferPool = NewByteBufferPool(32*1024, 1024*1024)

// CopyRequestBody is a convenience function using the default pool.
func CopyRequestBody(body []byte) []byte {
	return DefaultByteBufferPool.Copy(body)
}
