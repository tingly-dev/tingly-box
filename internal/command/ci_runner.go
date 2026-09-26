package command

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/agent"
)

// applyCISpec is the runner for `ci apply`. It performs two steps:
//
//  1. upsert the provider by URL (idempotent: re-running CI with the same URL
//     updates the existing row rather than duplicating it).
//  2. delegate to agent.ApplyAgent with Force=true so it never blocks on a
//     confirmation prompt. ApplyAgent handles routing-rule creation and
//     config-file writes idempotently when given a concrete provider UUID and model.
func applyCISpec(am *AppManager, s *ciSpec) error {
	providerUUID, action, err := upsertProviderByURL(am, s)
	if err != nil {
		return fmt.Errorf("provider upsert failed: %w", err)
	}
	fmt.Printf("provider %s: %s (%s)\n", action, s.ProviderURL, providerUUID)

	req := &agent.ApplyAgentRequest{
		AgentType:         s.AgentType,
		Provider:          providerUUID,
		Model:             s.Model,
		Unified:           s.Unified,
		InstallStatusLine: s.StatusLine,
		Force:             true,
	}

	globalConfig := am.GetGlobalConfig()
	apply := agent.NewAgentApply(globalConfig, "127.0.0.1")
	result, err := apply.ApplyAgent(req)
	if err != nil {
		return fmt.Errorf("agent apply failed: %w", err)
	}
	if !result.Success {
		return fmt.Errorf("agent apply did not succeed: %s", result.Message)
	}

	fmt.Print("\n" + result.Message)
	return nil
}

// upsertProviderByURL looks for an existing provider whose APIBase matches
// s.ProviderURL and updates it in place; otherwise creates a new one with the
// URL as its name. The returned action string ("created" or "updated") is for
// the user-facing log line only.
func upsertProviderByURL(am *AppManager, s *ciSpec) (string, string, error) {
	for _, p := range am.ListProviders() {
		if p.APIBase == s.ProviderURL {
			p.Token = s.ProviderToken
			p.APIStyle = s.ProviderStyle
			p.Enabled = true
			if err := am.UpdateProviderByUUID(p.UUID, p); err != nil {
				return "", "", err
			}
			return p.UUID, "updated", nil
		}
	}

	uuid, err := am.AddProvider(s.ProviderURL, s.ProviderURL, s.ProviderToken, s.ProviderStyle)
	if err != nil {
		return "", "", err
	}
	return uuid, "created", nil
}
