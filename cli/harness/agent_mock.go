package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocoltest"
)

// runVirtualAgentTest executes a virtual-model e2e test by running the actual
// agent CLI command against an in-process gateway wired to a mock upstream.
// It returns a slice of results (always length 1) so every caller — single-agent
// or batch — sees the same structured shape as the real-provider path.
//
// If writer is non-nil, the produced result row is appended immediately. If
// the (agent, "mock") key is already present in skip, the run is skipped and
// an empty slice is returned with a message.
func runVirtualAgentTest(agentName string, prompt string, writer *summaryWriter, skip map[resumeKey]struct{}) ([]*RealAgentTestResult, error) {
	if prompt == "" {
		prompt = defaultPrompts[agentName]
	}

	agentType := parseAgentType(agentName)
	if agentType == "" {
		return nil, fmt.Errorf("unknown agent: %q (available: claude, codex, opencode)", agentName)
	}

	if _, ok := skip[resumeKey{Agent: agentName, Entry: "mock"}]; ok {
		fmt.Printf("⏭  Skipping (resume) %s / mock\n\n", agentName)
		return nil, nil
	}

	fmt.Printf("🧪 Virtual-model test: %s\n", agentName)
	fmt.Printf("📝 Prompt: %s\n", prompt)
	fmt.Println()

	start := time.Now()
	inner, err := executeAgentCommand(agentType, prompt)

	// Wrap the agent-CLI outcome in the unified result shape.
	r := &RealAgentTestResult{
		EntryName:    "mock",
		Agent:        agentName,
		APIStyle:     virtualAPIStyle(agentType),
		Prompt:       prompt,
		Model:        protocoltest.BuiltinRequestModel(agentType),
		RequestModel: protocoltest.BuiltinRequestModel(agentType),
	}
	if err != nil {
		r.Duration = time.Since(start)
		r.Error = err.Error()
		fmt.Printf("❌ Execution failed: %v\n", err)
		printRealAgentTestResult(r)
		fmt.Println()
		if writer != nil {
			if aerr := writer.Append(r); aerr != nil {
				fmt.Printf("⚠️  summary append failed: %v\n", aerr)
			}
		}
		return []*RealAgentTestResult{r}, nil
	}

	r.Success = inner.Success
	r.TimedOut = inner.TimedOut
	r.Output = inner.Output
	r.Error = inner.Error
	r.ExitCode = inner.ExitCode
	r.Duration = inner.Duration

	// The virtual upstream's answer is fixed, so a zero exit code alone is not
	// success: the CLI's output must carry the round-trip content. This catches
	// a CLI that prints an error (or drops the response) yet exits cleanly.
	if r.Success && !strings.Contains(r.Output, protocoltest.VirtualMockAnswerMarker) {
		r.Success = false
		r.Error = fmt.Sprintf("agent output does not contain the mock answer marker %q — the gateway round trip's content never reached the CLI output", protocoltest.VirtualMockAnswerMarker)
	}

	printRealAgentTestResult(r)
	fmt.Println()
	if writer != nil {
		if aerr := writer.Append(r); aerr != nil {
			fmt.Printf("⚠️  summary append failed: %v\n", aerr)
		}
	}
	return []*RealAgentTestResult{r}, nil
}

// virtualAPIStyle returns the API style used by the virtual upstream for a given agent.
// It mirrors the branches in AgentTestEnv.SetupAgent.
func virtualAPIStyle(agentType protocoltest.AgentType) string {
	switch agentType {
	case protocoltest.AgentTypeClaudeCode, protocoltest.AgentTypeOpenCode:
		return "anthropic"
	case protocoltest.AgentTypeCodex:
		return "openai"
	}
	return ""
}
