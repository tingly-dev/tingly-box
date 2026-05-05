package agentboot

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"
)

// stubResult is a small helper for waitFn closures.
func stubResult() *Result { return &Result{} }

func TestRunnerHandle_EventsEmittedInOrder(t *testing.T) {
	h := newRunnerHandle(8,
		func(string, ControlResponse) error { return nil },
		func() {},
		func() (*Result, error) { return stubResult(), nil },
	)
	ctx := context.Background()

	go func() {
		h.emit(ctx, MessageEvent{})
		h.emit(ctx, MessageEvent{})
		h.emit(ctx, MessageEvent{})
		h.closeStream()
	}()

	count := 0
	for range h.Events() {
		count++
	}
	if count != 3 {
		t.Fatalf("got %d events, want 3", count)
	}
}

func TestRunnerHandle_CloseStreamIdempotent(t *testing.T) {
	h := newRunnerHandle(0,
		func(string, ControlResponse) error { return nil },
		func() {},
		func() (*Result, error) { return stubResult(), nil },
	)

	h.closeStream()
	h.closeStream() // must not panic

	_, ok := <-h.Events()
	if ok {
		t.Fatalf("Events() not closed")
	}
}

func TestRunnerHandle_RespondRoutesByID(t *testing.T) {
	var captured struct {
		sync.Mutex
		reqID string
		resp  ControlResponse
	}
	respFn := func(reqID string, resp ControlResponse) error {
		captured.Lock()
		defer captured.Unlock()
		captured.reqID = reqID
		captured.resp = resp
		return nil
	}

	h := newRunnerHandle(4, respFn,
		func() {},
		func() (*Result, error) { return stubResult(), nil },
	)
	ctx := context.Background()

	go func() {
		h.emit(ctx, ApprovalRequestEvent{ID: "req-A", ToolName: "Bash"})
		// simulate the runner not closing yet; we close after the test finishes Respond.
	}()

	ev := <-h.Events()
	got, ok := ev.(ApprovalRequestEvent)
	if !ok || got.ID != "req-A" {
		t.Fatalf("first event = %+v", ev)
	}

	if err := h.Respond("req-A", ApprovalResponse{Approved: true}); err != nil {
		t.Fatalf("Respond: %v", err)
	}

	captured.Lock()
	defer captured.Unlock()
	if captured.reqID != "req-A" {
		t.Fatalf("captured reqID = %q", captured.reqID)
	}
	if ar, ok := captured.resp.(ApprovalResponse); !ok || !ar.Approved {
		t.Fatalf("captured resp = %+v", captured.resp)
	}

	h.closeStream()
}

func TestRunnerHandle_RespondUnknownIDErrors(t *testing.T) {
	h := newRunnerHandle(0,
		func(string, ControlResponse) error {
			t.Fatalf("responseFn must not be called for unknown id")
			return nil
		},
		func() {},
		func() (*Result, error) { return stubResult(), nil },
	)

	err := h.Respond("never-registered", ApprovalResponse{Approved: true})
	if !errors.Is(err, ErrUnknownRequestID) {
		t.Fatalf("err = %v, want ErrUnknownRequestID", err)
	}
	h.closeStream()
}

func TestRunnerHandle_RespondTwiceErrors(t *testing.T) {
	respCalls := 0
	h := newRunnerHandle(4,
		func(string, ControlResponse) error { respCalls++; return nil },
		func() {},
		func() (*Result, error) { return stubResult(), nil },
	)
	ctx := context.Background()

	go h.emit(ctx, ApprovalRequestEvent{ID: "req-X"})
	<-h.Events()

	if err := h.Respond("req-X", ApprovalResponse{Approved: true}); err != nil {
		t.Fatalf("first Respond: %v", err)
	}
	err := h.Respond("req-X", ApprovalResponse{Approved: false})
	if !errors.Is(err, ErrUnknownRequestID) {
		t.Fatalf("second Respond err = %v, want ErrUnknownRequestID", err)
	}
	if respCalls != 1 {
		t.Fatalf("responseFn called %d times, want 1", respCalls)
	}
	h.closeStream()
}

func TestRunnerHandle_RespondAfterCloseErrors(t *testing.T) {
	h := newRunnerHandle(0,
		func(string, ControlResponse) error {
			t.Fatalf("responseFn must not be called after close")
			return nil
		},
		func() {},
		func() (*Result, error) { return stubResult(), nil },
	)

	h.closeStream()

	if err := h.Respond("anything", ApprovalResponse{}); !errors.Is(err, ErrHandleClosed) {
		t.Fatalf("err = %v, want ErrHandleClosed", err)
	}
}

func TestRunnerHandle_CancelIdempotent(t *testing.T) {
	var calls int32
	h := newRunnerHandle(0,
		func(string, ControlResponse) error { return nil },
		func() { atomic.AddInt32(&calls, 1) },
		func() (*Result, error) { return stubResult(), nil },
	)

	h.Cancel()
	h.Cancel()
	h.Cancel()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("cancelFn called %d times, want 1", got)
	}
	h.closeStream()
}

func TestRunnerHandle_WaitDelegates(t *testing.T) {
	wantErr := errors.New("agent crashed")
	h := newRunnerHandle(0,
		func(string, ControlResponse) error { return nil },
		func() {},
		func() (*Result, error) { return &Result{Output: "out"}, wantErr },
	)
	h.closeStream()

	res, err := h.Wait()
	if !errors.Is(err, wantErr) {
		t.Fatalf("Wait err = %v, want %v", err, wantErr)
	}
	if res == nil || res.Output != "out" {
		t.Fatalf("Wait res = %+v", res)
	}
}

func TestRunnerHandle_EmitDropsOnCtxCancel(t *testing.T) {
	h := newRunnerHandle(0, // unbuffered: emit blocks until consumed
		func(string, ControlResponse) error { return nil },
		func() {},
		func() (*Result, error) { return stubResult(), nil },
	)

	ctx, cancel := context.WithCancel(context.Background())

	emitDone := make(chan struct{})
	go func() {
		h.emit(ctx, ApprovalRequestEvent{ID: "to-be-dropped"})
		close(emitDone)
	}()

	// Give emit a moment to register pending and start blocking on the channel.
	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case <-emitDone:
	case <-time.After(time.Second):
		t.Fatalf("emit did not return after ctx cancel")
	}

	// Pending registration must have been cleaned up: a Respond for the
	// dropped ID must report unknown rather than be routed.
	if err := h.Respond("to-be-dropped", ApprovalResponse{}); !errors.Is(err, ErrUnknownRequestID) {
		t.Fatalf("Respond for dropped event err = %v, want ErrUnknownRequestID", err)
	}
	h.closeStream()
}

func TestRunnerHandle_NoGoroutineLeak(t *testing.T) {
	defer goleak.VerifyNone(t)

	h := newRunnerHandle(4,
		func(string, ControlResponse) error { return nil },
		func() {},
		func() (*Result, error) { return stubResult(), nil },
	)
	ctx := context.Background()

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.emit(ctx, MessageEvent{})
		h.emit(ctx, ApprovalRequestEvent{ID: "leak-check"})
		h.emit(ctx, MessageEvent{})
		h.closeStream()
	}()

	for ev := range h.Events() {
		if ar, ok := ev.(ApprovalRequestEvent); ok {
			_ = h.Respond(ar.ID, ApprovalResponse{Approved: true})
		}
	}
	<-done
}
