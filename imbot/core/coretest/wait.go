// Package coretest holds small helpers shared by platform adapter tests.
package coretest

import (
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/imbot/core"
)

// WaitForMessage blocks until ch delivers one message, or fails the test
// after a short timeout. BaseBot.EmitMessage (core/base.go) dispatches each
// OnMessage handler via `go func`, never synchronously, so a test that
// triggers a message and then reads it back needs this instead of a bare
// channel receive right after the trigger call.
func WaitForMessage(t *testing.T, ch <-chan core.Message) core.Message {
	t.Helper()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for OnMessage to fire")
		return core.Message{}
	}
}
