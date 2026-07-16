package codex

import (
	"context"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/process"
)

type Agent struct {
	runner *agentboot.Runner
	driver *Driver
}

func NewAgent(config Config) *Agent {
	driver := NewDriver(config)
	return &Agent{runner: agentboot.NewRunner(driver, NewTransport()), driver: driver}
}

func NewAgentWithFactory(config Config, factory process.Factory) *Agent {
	driver := NewDriver(config)
	driver.SetForceAvailable(true)
	return &Agent{runner: agentboot.NewRunnerWithFactory(driver, NewTransport(), factory), driver: driver}
}

func (a *Agent) Execute(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (agentboot.ExecutionHandle, error) {
	return a.runner.Execute(ctx, prompt, opts)
}

func (a *Agent) IsAvailable() bool                              { return a.driver.IsAvailable() }
func (*Agent) Type() agentboot.AgentType                        { return agentboot.AgentTypeCodex }
func (a *Agent) SetDefaultFormat(format agentboot.OutputFormat) { a.runner.SetDefaultFormat(format) }
func (a *Agent) GetDefaultFormat() agentboot.OutputFormat       { return a.runner.GetDefaultFormat() }
func (a *Agent) SetCLIPath(path string)                         { a.driver.SetCLIPath(path) }
