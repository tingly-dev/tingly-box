package main

import "fmt"

// runBatchAgentTests runs every supported agent in sequence. All agents run
// regardless of earlier failures; the command returns an error iff any agent
// failed. In virtual mode each agent uses its own default prompt unless
// `prompt` is non-empty. In real mode the same config file is reused across
// agents. If filter is non-empty, only config entries whose name matches (case-
// insensitive) are run; ignored in virtual mode.
func runBatchAgentTests(useMock bool, configFile string, prompt string, writer *summaryWriter, skip map[resumeKey]struct{}, only map[resumeKey]struct{}, filter []string) error {
	fmt.Printf("🧪 Batch agent test: %v\n", batchAgents)
	if configFile != "" {
		fmt.Printf("📋 Config: %s\n", configFile)
	} else {
		fmt.Printf("📋 Mode: virtual upstream (--mock)\n")
	}
	if len(filter) > 0 {
		fmt.Printf("🔍 Filter: %v\n", filter)
	}
	if prompt != "" {
		fmt.Printf("📝 Prompt: %s\n", prompt)
	} else {
		fmt.Printf("📝 Prompt: <per-agent default>\n")
	}
	fmt.Println()

	// Collect all per-entry results across all agents so we can render a single
	// unified detail table at the end. Fatal per-agent errors (e.g. bad config,
	// no runnable entries) are captured as synthetic failed result rows so they
	// still show up in the summary rather than vanishing into a log line.
	allResults := make([]*RealAgentTestResult, 0, len(batchAgents))

	for i, agentName := range batchAgents {
		fmt.Printf("══ [%d/%d] agent=%s ══\n", i+1, len(batchAgents), agentName)

		var results []*RealAgentTestResult
		var err error
		switch {
		case configFile != "":
			results, err = runRealAgentTests(agentName, configFile, prompt, writer, skip, only, filter)
		case useMock:
			results, err = runVirtualAgentTest(agentName, prompt, writer, skip)
		default:
			err = fmt.Errorf("internal: batch invoked without mode")
		}

		if len(results) > 0 {
			allResults = append(allResults, results...)
		} else if err != nil {
			// No per-entry results produced (e.g. config load failure). Emit a
			// synthetic row so the unified report still lists this agent.
			synthetic := &RealAgentTestResult{
				EntryName: "-",
				Agent:     agentName,
				APIStyle:  virtualAPIStyle(parseAgentType(agentName)),
				Error:     err.Error(),
			}
			allResults = append(allResults, synthetic)
			appendSummary(writer, synthetic)
		}
		fmt.Println()
	}

	// Unified summary: same shape as the single-agent path.
	printAgentSummary(allResults)

	failCount := 0
	for _, r := range allResults {
		if !r.Success {
			failCount++
		}
	}
	if failCount > 0 {
		return fmt.Errorf("%d of %d agent runs failed in batch", failCount, len(allResults))
	}
	return nil
}
