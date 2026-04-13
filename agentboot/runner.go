package agentboot

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot/mitm"
)

// Runner is a generic agent executor that composes an AgentDriver (process setup)
// with an AgentTransport (protocol) to implement the Agent interface.
type Runner struct {
	mu            sync.RWMutex
	driver        AgentDriver
	transport     AgentTransport
	defaultFormat OutputFormat
}

// NewRunner creates a Runner from a driver and transport.
func NewRunner(driver AgentDriver, transport AgentTransport) *Runner {
	return &Runner{
		driver:        driver,
		transport:     transport,
		defaultFormat: OutputFormatStreamJSON,
	}
}

// Type returns the agent type from the underlying driver.
func (r *Runner) Type() AgentType { return r.driver.Type() }

// IsAvailable delegates to the driver.
func (r *Runner) IsAvailable() bool { return r.driver.IsAvailable() }

// SetDefaultFormat sets the default output format.
func (r *Runner) SetDefaultFormat(f OutputFormat) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defaultFormat = f
}

// GetDefaultFormat returns the default output format.
func (r *Runner) GetDefaultFormat() OutputFormat {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.defaultFormat == "" {
		return OutputFormatText
	}
	return r.defaultFormat
}

// Execute runs the agent and blocks until completion.
//
//   - If opts.Handler is set, messages are streamed to the handler and (nil, err)
//     is returned — the handler is responsible for collecting results.
//   - Otherwise a ResultCollector is used internally and the collected result is
//     returned when execution finishes.
func (r *Runner) Execute(ctx context.Context, prompt string, opts ExecutionOptions) (*Result, error) {
	r.mu.RLock()
	defaultFormat := r.defaultFormat
	r.mu.RUnlock()

	if opts.OutputFormat == "" {
		opts.OutputFormat = defaultFormat
	}
	if opts.OutputFormat == "" {
		opts.OutputFormat = OutputFormatStreamJSON
	}

	timeout := opts.Timeout
	logrus.Infof("runner.Execute: starting %s agent", r.driver.Type())

	if opts.Handler != nil {
		err := r.run(ctx, prompt, timeout, opts, opts.Handler)
		return nil, err
	}

	return r.runCollect(ctx, prompt, timeout, opts)
}

// runCollect runs execution with an internal ResultCollector.
func (r *Runner) runCollect(ctx context.Context, prompt string, timeout time.Duration, opts ExecutionOptions) (*Result, error) {
	start := time.Now()

	if !r.driver.IsAvailable() {
		return &Result{Error: "agent CLI not found", Format: opts.OutputFormat}, exec.ErrNotFound
	}

	collector := newResultCollector()
	if err := r.run(ctx, prompt, timeout, opts, collector); err != nil {
		result := collector.result()
		result.Duration = time.Since(start)
		return result, err
	}

	result := collector.result()
	result.Duration = time.Since(start)
	if result.Error != "" {
		return result, errors.New(result.Error)
	}
	return result, nil
}

// run is the core execution loop.
func (r *Runner) run(
	ctx context.Context,
	prompt string,
	timeout time.Duration,
	opts ExecutionOptions,
	handler MessageHandler,
) error {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	if !r.driver.IsAvailable() {
		return exec.ErrNotFound
	}

	spec, err := r.driver.Prepare(ctx, prompt, opts)
	if err != nil {
		return fmt.Errorf("prepare launch spec: %w", err)
	}

	cmd := spec.BuildCmd(ctx)

	mitmRunner := mitm.New(cmd, nil, nil)
	mitmRunner.Codec = mitm.CodecJSON

	// Feed initial input from the spec's bootstrap channel.
	inputSource := mitm.NewChanSource(100)
	feederDone := make(chan struct{})
	go func() {
		defer close(feederDone)
		if spec.InitialInput == nil {
			return
		}
		for {
			select {
			case m, ok := <-spec.InitialInput:
				if !ok {
					return
				}
				inputSource.Write(m)
			case <-ctx.Done():
				return
			}
		}
	}()
	mitmRunner.InputSource = inputSource
	defer inputSource.Close()

	var processIntentionallyKilled bool

	write := func(msg any) error {
		return inputSource.WriteWait(ctx, msg)
	}

	mitmRunner.OutputHandler = func(octx context.Context, c *mitm.IOContext) (*mitm.OutputResult, error) {
		ae, decErr := r.transport.Decode(c.Msg)
		if decErr != nil {
			return nil, fmt.Errorf("decode event: %w", decErr)
		}

		msgs, isTerminal, success := r.transport.AccumulateAndForward(ae)

		switch {
		case ae.IsControl:
			if hErr := r.transport.HandleControl(octx, ae, handler, write); hErr != nil {
				logrus.WithError(hErr).Error("runner: control handler error")
			}

		case isTerminal:
			handler.OnComplete(&CompletionResult{Success: success})
			processIntentionallyKilled = true
			_ = cmd.Process.Kill()
			logrus.Debugf("runner: killed process %d after result event", cmd.Process.Pid)
			return &mitm.OutputResult{Action: mitm.Stop}, nil

		default:
			for _, msg := range msgs {
				if msg == nil {
					continue
				}
				if hErr := handler.OnMessage(msg); hErr != nil {
					handler.OnError(hErr)
				}
			}
		}

		return &mitm.OutputResult{Action: mitm.Pass}, nil
	}

	runErr := mitmRunner.Run(ctx)
	<-feederDone

	if runErr != nil && processIntentionallyKilled {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			if exitErr.ProcessState != nil && !exitErr.ProcessState.Success() {
				logrus.Debugf("runner: process terminated after intentional kill (expected)")
				return nil
			}
		}
	}

	return runErr
}

// --- internal ResultCollector -----------------------------------------------

type resultCollector struct {
	mu     sync.Mutex
	events []Event
	errStr string
}

func newResultCollector() *resultCollector { return &resultCollector{} }

func (c *resultCollector) OnMessage(msg interface{}) error {
	if e, ok := msg.(Event); ok {
		c.mu.Lock()
		c.events = append(c.events, e)
		c.mu.Unlock()
	}
	return nil
}

func (c *resultCollector) OnError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.errStr = err.Error()
}

func (c *resultCollector) OnComplete(r *CompletionResult) {
	if r != nil && !r.Success && r.Error != "" {
		c.mu.Lock()
		c.errStr = r.Error
		c.mu.Unlock()
	}
}

func (c *resultCollector) OnApproval(_ context.Context, _ PermissionRequest) (PermissionResult, error) {
	return PermissionResult{Approved: true}, nil
}

func (c *resultCollector) OnAsk(_ context.Context, req AskRequest) (AskResult, error) {
	return AskResult{ID: req.ID, Approved: true}, nil
}

func (c *resultCollector) result() *Result {
	c.mu.Lock()
	defer c.mu.Unlock()
	return &Result{
		Format: OutputFormatStreamJSON,
		Events: c.events,
		Error:  c.errStr,
	}
}
