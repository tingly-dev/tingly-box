package agent

import (
	"fmt"

	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
)

// OpenCodeConfig implements AgentConfig for OpenCode
type OpenCodeConfig struct{}

// OpenCodeParams contains parameters for applying OpenCode configuration
type OpenCodeParams struct {
	// Config is the complete OpenCode configuration object
	// Caller is responsible for constructing this with appropriate structure
	Config map[string]interface{}
}

// Apply applies OpenCode configuration
func (o *OpenCodeConfig) Apply(paramsInterface interface{}) (*ApplyAgentResult, error) {
	params, ok := paramsInterface.(*OpenCodeParams)
	if !ok {
		return nil, fmt.Errorf("invalid params type, expected *OpenCodeParams")
	}

	// Apply config
	applyResult, err := serverconfig.ApplyOpenCodeConfig(params.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to apply OpenCode config: %w", err)
	}

	result := &ApplyAgentResult{
		AgentType: AgentTypeOpenCode,
		Success:   applyResult.Success,
	}

	if applyResult.Success {
		result.ConfigFiles = []string{"~/.config/opencode/opencode.json"}
		if applyResult.Created {
			result.ConfigFiles[0] += " (created)"
		} else {
			result.ConfigFiles[0] += " (updated)"
		}
	}

	if applyResult.BackupPath != "" {
		result.BackupPaths = []string{applyResult.BackupPath}
	}

	return result, nil
}

// Restore restores OpenCode configuration from backup
func (o *OpenCodeConfig) Restore() (*RestoreAgentResult, error) {
	return RestoreAgent(AgentTypeOpenCode)
}

// ApplyOpenCode applies OpenCode configuration.
// Deprecated: Use OpenCodeConfig.Apply() instead
func ApplyOpenCode(params *OpenCodeParams) (*ApplyAgentResult, error) {
	config := &OpenCodeConfig{}
	return config.Apply(params)
}
