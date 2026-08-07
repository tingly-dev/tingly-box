package core

import (
	"testing"
	"time"
)

func TestRecoverCallbackContainsPanic(t *testing.T) {
	b := NewBaseBot(&Config{UUID: "u", Platform: PlatformTelegram})
	func() {
		defer b.RecoverCallback("test callback")
		panic("bad message")
	}()
}

// A receive-loop panic must not only be contained: the bot must flip to
// disconnected and emit an ErrPanic error event so its lifecycle owner can
// close and rebuild it. It must NOT emit a disconnect event — that path
// means "reconnect in place", which is wrong for a bot whose state is
// suspect after a crash.
func TestRecoverLoopEmitsPanicErrorNotDisconnect(t *testing.T) {
	b := NewBaseBot(&Config{UUID: "u", Platform: PlatformTelegram})
	b.MarkConnected(true)

	errCh := make(chan error, 1)
	b.OnError(func(err error) { errCh <- err })
	disconnected := make(chan struct{}, 1)
	b.OnDisconnected(func() { disconnected <- struct{}{} })

	func() {
		defer b.RecoverLoop("test loop")
		panic("loop died")
	}()

	if b.IsConnected() {
		t.Fatal("bot still marked connected after loop panic")
	}
	select {
	case err := <-errCh:
		if !IsPanicError(err) {
			t.Fatalf("expected ErrPanic error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("panic error event not emitted after loop panic")
	}
	select {
	case <-disconnected:
		t.Fatal("disconnect event emitted after loop panic; crash must close the bot, not reconnect it")
	case <-time.After(100 * time.Millisecond):
	}
}
