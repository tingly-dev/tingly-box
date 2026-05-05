package agentboot

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot/common"
	"github.com/tingly-dev/tingly-box/agentboot/process"
	"github.com/tingly-dev/tingly-box/agentboot/protocol"
)

// Runner is a generic agent executor that composes an [AgentDriver] (process
// setup) with an [AgentTransport] (protocol parsing) and a [process.Factory]
// (process supervision) to implement the [Agent] interface.
//
// The default Runner uses [process.NewOSExecFactory] to spawn real processes.
// Tests inject [process.NewFakeFactory] via [NewRunnerWithFactory] to
// substitute the binary while exercising the same driver and transport.
type Runner struct {
	mu            sync.RWMutex
	driver        AgentDriver
	transport     AgentTransport
	procFactory   process.Factory
	defaultFormat OutputFormat
}

// NewRunner creates a Runner backed by [process.NewOSExecFactory].
func NewRunner(driver AgentDriver, transport AgentTransport) *Runner {
	return NewRunnerWithFactory(driver, transport, process.NewOSExecFactory())
}

// NewRunnerWithFactory creates a Runner with a custom process factory. Use
// [process.NewFakeFactory] in tests.
func NewRunnerWithFactory(driver AgentDriver, transport AgentTransport, factory process.Factory) *Runner {
	return &Runner{
		driver:        driver,
		transport:     transport,
		procFactory:   factory,
		defaultFormat: OutputFormatStreamJSON,
	}
}

func (r *Runner) Type() AgentType   { return r.driver.Type() }
func (r *Runner) IsAvailable() bool { return r.driver.IsAvailable() }

func (r *Runner) SetDefaultFormat(f OutputFormat) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defaultFormat = f
}

func (r *Runner) GetDefaultFormat() OutputFormat {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.defaultFormat == "" {
		return OutputFormatText
	}
	return r.defaultFormat
}

// Execute starts the agent and returns an [ExecutionHandle] for the caller
// to consume events from and respond to control requests via.
//
// The returned handle's Events channel closes after the underlying process
// has exited and all decoded events have been delivered. Wait then returns
// the aggregated Result.
func (r *Runner) Execute(ctx context.Context, prompt string, opts ExecutionOptions) (ExecutionHandle, error) {
	r.mu.RLock()
	defaultFormat := r.defaultFormat
	r.mu.RUnlock()
	if opts.OutputFormat == "" {
		opts.OutputFormat = defaultFormat
	}
	if opts.OutputFormat == "" {
		opts.OutputFormat = OutputFormatStreamJSON
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

	r.transport.SetExecutionContext(opts.SessionID, opts.ChatID, opts.Platform, opts.BotUUID)

	runCtx, cancel := context.WithCancel(ctx)
	if opts.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(runCtx, opts.Timeout)
	}

	procSpec := process.LaunchSpec{
		Path:    spec.Command[0],
		Args:    spec.Command[1:],
		Env:     spec.Env,
		WorkDir: spec.WorkDir,
	}

	logrus.Infof("runner.Execute: starting %s", r.driver.Type())
	proc, err := r.procFactory.Start(runCtx, procSpec)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("start process: %w", err)
	}

	decoder := protocol.NewDecoder(proc.Stdout())
	encoder := protocol.NewEncoder(proc.Stdin())
	decoderEvents, decoderErr := decoder.Stream(runCtx)

	startTime := time.Now()

	state := &runState{
		opts:         opts,
		startTime:    startTime,
		pendingInput: make(map[string]map[string]any),
	}

	var handle *runnerHandle
	handle = newRunnerHandle(
		16,
		// responseFn: encode and write the control response.
		func(reqID string, resp ControlResponse) error {
			state.mu.Lock()
			input, ok := state.pendingInput[reqID]
			if ok {
				delete(state.pendingInput, reqID)
			}
			state.mu.Unlock()

			wire := r.transport.EncodeControlResponse(reqID, resp, input)
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

			kind, parsed := r.transport.Classify(ev)

			switch kind {
			case EventKindIgnore:
				// Drop.

			case EventKindMessage:
				msgs := r.transport.AccumulateMessage(ev)
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
				state.terminalSuccess = true
				state.mu.Unlock()
				_ = proc.Kill()
				// Continue draining; decoder loop ends after EOF.

			case EventKindTerminalError:
				state.mu.Lock()
				state.terminalSeen = true
				state.terminalSuccess = false
				state.mu.Unlock()
				_ = proc.Kill()
			}
		}

		// Decoder closed (EOF, ctx, or error). Wait for the process to actually
		// exit before considering execution done — this enforces the
		// OnComplete-after-exit invariant the old runner violated.
		_ = proc.Wait()

		// Surface any decoder-level error as a tail ErrorEvent (non-fatal).
		if dErr := decoderErr(); dErr != nil && !errors.Is(dErr, context.Canceled) {
			handle.emit(runCtx, ErrorEvent{Err: dErr})
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
			cancel()

			state.mu.Lock()
			defer state.mu.Unlock()

			waitResult = &Result{
				Format:   opts.OutputFormat,
				Events:   append([]common.Event(nil), state.events...),
				Duration: time.Since(state.startTime),
				Metadata: map[string]any{},
			}
			if state.opts.SessionID != "" {
				waitResult.Metadata["session_id"] = state.opts.SessionID
			}
			if state.terminalSeen && !state.terminalSuccess {
				waitResult.Error = "agent reported error result"
				waitErr = errors.New(waitResult.Error)
			}
			if rctxErr := runCtx.Err(); rctxErr != nil && !state.terminalSeen {
				if errors.Is(rctxErr, context.DeadlineExceeded) {
					waitResult.Error = "agent execution timed out"
					waitErr = context.DeadlineExceeded
				} else if errors.Is(rctxErr, context.Canceled) && ctx.Err() != nil {
					waitErr = ctx.Err()
				}
			}
		})
		return waitResult, waitErr
	}
	handle.waitFn = waitFn

	return handle, nil
}

// runState is the shared accumulator driven by the pump goroutine.
type runState struct {
	mu sync.Mutex

	opts            ExecutionOptions
	startTime       time.Time
	events          []common.Event
	pendingInput    map[string]map[string]any
	terminalSeen    bool
	terminalSuccess bool
}
