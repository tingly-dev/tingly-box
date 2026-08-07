// Package safego contains panics to the goroutine that raised them.
//
// A panic that escapes any goroutine kills the whole host process (agentboot
// runs inside tingly-box), and recover() cannot cross goroutine boundaries —
// so every goroutine this module spawns recovers for itself. Mirrors
// remote/safego in the main module and imbot/core/safego.go; see the
// tingly-box .design/bot-panic-isolation.md for the layered model.
package safego

import (
	"runtime/debug"

	"github.com/sirupsen/logrus"
)

// Recover is the deferred panic container:
//
//	defer wg.Done()          // runs last
//	defer safego.Recover("pump")
//
// Order defers so cleanup that must still happen on panic (close a channel,
// wg.Done) is registered BEFORE Recover, i.e. runs after it.
func Recover(name string) {
	if r := recover(); r != nil {
		logrus.WithFields(logrus.Fields{
			"goroutine": name,
			"panic":     r,
			"stack":     string(debug.Stack()),
		}).Error("Panic contained; goroutine exited, process unaffected")
	}
}
