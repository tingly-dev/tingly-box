package process

import (
	"bytes"
	"context"
	"errors"
	"io"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestOSExecFactory_EchoSucceeds(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix-only test")
	}
	f := NewOSExecFactory()
	h, err := f.Start(context.Background(), LaunchSpec{
		Command: []string{"/bin/echo", "hello"},
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	out, err := io.ReadAll(h.Stdout())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got := string(bytes.TrimSpace(out)); got != "hello" {
		t.Fatalf("stdout = %q, want %q", got, "hello")
	}
	if err := h.Wait(); err != nil {
		t.Fatalf("wait: %v", err)
	}
	select {
	case <-h.Done():
	default:
		t.Fatalf("Done() not closed after Wait")
	}
}

func TestOSExecFactory_KillTerminates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix-only test")
	}
	f := NewOSExecFactory()
	h, err := f.Start(context.Background(), LaunchSpec{
		Command: []string{"/bin/sh", "-c", "sleep 60"},
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := h.Kill(); err != nil {
		t.Fatalf("kill: %v", err)
	}
	// Wait must return promptly after Kill.
	done := make(chan error, 1)
	go func() { done <- h.Wait() }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("Wait did not return after Kill")
	}
}

func TestOSExecFactory_KillIdempotent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix-only test")
	}
	f := NewOSExecFactory()
	h, err := f.Start(context.Background(), LaunchSpec{
		Command: []string{"/bin/echo", "x"},
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	_ = h.Wait()
	if err := h.Kill(); err != nil {
		t.Fatalf("first Kill after exit: %v", err)
	}
	if err := h.Kill(); err != nil {
		t.Fatalf("second Kill: %v", err)
	}
}

func TestOSExecFactory_StartMissingBinary(t *testing.T) {
	f := NewOSExecFactory()
	_, err := f.Start(context.Background(), LaunchSpec{
		Command: []string{"/definitely/not/a/binary/here"},
	})
	if err == nil {
		t.Fatalf("expected error starting missing binary")
	}
}

func TestFakeFactory_RecordsSpec(t *testing.T) {
	f := NewFakeFactory()
	spec := LaunchSpec{Command: []string{"claude", "--resume", "abc"}}
	h, err := f.Start(context.Background(), spec)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer h.Kill()

	starts := f.Starts()
	if len(starts) != 1 || len(starts[0].Command) != 3 || starts[0].Command[0] != "claude" {
		t.Fatalf("unexpected Starts: %+v", starts)
	}
	if got := h.(*FakeHandle).Spec().Command[0]; got != "claude" {
		t.Fatalf("Spec().Command[0] = %q", got)
	}
}

func TestFakeFactory_ScriptedOutputThenExit(t *testing.T) {
	f := NewFakeFactory()
	f.OnStart = func(_ context.Context, _ LaunchSpec, h *FakeHandle) {
		go func() {
			_, _ = h.WriteOutput([]byte("event1\n"))
			_, _ = h.WriteOutput([]byte("event2\n"))
			h.FinishOutput()
			h.SignalExit(nil)
		}()
	}

	h, err := f.Start(context.Background(), LaunchSpec{Command: []string{"x"}})
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	got, err := io.ReadAll(h.Stdout())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "event1\nevent2\n" {
		t.Fatalf("stdout = %q", got)
	}
	if err := h.Wait(); err != nil {
		t.Fatalf("wait: %v", err)
	}
}

func TestFakeFactory_ExitErrPropagated(t *testing.T) {
	wantErr := errors.New("boom")
	f := NewFakeFactory()
	f.OnStart = func(_ context.Context, _ LaunchSpec, h *FakeHandle) {
		go func() {
			h.FinishOutput()
			h.SignalExit(wantErr)
		}()
	}
	h, err := f.Start(context.Background(), LaunchSpec{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	_, _ = io.ReadAll(h.Stdout())
	if got := h.Wait(); !errors.Is(got, wantErr) {
		t.Fatalf("Wait err = %v, want %v", got, wantErr)
	}
}

func TestFakeFactory_KillBeforeExitClosesAndSignals(t *testing.T) {
	f := NewFakeFactory()
	h, err := f.Start(context.Background(), LaunchSpec{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	done := make(chan struct{})
	go func() {
		_, _ = io.ReadAll(h.Stdout())
		close(done)
	}()

	if err := h.Kill(); err != nil {
		t.Fatalf("kill: %v", err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("Stdout did not EOF after Kill")
	}

	if got := h.Wait(); !errors.Is(got, ErrKilled) {
		t.Fatalf("Wait after Kill = %v, want ErrKilled", got)
	}
	// Idempotent.
	if err := h.Kill(); err != nil {
		t.Fatalf("second Kill: %v", err)
	}
}

func TestFakeHandle_StdinObservable(t *testing.T) {
	f := NewFakeFactory()
	h, err := f.Start(context.Background(), LaunchSpec{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	fh := h.(*FakeHandle)
	defer fh.Kill()

	var got bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(&got, fh.StdinR)
	}()

	if _, err := fh.Stdin().Write([]byte("hello child\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = fh.Stdin().Close()
	wg.Wait()

	if s := got.String(); s != "hello child\n" {
		t.Fatalf("StdinR observed %q", s)
	}
}
