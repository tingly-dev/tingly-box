package command

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/agent"
)

// AgentCmdKong is the Kong version of agent command with flag-based operations.
// The default behavior (no subcommand) is to list agents.
type AgentCmdKong struct {
	// Flag-based operations (primary interface)
	List    AgentListFlagCmdKong    `kong:"cmd,name='list',default='1',hidden,help='List configured agents (default)'"`
	Apply   AgentApplyFlagCmdKong   `kong:"cmd,help='Apply agent configuration'"`
	Show    AgentShowFlagCmdKong    `kong:"cmd,help='Show agent configuration details'"`
	Restore AgentRestoreFlagCmdKong `kong:"cmd,help='Restore agent configuration from backup'"`
}

// AgentListFlagCmdKong lists configured agents (default behavior)
type AgentListFlagCmdKong struct{}

func (a *AgentListFlagCmdKong) Run(appManager *AppManager) error {
	return listAgentTypes()
}

// AgentApplyFlagCmdKong applies agent configuration via flags
type AgentApplyFlagCmdKong struct {
	AgentType  string `kong:"arg,optional,help='Agent type (claude-code, opencode, codex)'"`
	Provider   string `kong:"flag,name='provider',help='Provider UUID'"`
	Model      string `kong:"flag,name='model',help='Model name'"`
	Unified    bool   `kong:"flag,name='unified',default='true',help='Unified mode (claude-code only)'"`
	StatusLine bool   `kong:"flag,name='status-line',help='Install status line integration (claude-code only)'"`
	Force      bool   `kong:"flag,name='force',help='Skip confirmation'"`
	Preview    bool   `kong:"flag,name='preview',help='Preview without applying'"`
}

func (a *AgentApplyFlagCmdKong) Run(appManager *AppManager) error {
	var req agent.ApplyAgentRequest
	req.AgentType = agent.AgentType(a.AgentType)
	req.Provider = a.Provider
	req.Model = a.Model
	req.Unified = a.Unified
	req.InstallStatusLine = a.StatusLine
	req.Force = a.Force
	req.Preview = a.Preview

	reader := bufio.NewReader(os.Stdin)

	// Handle agent type: empty vs invalid vs valid
	if a.AgentType == "" {
		// No agent type specified, prompt for selection
		agentType, err := promptForAgentTypeChoice(reader)
		if err != nil {
			return err
		}
		req.AgentType = agentType
	} else if !req.AgentType.IsValid() {
		// Invalid agent type provided - fail fast
		fmt.Fprintf(os.Stderr, "Unknown agent type: %s\n\n", a.AgentType)
		fmt.Fprintln(os.Stderr, "Available agent types:")
		fmt.Fprintln(os.Stderr, "  claude-code - Claude Code CLI agent (@cc)")
		fmt.Fprintln(os.Stderr, "  opencode   - OpenCode editor agent (@oc)")
		fmt.Fprintln(os.Stderr, "  codex      - Codex agent (@cx)")
		return fmt.Errorf("unknown agent type: %s", a.AgentType)
	}

	// Interactive prompts if provider/model not specified
	if req.Provider == "" || req.Model == "" {
		if err := promptForAgentConfig(reader, appManager, &req); err != nil {
			return err
		}
	}

	// Show preview if requested
	if req.Preview {
		return showPreview(appManager, &req)
	}

	// Confirm if not forced
	if !req.Force {
		if err := confirmApply(reader, &req); err != nil {
			return err
		}
	}

	// Apply configuration
	return executeAgentApply(appManager, &req)
}

// AgentShowFlagCmdKong shows agent configuration details via flags
type AgentShowFlagCmdKong struct {
	AgentType string `kong:"arg,optional,help='Agent type to show'"`
}

func (a *AgentShowFlagCmdKong) Run(appManager *AppManager) error {
	reader := bufio.NewReader(os.Stdin)

	// Handle agent type: empty vs invalid vs valid
	if a.AgentType == "" {
		// No agent type specified, prompt for selection
		agentType, err := promptForAgentTypeChoice(reader)
		if err != nil {
			return err
		}
		return showAgentConfig(appManager, agentType)
	}

	agentType := agent.AgentType(a.AgentType)
	if !agentType.IsValid() {
		// Invalid agent type provided - fail fast
		fmt.Fprintf(os.Stderr, "Unknown agent type: %s\n\n", a.AgentType)
		return fmt.Errorf("unknown agent type: %s", a.AgentType)
	}

	return showAgentConfig(appManager, agentType)
}

// AgentRestoreFlagCmdKong restores agent configuration from backup
type AgentRestoreFlagCmdKong struct {
	AgentType string `kong:"arg,optional,help='Agent type to restore'"`
	Force     bool   `kong:"flag,name='force',help='Skip confirmation prompt'"`
}

func (a *AgentRestoreFlagCmdKong) Run(appManager *AppManager) error {
	var req agent.RestoreAgentRequest
	req.AgentType = agent.AgentType(a.AgentType)
	req.Force = a.Force

	reader := bufio.NewReader(os.Stdin)

	// Handle agent type: empty vs invalid vs valid
	if a.AgentType == "" {
		// No agent type specified, prompt for selection
		agentType, err := promptForAgentTypeChoice(reader)
		if err != nil {
			return err
		}
		req.AgentType = agentType
	} else if !req.AgentType.IsValid() {
		// Invalid agent type provided - fail fast
		fmt.Fprintf(os.Stderr, "Unknown agent type: %s\n\n", a.AgentType)
		return fmt.Errorf("unknown agent type: %s", a.AgentType)
	}

	info, ok := agent.GetAgentInfo(req.AgentType)
	if !ok {
		return fmt.Errorf("no info registered for agent type: %s", req.AgentType)
	}

	if !req.Force {
		fmt.Println("\nFiles that will be restored from their most recent backup:")
		for _, f := range info.ConfigFiles {
			fmt.Printf("  - %s\n", f)
		}
		fmt.Print("\nProceed? [y/N]: ")
		input, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		input = strings.TrimSpace(strings.ToLower(input))
		if input != "y" && input != "yes" {
			return fmt.Errorf("cancelled by user")
		}
	}

	return executeAgentRestore(appManager, &req)
}
