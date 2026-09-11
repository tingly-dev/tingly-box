package process

import (
	"context"
	"errors"
	"io"
	"sync"
)

// FakeFactory is the test Factory. It never spawns a real process; instead
// each call to Start produces a [FakeHandle] whose stdin/stdout are wired to
// in-memory pipes. Tests script the handle's behavior either via [OnStart]
// or by calling helpers on the returned FakeHandle directly.
type FakeFactory struct {
	// OnStart, if non-nil, is invoked synchronously inside Start with the
	// freshly-constructed FakeHandle. Use it to install scripted I/O.
	OnStart func(context.Context, LaunchSpec, *FakeHandle)

	mu      sync.Mutex
	starts  []LaunchSpec
	handles []*FakeHandle
}

// NewFakeFactory returns an empty FakeFactory. Configure OnStart before
// passing it to a runner.
func NewFakeFactory() *FakeFactory { return &FakeFactory{} }

// Starts returns the LaunchSpec from every Start call, in order.
func (f *FakeFactory) Starts() []LaunchSpec {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]LaunchSpec, len(f.starts))
	copy(out, f.starts)
	return out
}

// Handles returns every FakeHandle produced by this factory, in order.
func (f *FakeFactory) Handles() []*FakeHandle {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*FakeHandle, len(f.handles))
	copy(out, f.handles)
	return out
}

func (f *FakeFactory) Start(ctx context.Context, spec LaunchSpec) (Handle, error) {
	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()

	h := &FakeHandle{
		StdinR:  stdinR,
		stdinW:  stdinW,
		stdoutR: stdoutR,
		stdoutW: stdoutW,
		done:    make(chan struct{}),
		exitErr: nil,
		spec:    spec,
	}

	f.mu.Lock()
	f.starts = append(f.starts, spec)
	f.handles = append(f.handles, h)
	onStart := f.OnStart
	f.mu.Unlock()

	if onStart != nil {
		onStart(ctx, spec, h)
	}
	return h, nil
}

// FakeHandle is the Handle returned by FakeFactory.
//
// Its Stdin and Stdout are connected to in-memory pipes:
//   - Bytes the system under test writes to Stdin can be read from StdinR.
//   - Bytes pushed via WriteOutput appear on Stdout.
//
// A FakeHandle starts in the running state. Call SignalExit (with optional
// error) or Kill to transition it to exited.
type FakeHandle struct {
	// StdinR exposes the read end of the stdin pipe so tests can observe
	// what the system under test wrote to the child.
	StdinR *io.PipeReader

	stdinW  *io.PipeWriter
	stdoutR *io.PipeReader
	stdoutW *io.PipeWriter

	mu      sync.Mutex
	done    chan struct{}
	exited  bool
	exitErr error

	closeOutOnce sync.Once
	killOnce     sync.Once

	spec LaunchSpec
}

// Spec returns the LaunchSpec used to construct this handle.
func (h *FakeHandle) Spec() LaunchSpec { return h.spec }

func (h *FakeHandle) Stdin() io.WriteCloser { return h.stdinW }
func (h *FakeHandle) Stdout() io.ReadCloser { return h.stdoutR }
func (h *FakeHandle) Done() <-chan struct{} { return h.done }

// WriteOutput pushes bytes to the handle's Stdout. Tests use this to emit
// scripted events. Safe to call concurrently with reads from Stdout.
func (h *FakeHandle) WriteOutput(p []byte) (int, error) {
	return h.stdoutW.Write(p)
}

// FinishOutput closes the Stdout pipe, signaling EOF to the reader. Idempotent.
func (h *FakeHandle) FinishOutput() {
	h.closeOutOnce.Do(func() { _ = h.stdoutW.Close() })
}

// SignalExit transitions the handle to the exited state with the given error.
// Subsequent Wait calls return err. Idempotent: only the first call's error is recorded.
func (h *FakeHandle) SignalExit(err error) {
	h.mu.Lock()
	if h.exited {
		h.mu.Unlock()
		return
	}
	h.exited = true
	h.exitErr = err
	h.mu.Unlock()
	close(h.done)
}

func (h *FakeHandle) Wait() error {
	<-h.done
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.exitErr
}

// Kill closes both pipes and signals exit with a synthetic "killed" error.
// Idempotent.
func (h *FakeHandle) Kill() error {
	h.killOnce.Do(func() {
		_ = h.stdinW.Close()
		h.FinishOutput()
		h.SignalExit(ErrKilled)
	})
	return nil
}

// ErrKilled is the synthetic Wait error reported after Kill.
var ErrKilled = errors.New("process killed")
