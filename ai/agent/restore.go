package agent

import (
	"fmt"
	"strings"

	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
)

// RestoreAgent restores all config files for the given agent type from their
// most recent backup. Each file is handled independently — a missing backup
// for one file does not abort the others; per-file outcomes are summarised in
// the returned RestoreAgentResult.
//
// Routing rules and other in-process state are NOT touched: backups only cover
// the on-disk config files.
func RestoreAgent(agentType AgentType) (*RestoreAgentResult, error) {
	if !agentType.IsValid() {
		return nil, fmt.Errorf("unknown agent type: %s", agentType)
	}

	info, ok := GetAgentInfo(agentType)
	if !ok {
		return nil, fmt.Errorf("no info registered for agent type: %s", agentType)
	}

	result := &RestoreAgentResult{AgentType: agentType}

	for _, displayPath := range info.ConfigFiles {
		realPath, err := expandUser(displayPath)
		if err != nil {
			result.Failures = append(result.Failures,
				fmt.Sprintf("%s: %v", displayPath, err))
			continue
		}
		r, err := serverconfig.RestoreLatestBackup(realPath)
		if err != nil {
			msg := fmt.Sprintf("%s: %v", displayPath, err)
			if r != nil && r.Message != "" {
				msg = fmt.Sprintf("%s: %s", displayPath, r.Message)
			}
			result.Failures = append(result.Failures, msg)
			continue
		}
		result.RestoredFiles = append(result.RestoredFiles,
			fmt.Sprintf("%s <- %s", displayPath, r.RestoredFrom))
		if r.PreRestoreBackup != "" {
			result.PreRestoreBackups = append(result.PreRestoreBackups, r.PreRestoreBackup)
		}
	}

	result.Success = len(result.RestoredFiles) > 0 && len(result.Failures) == 0
	result.Message = buildRestoreMessage(result)
	return result, nil
}

// buildRestoreMessage formats a human-readable summary of a restore run.
func buildRestoreMessage(result *RestoreAgentResult) string {
	var sb strings.Builder
	if result.Success {
		sb.WriteString(fmt.Sprintf("Restored configuration for %s\n", result.AgentType))
	} else if len(result.RestoredFiles) > 0 {
		sb.WriteString(fmt.Sprintf("Partial restore for %s\n", result.AgentType))
	} else {
		sb.WriteString(fmt.Sprintf("Restore failed for %s\n", result.AgentType))
	}

	if len(result.RestoredFiles) > 0 {
		sb.WriteString("\nRestored files:\n")
		for _, f := range result.RestoredFiles {
			sb.WriteString(fmt.Sprintf("  - %s\n", f))
		}
	}
	if len(result.PreRestoreBackups) > 0 {
		sb.WriteString("\nPre-restore safety backups (used to roll back this restore):\n")
		for _, p := range result.PreRestoreBackups {
			sb.WriteString(fmt.Sprintf("  - %s\n", p))
		}
	}
	if len(result.Failures) > 0 {
		sb.WriteString("\nFailures:\n")
		for _, m := range result.Failures {
			sb.WriteString(fmt.Sprintf("  - %s\n", m))
		}
	}
	sb.WriteString("\nNote: routing rules are not part of the backup. Run apply to resync them if needed.\n")
	return sb.String()
}
