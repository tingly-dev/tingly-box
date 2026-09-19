package agentboot

import (
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot/process"
)

// Runner is a generic agent executor that composes an [AgentDriver] (process
// setup) with an [AgentTransport] (protocol parsing) and a [process.Factory]
// (process supervision) to implement the [Agent] interface.
//
// The default Runner uses [process.NewOSExecFactory] to spawn real processes.
// Tests inject [process.NewFakeFactory] via [NewRunnerWithFactory] to
// substitute the binary while exercising the same driver and transport.
type Runner struct {
	mu                  sync.RWMutex
	driver              AgentDriver
	transportFactory    AgentTransportFactory
	procFactory         process.Factory
	defaultFormat       OutputFormat
	eventBufferSize     int
	defaultTimeout      time.Duration
	shutdownGracePeriod time.Duration
}

const (
	defaultEventBufferSize     = 100
	defaultShutdownGracePeriod = 5 * time.Second
)

// RunnerConfig controls execution defaults owned by a Runner. Per-call values
// in [ExecutionOptions] still take precedence.
type RunnerConfig struct {
	DefaultFormat           OutputFormat
	EventBufferSize         int
	DefaultExecutionTimeout time.Duration
	ShutdownGracePeriod     time.Duration
}

func normalizeRunnerConfig(config RunnerConfig) RunnerConfig {
	if config.DefaultFormat == "" {
		config.DefaultFormat = OutputFormatStreamJSON
	}
	if config.EventBufferSize <= 0 {
		config.EventBufferSize = defaultEventBufferSize
	}
	if config.ShutdownGracePeriod <= 0 {
		config.ShutdownGracePeriod = defaultShutdownGracePeriod
	}
	return config
}

// NewRunnerWithConfig creates an OS-backed Runner with explicit defaults.
func NewRunnerWithConfig(driver AgentDriver, transportFactory AgentTransportFactory, config RunnerConfig) *Runner {
	return NewRunnerWithFactoryAndConfig(driver, transportFactory, process.NewOSExecFactory(), config)
}

// NewRunnerWithFactoryAndConfig creates a Runner with both a custom process
// factory and explicit execution defaults.
func NewRunnerWithFactoryAndConfig(driver AgentDriver, transportFactory AgentTransportFactory, factory process.Factory, config RunnerConfig) *Runner {
	config = normalizeRunnerConfig(config)
	return &Runner{
		driver:              driver,
		transportFactory:    transportFactory,
		procFactory:         factory,
		defaultFormat:       config.DefaultFormat,
		eventBufferSize:     config.EventBufferSize,
		defaultTimeout:      config.DefaultExecutionTimeout,
		shutdownGracePeriod: config.ShutdownGracePeriod,
	}
}

func (r *Runner) Type() AgentType   { return r.driver.Type() }
func (r *Runner) IsAvailable() bool { return r.driver.IsAvailable() }

func (r *Runner) SetDefaultFormat(f OutputFormat) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defaultFormat = f
}
