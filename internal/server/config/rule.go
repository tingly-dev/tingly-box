package config

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// SmartGuide rule management

// SmartGuideRuleUUIDPrefix is the prefix for internal SmartGuide rules
// Each bot will have its own rule: _internal_smart_guide_{botUUID}
const SmartGuideRuleUUIDPrefix = "_internal_smart_guide_"

// SmartGuideRuleUUID generates a unique rule UUID for a specific bot
func SmartGuideRuleUUID(botUUID string) string {
	return SmartGuideRuleUUIDPrefix + botUUID
}

// EnsureSmartGuideRuleForBot ensures the _smart_guide rule exists for a specific bot
// If the rule doesn't exist, it creates it. If it exists but the configuration differs,
// it updates the rule. The rule is persisted to config.json.
//
// Each bot gets its own rule with UUID: _internal_smart_guide_{botUUID}
// The rule name uses the bot's name (if provided), otherwise uses botUUID
func (c *Config) EnsureSmartGuideRuleForBot(botUUID, botName, providerUUID, modelID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Generate rule UUID for this specific bot
	ruleUUID := SmartGuideRuleUUID(botUUID)

	// Use bot name for description, fallback to botUUID
	displayName := botName
	if displayName == "" {
		displayName = botUUID
	}

	// Validate provider exists and is enabled
	provider, err := c.providerStore.GetByUUID(providerUUID)
	if err != nil || provider == nil {
		logrus.WithFields(logrus.Fields{
			"provider": providerUUID,
			"bot":      botUUID,
		}).Error("SmartGuide provider not found")
		return fmt.Errorf("provider %s not found", providerUUID)
	}
	if !provider.Enabled {
		logrus.WithFields(logrus.Fields{
			"provider": providerUUID,
			"bot":      botUUID,
		}).Error("SmartGuide provider is disabled")
		return fmt.Errorf("provider %s is disabled", providerUUID)
	}

	// Check if rule already exists for this bot
	existingRuleIndex := -1
	for i, rule := range c.Rules {
		if rule.UUID == ruleUUID {
			existingRuleIndex = i
			break
		}
	}

	// Create new rule if it doesn't exist
	if existingRuleIndex == -1 {
		newRule := typ.Rule{
			UUID:          ruleUUID,
			Scenario:      typ.ScenarioSmartGuide,
			RequestModel:  WildcardRuleName,
			ResponseModel: modelID,
			Description:   fmt.Sprintf("Auto-generated rule for SmartGuide bot '%s' (DO NOT EDIT)", displayName),
			Services: []*loadbalance.Service{
				{
					Provider: providerUUID,
					Model:    modelID,
					Active:   true,
				},
			},
			LBTactic: typ.Tactic{
				Type: loadbalance.TacticAdaptive,
			},
			Active:       true,
			SmartEnabled: false,
		}

		c.Rules = append(c.Rules, newRule)
		if err := c.Save(); err != nil {
			logrus.WithError(err).Error("Failed to save SmartGuide rule")
			return fmt.Errorf("failed to save rule: %w", err)
		}

		logrus.WithFields(logrus.Fields{
			"bot":            botUUID,
			"bot_name":       botName,
			"provider":       providerUUID,
			"model":          modelID,
			"rule_uuid":      ruleUUID,
			"scenario":       typ.ScenarioSmartGuide,
			"request_model":  WildcardRuleName,
			"response_model": modelID,
		}).Info("SmartGuide rule created")
		return nil
	}

	// Update existing rule if configuration differs
	existingRule := &c.Rules[existingRuleIndex]
	needsUpdate := false

	// Check if services need updating
	if len(existingRule.Services) == 0 {
		needsUpdate = true
	} else {
		firstService := existingRule.Services[0]
		if firstService.Provider != providerUUID || firstService.Model != modelID {
			needsUpdate = true
		}
	}

	if needsUpdate {
		existingRule.Services = []*loadbalance.Service{
			{
				Provider: providerUUID,
				Model:    modelID,
				Active:   true,
			},
		}
		existingRule.ResponseModel = modelID
		// Update description if bot name changed
		if botName != "" {
			existingRule.Description = fmt.Sprintf("Auto-generated rule for SmartGuide bot '%s' (DO NOT EDIT)", botName)
		}

		if err := c.Save(); err != nil {
			logrus.WithError(err).Error("Failed to update SmartGuide rule")
			return fmt.Errorf("failed to update rule: %w", err)
		}

		logrus.WithFields(logrus.Fields{
			"bot":       botUUID,
			"bot_name":  botName,
			"provider":  providerUUID,
			"model":     modelID,
			"rule_uuid": ruleUUID,
			"scenario":  typ.ScenarioSmartGuide,
		}).Info("SmartGuide rule updated")
	} else {
		logrus.WithFields(logrus.Fields{
			"bot":       botUUID,
			"bot_name":  botName,
			"provider":  providerUUID,
			"model":     modelID,
			"rule_uuid": ruleUUID,
		}).Debug("SmartGuide rule already exists with correct configuration")
	}

	return nil
}

// EnsureSmartGuideRule ensures the _smart_guide rule exists (backward compatible)
// This method creates a rule without bot-specific identification
// Deprecated: Use EnsureSmartGuideRuleForBot for bot-specific rules
func (c *Config) EnsureSmartGuideRule(providerUUID, modelID string) error {
	return c.EnsureSmartGuideRuleForBot("", "", providerUUID, modelID)
}

// GetSmartGuideRuleForBot returns the _smart_guide rule for a specific bot
func (c *Config) GetSmartGuideRuleForBot(botUUID string) *typ.Rule {
	c.mu.RLock()
	defer c.mu.RUnlock()

	ruleUUID := SmartGuideRuleUUID(botUUID)
	for i, rule := range c.Rules {
		if rule.UUID == ruleUUID {
			return &c.Rules[i]
		}
	}
	return nil
}

// GetSmartGuideRule returns the _smart_guide rule (backward compatible, non-bot-specific)
// Deprecated: Use GetSmartGuideRuleForBot for bot-specific rules
func (c *Config) GetSmartGuideRule() *typ.Rule {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Return first matching smart guide rule
	for i, rule := range c.Rules {
		if rule.Scenario == typ.ScenarioSmartGuide {
			return &c.Rules[i]
		}
	}
	return nil
}
