package agentboot

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot/protocol"
)

// Open starts the agent process and returns a [PersistentSession]: unlike
// Execute, the process stays alive after the first turn's terminal result,
// ready for further Send calls, until Close is called or the process exits
// unexpectedly. See .design/claude-code.md for the motivating design.
//
// prompt is the session's first turn — Open both starts the process and
// submits this prompt, exactly like Execute does for a one-shot run.
//
// Open only supports [OutputFormatStreamJSON]: that is the one wire format
// whose multi-turn behavior over a single stdin has been verified (see
// .design/claude-code.md §3.1).
func (r *Runner) Open(ctx context.Context, prompt string, opts ExecutionOptions) (PersistentSession, error) {
	r.mu.RLock()
	defaultFormat := r.defaultFormat
	eventBufferSize := r.eventBufferSize
	shutdownGracePeriod := r.shutdownGracePeriod
	r.mu.RUnlock()
	if opts.OutputFormat == "" {
		opts.OutputFormat = defaultFormat
	}
	if opts.OutputFormat == "" {
		opts.OutputFormat = OutputFormatStreamJSON
	}
	if opts.OutputFormat != OutputFormatStreamJSON {
		return nil, fmt.Errorf("agentboot: persistent sessions require stream-json output, got %q", opts.OutputFormat)
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
	transport.SetExecutionContext(ExecutionContext{
		SessionID: opts.SessionID,
		Metadata:  opts.ControlMetadata,
	})

	// Persistent sessions manage their own lifetime across many turns and
	// outlive the ctx that opened them: only Close ends one. Deriving
	// runCtx from ctx would be a serious bug, not just a Timeout question
	// — Execute's one-shot process is correctly scoped to its single
	// request's ctx, but a caller here is typically a per-message request
	// handler whose ctx is canceled the moment that one message finishes
	// processing. Tying the process (and, for the real OS factory,
	// exec.CommandContext's auto-kill) to that ctx would kill the process
	// before the *next* Send ever arrives — silently defeating persistence
	// on every second message. runCtx is therefore detached from ctx;
	// opts.Timeout is likewise not applied to the whole process here, since
	// that would kill a session that is legitimately idle between turns. A
	// caller that wants an idle timeout drives it externally via Close
	// (see agentboot/pool for the idle-timeout sweep that does this).
	runCtx, cancel := context.WithCancel(context.Background())

	logrus.Infof("runner.Open: starting %s (persistent)", r.driver.Type())
	proc, err := r.procFactory.Start(runCtx, *spec)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("start process: %w", err)
	}

	decoder := protocol.NewDecoder(proc.Stdout())
	encoder := protocol.NewEncoder(proc.Stdin())
	decoderEvents, decoderErr := decoder.Stream(runCtx)

	// process.Factory implementations must observe cancellation
	// consistently, including test/custom factories that do not use
	// exec.CommandContext.
	go func() {
		select {
		case <-runCtx.Done():
			_ = proc.Kill()
		case <-proc.Done():
		}
	}()

	s := &persistentSession{
		agentType:           r.driver.Type(),
		runCtx:              runCtx,
		cancel:              cancel,
		events:              make(chan StreamEvent, eventBufferSize),
		state:               SessionStateRunning, // Open's prompt is already the first in-flight turn
		pendingInput:        make(map[string]map[string]any),
		turnStart:           time.Now(),
		transport:           transport,
		encoder:             encoder,
		proc:                proc,
		shutdownGracePeriod: shutdownGracePeriod,
		done:                make(chan struct{}),
	}

	// Input feeder: delivers the first turn's message (built by the driver
	// into spec.InitialInput) via encoder. Every turn after that bypasses
	// this entirely — Send encodes directly onto the same encoder.
	if spec.InitialInput != nil {
		go func() {
			for {
				select {
				case m, ok := <-spec.InitialInput:
					if !ok {
						return
					}
					if err := encoder.Encode(m); err != nil {
						logrus.WithError(err).Debug("runner: persistent session input feeder encode error (process likely exited)")
						return
					}
				case <-runCtx.Done():
					return
				}
			}
		}()
	}

	go s.pump(decoderEvents, decoderErr)

	return s, nil
}
