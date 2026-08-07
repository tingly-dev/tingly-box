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
// disconnected and emit the disconnect event so the manager's auto-reconnect
// takes over instead of leaving a silently deaf bot.
func TestRecoverLoopFlipsDisconnectedAndEmits(t *testing.T) {
	b := NewBaseBot(&Config{UUID: "u", Platform: PlatformTelegram})
	b.MarkConnected(true)

	disconnected := make(chan struct{})
	b.OnDisconnected(func() { close(disconnected) })

	func() {
		defer b.RecoverLoop("test loop")
		panic("loop died")
	}()

	if b.IsConnected() {
		t.Fatal("bot still marked connected after loop panic")
	}
	select {
	case <-disconnected:
	case <-time.After(time.Second):
		t.Fatal("disconnect event not emitted after loop panic")
	}
}
