package safego

import (
	"testing"
	"time"
)

func TestGoContainsPanic(t *testing.T) {
	done := make(chan struct{})
	Go("test worker", func() {
		defer close(done)
		panic("boom")
	})
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("goroutine did not finish")
	}
	// Reaching here at all means the panic did not escape the goroutine.
}

func TestRecoverAsDefer(t *testing.T) {
	func() {
		defer Recover("inline")
		panic("boom")
	}()
}
