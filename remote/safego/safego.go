// Package safego contains panics to the goroutine that raised them.
//
// A panic that escapes any goroutine kills the whole tingly-box process, and
// recover() cannot cross goroutine boundaries — so every goroutine the remote
// bot stack spawns must recover for itself. This package is the one way to do
// that; a bare `go` statement in bot-facing code is a review error.
// See .design/bot-panic-isolation.md for the layered model (the imbot module
// has its own equivalent in imbot/core/safego.go).
package safego

import (
	"runtime/debug"

	"github.com/sirupsen/logrus"
)

// Go runs fn in a new goroutine, containing any panic to an error log.
func Go(name string, fn func()) {
	go func() {
		defer Recover(name)
		fn()
	}()
}

// Recover is the deferred half of Go, for goroutines that need their own
// defer ordering (wg.Done, channel close):
//
//	defer m.wg.Done()
//	defer safego.Recover("session cleanup loop")
func Recover(name string) {
	if r := recover(); r != nil {
		logrus.WithFields(logrus.Fields{
			"goroutine": name,
			"panic":     r,
			"stack":     string(debug.Stack()),
		}).Error("Panic contained; goroutine exited, process unaffected")
	}
}
