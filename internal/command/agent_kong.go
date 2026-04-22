//go:build kong

package command

import ()

// AgentCmdKong is the Kong version of agent command
type AgentCmdKong struct {
	Apply AgentApplyCmdKong `kong:"cmd,help='Apply agent configuration'"`
	List  AgentListCmdKong  `kong:"cmd,help='List configured agents'"`
	Show  AgentShowCmdKong  `kong:"cmd,help='Show agent configuration details'"`
}

func (a *AgentCmdKong) Run(appManager *AppManager) error {
	// For now, call the existing cobra command
	cmd := AgentCommand(appManager)
	cmd.SetArgs([]string{})
	return cmd.Execute()
}

// AgentApplyCmdKong applies an agent configuration
type AgentApplyCmdKong struct {
	AgentType string `kong:"arg,optional,help='Agent type (claude-code, opencode, codex)'"`
	Provider  string `kong:"flag,name='provider',help='Provider UUID'"`
	Model     string `kong:"flag,name='model',help='Model name'"`
	Unified   bool   `kong:"flag,name='unified',default='true',help='Unified mode'"`
	Force     bool   `kong:"flag,name='force',help='Skip confirmation'"`
	Preview   bool   `kong:"flag,name='preview',help='Preview without applying'"`
}

func (a *AgentApplyCmdKong) Run(appManager *AppManager) error {
	cmd := AgentCommand(appManager)
	args := []string{"apply"}
	if a.AgentType != "" {
		args = append(args, a.AgentType)
	}
	if a.Provider != "" {
		args = append(args, "--provider", a.Provider)
	}
	if a.Model != "" {
		args = append(args, "--model", a.Model)
	}
	if a.Unified {
		args = append(args, "--unified=true")
	}
	if a.Force {
		args = append(args, "--force")
	}
	if a.Preview {
		args = append(args, "--preview")
	}
	cmd.SetArgs(args)
	return cmd.Execute()
}

// AgentListCmdKong lists configured agents
type AgentListCmdKong struct{}

func (a *AgentListCmdKong) Run(appManager *AppManager) error {
	cmd := AgentCommand(appManager)
	cmd.SetArgs([]string{"list"})
	return cmd.Execute()
}

// AgentShowCmdKong shows agent configuration details
type AgentShowCmdKong struct {
	AgentType string `kong:"arg,optional,help='Agent type to show'"`
}

func (a *AgentShowCmdKong) Run(appManager *AppManager) error {
	cmd := AgentCommand(appManager)
	args := []string{"show"}
	if a.AgentType != "" {
		args = append(args, a.AgentType)
	}
	cmd.SetArgs(args)
	return cmd.Execute()
}
