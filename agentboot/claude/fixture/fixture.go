// Package fixture provides a Claude wire-format scripted [process.Factory]
// for testing. It replaces the legacy mockagent: tests now exercise the real
// claude.Driver + claude.Transport + agentboot.Runner pipeline by injecting
// a Factory whose stdout emits scripted events.
//
// Usage:
//
//	script := fixture.Script{
//	    fixture.System("sess-1", "/tmp"),
//	    fixture.AssistantText("Hello, world."),
//	    fixture.PermissionRequest("req-1", "Bash", map[string]any{"command": "ls"}),
//	    fixture.AssistantText("Done."),
//	    fixture.Result(true),
//	}
//	factory := fixture.Factory(script)
//
//	agent := claude.NewAgentWithFactory(claude.Config{}, factory)
//	handle, _ := agent.Execute(ctx, "list files", agentboot.ExecutionOptions{})
//	for ev := range handle.Events() { ... handle.Respond(...) ... }
//	res, _ := handle.Wait()
package fixture

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot/process"
)

// Script is an ordered list of [Step]s the fixture emits to stdout when the
// returned [process.Factory] is started.
type Script []Step

type config struct {
	stepDelay time.Duration
	observe   func([]byte)
}

// Option tweaks fixture behavior.
type Option func(*config)

// WithStepDelay inserts a sleep between successive steps. Useful for tests
// that need to observe intermediate state.
func WithStepDelay(d time.Duration) Option {
	return func(c *config) { c.stepDelay = d }
}

// WithObserveStdin installs a callback that receives every JSON message the
// agent writes to stdin (initial user prompt, control responses, …). Tests
// use this to assert the wire-format of approval responses.
func WithObserveStdin(fn func([]byte)) Option {
	return func(c *config) { c.observe = fn }
}

// Factory returns a [process.Factory] that, when Start is called, spawns a
// goroutine to emit script as Claude wire-format events on stdout. Steps
// that report needsResponse=true block the script until one stdin message
// is observed (i.e. until the consumer calls handle.Respond).
//
// The factory exits cleanly with nil error after the last step. If ctx is
// canceled mid-script, the goroutine returns and signals exit.
func Factory(script Script, opts ...Option) process.Factory {
	cfg := config{}
	for _, o := range opts {
		o(&cfg)
	}

	f := process.NewFakeFactory()
	f.OnStart = func(ctx context.Context, _ process.LaunchSpec, h *process.FakeHandle) {
		go run(ctx, script, cfg, h)
	}
	return f
}

func run(ctx context.Context, script Script, cfg config, h *process.FakeHandle) {
	// Stdin reader: continuously decode JSON values arriving on stdin.
	// Buffered enough that the agent's input feeder can write the initial
	// user message without blocking before the first Step needs it.
	stdinMsgs := make(chan json.RawMessage, 4)
	var stdinWG sync.WaitGroup
	stdinWG.Add(1)
	go func() {
		defer stdinWG.Done()
		dec := json.NewDecoder(h.StdinR)
		for {
			var raw json.RawMessage
			if err := dec.Decode(&raw); err != nil {
				close(stdinMsgs)
				return
			}
			if cfg.observe != nil {
				cfg.observe(raw)
			}
			select {
			case stdinMsgs <- raw:
			case <-ctx.Done():
				return
			}
		}
	}()

	// Drain the initial user prompt (sent by the agentboot Runner's input
	// feeder when spec.InitialInput is non-nil) before starting the script.
	// We give it a short window; if no prompt arrives, proceed anyway
	// (tests with empty initial input still work).
	select {
	case <-stdinMsgs:
		// consumed initial prompt
	case <-time.After(50 * time.Millisecond):
		// no initial prompt; continue
	case <-ctx.Done():
		h.FinishOutput()
		h.SignalExit(nil)
		return
	}

	defer func() {
		h.FinishOutput()
		h.SignalExit(nil)
	}()

	for i, step := range script {
		if cfg.stepDelay > 0 && i > 0 {
			select {
			case <-time.After(cfg.stepDelay):
			case <-ctx.Done():
				return
			}
		}

		bytes, err := step.encode()
		if err != nil {
			logrus.WithError(err).Warnf("fixture: step %d encode error", i)
			return
		}
		bytes = append(bytes, '\n')
		if _, err := h.WriteOutput(bytes); err != nil {
			return
		}

		if _, needs := step.needsResponse(); needs {
			select {
			case _, ok := <-stdinMsgs:
				if !ok {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}
}
