package command

import (
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/agent"
)

// applyCISpec is the runner for `ci apply`. It performs two steps:
//
//  1. upsert the provider by name (so re-running CI doesn't keep adding
//     duplicate provider rows — names are not unique at the storage layer,
//     but for headless setup we treat them as the upsert key).
//  2. delegate to agent.ApplyAgent with Force=true so it never blocks on a
//     confirmation prompt. ApplyAgent already handles routing-rule creation
//     and config-file writes idempotently when given a concrete provider
//     UUID and model.
func applyCISpec(am *AppManager, s *ciSpec) error {
	providerUUID, action, err := upsertProviderByName(am, s)
	if err != nil {
		return fmt.Errorf("provider upsert failed: %w", err)
	}
	fmt.Printf("provider %s: %s (%s)\n", action, s.ProviderName, providerUUID)

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

// upsertProviderByName looks up an existing provider by name and updates its
// URL/token/style if found, otherwise creates a new one. The returned action
// string ("created" or "updated") is used purely for the user-facing log line.
//
// Note: the storage layer does not enforce uniqueness on provider name, so
// in theory two providers could share a name. For CI we ignore that edge
// case — if it ever matters, the operator should give their CI provider a
// distinctive name like "ci-openrouter".
func upsertProviderByName(am *AppManager, s *ciSpec) (string, string, error) {
	existing, _ := am.GetProviderByName(s.ProviderName)
	if existing != nil {
		// Update in place. We deliberately overwrite every field the user
		// specified — that is the contract of a declarative CI flow.
		existing.APIBase = s.ProviderURL
		existing.Token = s.ProviderToken
		existing.APIStyle = s.ProviderStyle
		existing.Enabled = true
		if err := am.UpdateProviderByUUID(existing.UUID, existing); err != nil {
			return "", "", err
		}
		return existing.UUID, "updated", nil
	}

	uuid, err := am.AddProvider(s.ProviderName, s.ProviderURL, s.ProviderToken, s.ProviderStyle)
	if err != nil {
		return "", "", err
	}
	return uuid, "created", nil
}
