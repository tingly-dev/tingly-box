package agentboot

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot/common"
	"github.com/tingly-dev/tingly-box/agentboot/protocol"
)

// Execute starts the agent and returns an [ExecutionHandle] for the caller
// to consume events from and respond to control requests via.
//
// The returned handle's Events channel closes after the underlying process
// has exited and all decoded events have been delivered. Wait then returns
// the aggregated Result.
func (r *Runner) Execute(ctx context.Context, prompt string, opts ExecutionOptions) (ExecutionHandle, error) {
	r.mu.RLock()
	defaultFormat := r.defaultFormat
	eventBufferSize := r.eventBufferSize
	defaultTimeout := r.defaultTimeout
	shutdownGracePeriod := r.shutdownGracePeriod
	r.mu.RUnlock()
	if opts.OutputFormat == "" {
		opts.OutputFormat = defaultFormat
	}
	if opts.OutputFormat == "" {
		opts.OutputFormat = OutputFormatStreamJSON
	}
	if opts.Timeout == 0 {
		opts.Timeout = defaultTimeout
	}

	if !r.driver.IsAvailable() {
		return nil, errors.New("agent CLI not available")
	}

	spec, err := r.driver.Prepare(ctx, prompt, opts)
	if err != nil {
		return nil, fmt.Errorf("prepare launch spec: %w", err)
	}
	if len(spec.Command) == 0 {
		return nil, errors.New("empty launch command")
	}

	if r.transportFactory == nil {
		return nil, errors.New("agentboot: transport factory is nil")
	}
	transport := r.transportFactory()
	if transport == nil {
		return nil, errors.New("agentboot: transport factory returned nil")
	}
	transport.SetExecutionContext(opts.SessionID, opts.ChatID, opts.Platform, opts.BotUUID)

	runCtx, cancel := context.WithCancel(ctx)
	if opts.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(runCtx, opts.Timeout)
	}

	logrus.Infof("runner.Execute: starting %s", r.driver.Type())
	proc, err := r.procFactory.Start(runCtx, *spec)
	if err != nil {
		cancel()
		if opts.Store != nil && opts.SessionID != "" {
			opts.Store.SetFailed(opts.SessionID, err.Error())
		}
		return nil, fmt.Errorf("start process: %w", err)
	}
	if opts.Store != nil && opts.SessionID != "" {
		opts.Store.SetRunning(opts.SessionID)
	}

	decoder := protocol.NewDecoder(proc.Stdout())
	encoder := protocol.NewEncoder(proc.Stdin())
	decoderEvents, decoderErr := decoder.Stream(runCtx)

	// process.Factory implementations must observe cancellation consistently,
	// including test/custom factories that do not use exec.CommandContext.
	go func() {
		select {
		case <-runCtx.Done():
			_ = proc.Kill()
		case <-proc.Done():
		}
	}()

	// A result is terminal, but Claude Code may still need to flush its session
	// file. End stdin first, then escalate to Kill only if it does not exit
	// within the grace period.
	var shutdownOnce sync.Once
	var shutdownWG sync.WaitGroup
	shutdownGracefully := func() {
		shutdownOnce.Do(func() {
			// Close may briefly wait for an in-flight encoder write. Keep the
			// pump draining stdout while shutdown proceeds.
			shutdownWG.Add(2)
			go func() {
				defer shutdownWG.Done()
				_ = encoder.Close()
			}()
			go func() {
				defer shutdownWG.Done()
				timer := time.NewTimer(shutdownGracePeriod)
				defer timer.Stop()
				select {
				case <-proc.Done():
				case <-timer.C:
					_ = proc.Kill()
				}
			}()
		})
	}

	startTime := time.Now()

	state := &runState{
		opts:         opts,
		startTime:    startTime,
		pendingInput: make(map[string]map[string]any),
	}

	var handle *runnerHandle
	handle = newRunnerHandle(
		eventBufferSize,
		// responseFn: encode and write the control response.
		func(reqID string, resp ControlResponse) error {
			state.mu.Lock()
			input, ok := state.pendingInput[reqID]
			if ok {
				delete(state.pendingInput, reqID)
			}
			state.mu.Unlock()

			wire := transport.EncodeControlResponse(reqID, resp, input)
			if wire == nil {
				return errors.New("transport produced nil control response")
			}
			return encoder.Encode(wire)
		},
		// cancelFn: cancel the run context. The pump goroutine will see
		// ctx.Done(), kill the process, and close the stream.
		cancel,
		// waitFn: built below after we know the wg.
		nil, // placeholder, filled in below
	)

	// Input feeder: consumes spec.InitialInput and writes via encoder.
	var feederWG sync.WaitGroup
	if spec.InitialInput != nil {
		feederWG.Add(1)
		go func() {
			defer feederWG.Done()
			for {
				select {
				case m, ok := <-spec.InitialInput:
					if !ok {
						return
					}
					if err := encoder.Encode(m); err != nil {
						logrus.WithError(err).Debug("runner: input feeder encode error (process likely exited)")
						return
					}
				case <-runCtx.Done():
					return
				}
			}
		}()
	}

	// Pump goroutine: classify decoded events, emit to handle, observe terminal.
	var pumpWG sync.WaitGroup
	pumpWG.Add(1)
	go func() {
		defer pumpWG.Done()
		defer handle.closeStream()

		for ev := range decoderEvents {
			// Always append the raw event to result.Events for back-compat
			// helpers (TextOutput, GetAssistantMessages, GetSessionID, …).
			state.mu.Lock()
			state.events = append(state.events, ev)
			state.mu.Unlock()

			kind, parsed := transport.Classify(ev)

			switch kind {
			case EventKindIgnore:
				// Drop.

			case EventKindMessage:
				msgs := transport.AccumulateMessage(ev)
				for _, m := range msgs {
					handle.emit(runCtx, MessageEvent{Raw: m})
				}

			case EventKindControl:
				// Stash the original input so EncodeControlResponse can use
				// it later when no UpdatedInput is supplied by the consumer.
				if parsed != nil {
					var (
						reqID string
						input map[string]any
					)
					switch p := parsed.(type) {
					case ApprovalRequestEvent:
						reqID = p.ID
						input = p.Input
					case AskRequestEvent:
						reqID = p.ID
						input = p.Input
					}
					if reqID != "" {
						state.mu.Lock()
						state.pendingInput[reqID] = input
						state.mu.Unlock()
					}
					handle.emit(runCtx, parsed)
				}

			case EventKindTerminalSuccess:
				state.mu.Lock()
				state.terminalSeen = true
				state.mu.Unlock()
				shutdownGracefully()

			case EventKindTerminalError:
				state.mu.Lock()
				state.terminalSeen = true
				state.terminalErr = resultErrorFromEvent(r.driver.Type(), ev)
				state.mu.Unlock()
				shutdownGracefully()
			}
		}

		dErr := decoderErr()
		state.mu.Lock()
		terminalSeen := state.terminalSeen
		state.mu.Unlock()

		// A fatal decoder error means the read side can no longer make
		// progress. Likewise, stream-json EOF without a result has no valid
		// completion to wait for. Stop the process before joining it so a
		// malformed or truncated stream cannot leave Wait blocked forever.
		fatalDecode := dErr != nil &&
			!errors.Is(dErr, context.Canceled) &&
			!errors.Is(dErr, context.DeadlineExceeded)
		if fatalDecode || (opts.OutputFormat == OutputFormatStreamJSON && !terminalSeen) {
			_ = proc.Kill()
		}

		// Wait for the process to actually exit before considering execution
		// done — this enforces the OnComplete-after-exit invariant.
		processWaitErr := proc.Wait()
		shutdownWG.Wait()

		state.mu.Lock()
		if processWaitErr != nil {
			state.processErr = newProcessError(r.driver.Type(), processWaitErr)
			state.exitCode = state.processErr.ExitCode
		}
		if dErr != nil && !errors.Is(dErr, context.Canceled) && !errors.Is(dErr, context.DeadlineExceeded) {
			state.protocolErr = &ProtocolError{AgentType: r.driver.Type(), Err: dErr}
		}
		processErr := state.processErr
		protocolErr := state.protocolErr
		terminalErr := state.terminalErr
		state.mu.Unlock()

		// Match the SDK's error-in-stream behavior while keeping Wait as the
		// authoritative terminal outcome.
		switch {
		case protocolErr != nil:
			handle.emit(runCtx, ErrorEvent{Err: protocolErr})
		case terminalErr == nil && processErr != nil:
			handle.emit(runCtx, ErrorEvent{Err: processErr})
		}
	}()

	// Build the waitFn now that pumpWG / feederWG are spawned.
	var waitOnce sync.Once
	var waitResult *Result
	var waitErr error
	waitFn := func() (*Result, error) {
		waitOnce.Do(func() {
			pumpWG.Wait()
			feederWG.Wait()
			runCtxErr := runCtx.Err()
			cancel()

			// Compute result under state lock, then release before calling store
			// (the store has its own lock; holding both would invert lock order).
			var sessID string
			var store = opts.Store
			func() {
				state.mu.Lock()
				defer state.mu.Unlock()

				waitResult = &Result{
					ExitCode: state.exitCode,
					Format:   opts.OutputFormat,
					Events:   append([]common.Event(nil), state.events...),
					Duration: time.Since(state.startTime),
					Metadata: map[string]any{},
				}
				if state.opts.SessionID != "" {
					waitResult.Metadata["session_id"] = state.opts.SessionID
				}
				switch {
				case state.terminalErr != nil:
					waitErr = state.terminalErr
				case runCtxErr != nil:
					waitErr = runCtxErr
				case state.protocolErr != nil:
					waitErr = state.protocolErr
				case state.processErr != nil:
					waitErr = state.processErr
				case opts.OutputFormat == OutputFormatStreamJSON && !state.terminalSeen:
					waitErr = &ProtocolError{
						AgentType: r.driver.Type(),
						Err:       ErrNoTerminalResult,
					}
				}
				if waitErr != nil {
					waitResult.Error = waitErr.Error()
				}
				sessID = state.opts.SessionID
			}()

			// Notify the session store outside the state lock.
			if store != nil && sessID != "" {
				if waitErr != nil {
					store.SetFailed(sessID, waitErr.Error())
				} else {
					store.SetCompleted(sessID, "")
				}
			}
		})
		return waitResult, waitErr
	}
	handle.waitFn = waitFn

	return handle, nil
}
