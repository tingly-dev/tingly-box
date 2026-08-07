package core

import (
	"runtime/debug"
)

// Panic containment for the bot stack.
//
// Layered model (see tingly-box .design/bot-panic-isolation.md):
// every goroutine we spawn and every entry point a third-party SDK calls
// back into must recover panics itself — a panic that escapes any goroutine
// kills the whole process, and recover() cannot cross goroutine boundaries.
// Panics inside goroutines the SDK itself spawns are NOT catchable here;
// those are fixed at the SDK (upgrade/patch), never papered over in callers.

// SafeGo runs fn in a new goroutine, containing any panic to a log line.
// Use it for every `go` statement whose body is not already guarded by
// RecoverPanic / a platform Recover* helper.
func SafeGo(logger Logger, name string, fn func()) {
	go func() {
		defer RecoverPanic(logger, name)
		fn()
	}()
}

// RecoverPanic is the deferred half of SafeGo, for goroutines that need
// their own defer ordering (e.g. wg.Done):
//
//	defer b.wg.Done()
//	defer core.RecoverPanic(b.Logger(), "x loop")
func RecoverPanic(logger Logger, name string) {
	if r := recover(); r != nil {
		if logger == nil {
			logger = NewLogger(nil)
		}
		logger.Error("panic in %s (contained): %v\n%s", name, r, debug.Stack())
	}
}

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
// emits the disconnect event — the manager's auto-reconnect then rebuilds
// the connection instead of leaving a zombie bot.
func (b *BaseBot) RecoverLoop(name string) {
	if r := recover(); r != nil {
		b.Logger().Error("panic in %s (contained, reconnecting): %v\n%s", name, r, debug.Stack())
		b.UpdateReady(false)
		b.UpdateConnected(false)
		b.EmitDisconnected()
	}
}
