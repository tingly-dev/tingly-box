package core

import (
	"fmt"
	"runtime/debug"
)

// Panic containment at the platform SDK boundary.
//
// A panic that escapes any goroutine kills the whole process, and recover()
// cannot cross goroutine boundaries. Rather than wrapping every goroutine,
// containment is applied where the actual risk lives — the trust boundary
// where third-party SDK code runs or third-party payloads are parsed:
//
//   - RecoverCallback: entry points an SDK invokes on its own goroutine
//   - RecoverLoop: receive loops of ours that run SDK code / adapters
//
// Goroutines running only our own code rely on the existing per-bot
// supervision and recovered handler dispatch instead. Panics inside
// goroutines an SDK spawns for itself are not catchable here at all; those
// are fixed at the SDK (see tingly-box .design/bot-panic-isolation.md).

// RecoverCallback contains a panic in a callback invoked by a third-party
// SDK on one of its own goroutines. The message is dropped, the connection
// is left alone — one poisonous update must not take the bot down.
func (b *BaseBot) RecoverCallback(name string) {
	if r := recover(); r != nil {
		b.Logger().Error("panic in %s callback (contained, message dropped): %v\n%s", name, r, debug.Stack())
	}
}

// RecoverLoop contains a panic in a platform receive loop. Unlike a
// per-message callback, a dead receive loop means the bot silently stops
// hearing anything, so besides logging it flips the bot to disconnected and
// emits an ErrPanic error event. Deliberately NOT a disconnect event: a
// disconnect means "network dropped, reconnect in place", but after a panic
// the bot's state is suspect — the lifecycle owner listening for ErrPanic
// (remote/control/bot.Manager) closes the whole bot cleanly and lets its
// reconcile loop rebuild a fresh instance. Either way no zombie: without an
// owner the bot at least reads as disconnected in status, never as healthy.
func (b *BaseBot) RecoverLoop(name string) {
	if r := recover(); r != nil {
		b.Logger().Error("panic in %s (contained, closing bot): %v\n%s", name, r, debug.Stack())
		b.UpdateReady(false)
		b.UpdateConnected(false)
		var platform Platform
		if b.config != nil {
			platform = b.config.Platform
		}
		b.EmitError(NewPanicError(platform, fmt.Sprintf("panic in %s: %v", name, r)))
	}
}
