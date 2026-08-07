package core

import (
	"fmt"
	"runtime/debug"
)

// Panic containment at the platform SDK trust boundary. recover() cannot
// cross goroutine boundaries, so it lives where the risk lives: entry points
// SDKs invoke on their own goroutines (RecoverCallback) and our receive
// loops (RecoverLoop). Rationale and the close-vs-reconnect split: ErrPanic
// in types.go and tingly-box .design/bot-panic-isolation.md.

// RecoverCallback contains a panic in a handler or SDK-invoked callback: the
// one event is dropped, the connection is left alone.
func (b *BaseBot) RecoverCallback(name string) {
	if r := recover(); r != nil {
		b.Logger().Error("panic in %s (contained, event dropped): %v\n%s", name, r, debug.Stack())
	}
}

// RecoverLoop contains a panic in a platform receive loop: the bot flips to
// disconnected and emits ErrPanic (see that const — close-and-rebuild, not
// reconnect-in-place, so deliberately no disconnect event). Register cleanup
// defers (wg.Done, close) BEFORE this one so they still run on panic.
func (b *BaseBot) RecoverLoop(name string) {
	if r := recover(); r != nil {
		b.Logger().Error("panic in %s (contained, closing bot): %v\n%s", name, r, debug.Stack())
		err := NewPanicError(b.config.Platform, fmt.Sprintf("panic in %s: %v", name, r))
		b.SetError(err)
		b.UpdateDisconnected()
		b.EmitError(err)
	}
}
