package config

import (
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Provider-related methods (merged from AppConfig)

// ProviderUpdateHook is called when a provider is updated
type ProviderUpdateHook interface {
	OnProviderUpdate(provider *typ.Provider)
}

// ProviderDeleteHook is called when a provider is deleted
type ProviderDeleteHook interface {
	OnProviderDelete(uuid string)
}

// migrateProvidersToDB migrates providers from JSON config to database.
// This is a one-time migration that runs on startup if the database is empty.
// After migration (or if the database is already authoritative), the JSON
// provider fields are cleared and the config is persisted so subsequent
// startups load empty JSON provider data.
func (c *Config) migrateProvidersToDB() error {
	if c.providerStore == nil {
		return nil // Provider store not initialized, skip migration
	}

	// Check if database already has providers
	count, err := c.providerStore.Count()
	if err != nil {
		return fmt.Errorf("failed to check provider count: %w", err)
	}

	if count > 0 {
		// Database is authoritative. Clear any stale JSON backup left over
		// from earlier versions that kept providers in JSON post-migration.
		if len(c.Providers) > 0 || len(c.ProvidersV1) > 0 {
			logrus.Infof("Clearing stale provider JSON data (%d v2 / %d v1); database is authoritative", len(c.Providers), len(c.ProvidersV1))
			return c.clearProviderJSON()
		}
		logrus.Debugf("Database already has %d providers, skipping migration", count)
		return nil
	}

	// Check if JSON config has providers to migrate
	if len(c.Providers) == 0 {
		return nil // No providers to migrate
	}

	logrus.Infof("Migrating %d providers from JSON config to database...", len(c.Providers))

	// Migrate each provider to database
	for _, provider := range c.Providers {
		if err := c.providerStore.Save(provider); err != nil {
			return fmt.Errorf("failed to migrate provider %s: %w", provider.UUID, err)
		}
	}

	logrus.Infof("Successfully migrated %d providers to database", len(c.Providers))

	return c.clearProviderJSON()
}

// clearProviderJSON drops the legacy JSON-config provider fields and persists
// the change so the on-disk config no longer carries duplicate provider data.
func (c *Config) clearProviderJSON() error {
	c.Providers = nil
	c.ProvidersV1 = nil
	if err := c.Save(); err != nil {
		return fmt.Errorf("failed to save config after clearing provider JSON: %w", err)
	}
	return nil
}

// AddProviderByName adds a new AI provider configuration by name, API base, and token
func (c *Config) AddProviderByName(name, apiBase, token string) error {
	if name == "" {
		return errors.New("provider name cannot be empty")
	}
	if apiBase == "" {
		return errors.New("API base URL cannot be empty")
	}

	provider := &typ.Provider{
		UUID:     GenerateUUID(), // Generate a new UUID for the provider
		Name:     name,
		APIBase:  apiBase,
		APIStyle: protocol.APIStyleOpenAI, // default to openai
		AuthType: typ.AuthTypeAPIKey,
		Token:    token,
		Enabled:  true,
	}

	return c.AddProvider(provider)
}

// GetProviderByUUID returns a provider from database
func (c *Config) GetProviderByUUID(uuid string) (*typ.Provider, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.providerStore == nil {
		return nil, fmt.Errorf("provider store not initialized")
	}

	provider, err := c.providerStore.GetByUUID(uuid)
	if err != nil {
		return nil, fmt.Errorf("provider '%s' not found: %w", uuid, err)
	}
	return provider, nil
}

// GetProviderStore returns the provider store instance
func (c *Config) GetProviderStore() *db.ProviderStore {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.providerStore
}

func (c *Config) GetProviderByName(name string) (*typ.Provider, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Try provider store first
	if c.providerStore == nil {
		panic("[db] Provider store missing")
	}

	if provider, err := c.providerStore.GetByName(name); err == nil {
		return provider, nil
	}

	return nil, fmt.Errorf("provider with name '%s' not found", name)
}

// ListProviders returns all providers
func (c *Config) ListProviders() []*typ.Provider {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Try provider store first (database is the source of truth)
	if c.providerStore == nil {
		panic("[db] Provider store missing")
	}
	providers, err := c.providerStore.List()
	if err == nil {
		return providers
	}
	// Database error - log warning and fall back to in-memory providers
	logrus.Warnf("Failed to list providers from database store, falling back to config file: %v", err)

	return nil
}

// ListOAuthProviders returns all OAuth-enabled providers
func (c *Config) ListOAuthProviders() ([]*typ.Provider, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Try provider store first
	if c.providerStore == nil {
		panic("[db] Provider store missing")
	}
	providers, err := c.providerStore.ListOAuth()
	if err == nil {
		return providers, nil
	}

	return nil, nil
}

// AddProvider adds a new provider using Provider struct
func (c *Config) AddProvider(provider *typ.Provider) error {
	if provider.Name == "" {
		return errors.New("provider name cannot be empty")
	}
	if provider.APIBase == "" {
		return errors.New("API base URL cannot be empty")
	}

	// Use provider store if available
	if c.providerStore != nil {
		if provider.UUID == "" {
			provider.UUID = GenerateUUID()
		}
		return c.providerStore.Save(provider)
	}
	return nil
}

// RegisterProviderUpdateHook adds a hook to be called when a provider is updated
func (c *Config) RegisterProviderUpdateHook(hook ProviderUpdateHook) {
	c.hookMu.Lock()
	defer c.hookMu.Unlock()
	c.providerUpdateHooks = append(c.providerUpdateHooks, hook)
}

// RegisterProviderDeleteHook adds a hook to be called when a provider is deleted
func (c *Config) RegisterProviderDeleteHook(hook ProviderDeleteHook) {
	c.hookMu.Lock()
	defer c.hookMu.Unlock()
	c.providerDeleteHooks = append(c.providerDeleteHooks, hook)
}

// notifyProviderUpdate notifies all registered hooks about a provider update
func (c *Config) notifyProviderUpdate(provider *typ.Provider) {
	c.hookMu.RLock()
	hooks := make([]ProviderUpdateHook, len(c.providerUpdateHooks))
	copy(hooks, c.providerUpdateHooks)
	c.hookMu.RUnlock()

	for _, hook := range hooks {
		// Call hook in goroutine to avoid blocking the update operation
		go func(h ProviderUpdateHook) {
			defer func() {
				if r := recover(); r != nil {
					logrus.Errorf("Provider update hook panic: %v", r)
				}
			}()
			h.OnProviderUpdate(provider)
		}(hook)
	}
}

// notifyProviderDelete notifies all registered hooks about a provider deletion
func (c *Config) notifyProviderDelete(uuid string) {
	c.hookMu.RLock()
	hooks := make([]ProviderDeleteHook, len(c.providerDeleteHooks))
	copy(hooks, c.providerDeleteHooks)
	c.hookMu.RUnlock()

	for _, hook := range hooks {
		go func(h ProviderDeleteHook) {
			defer func() {
				if r := recover(); r != nil {
					logrus.Errorf("Provider delete hook panic: %v", r)
				}
			}()
			h.OnProviderDelete(uuid)
		}(hook)
	}
}

// UpdateProvider updates an existing provider by UUID
func (c *Config) UpdateProvider(uuid string, provider *typ.Provider) error {
	// Use provider store if available
	if c.providerStore != nil {
		// Preserve the UUID
		provider.UUID = uuid
		if err := c.providerStore.Save(provider); err != nil {
			return err
		}

		// Notify hooks after successful update
		c.notifyProviderUpdate(provider)
		return nil
	}

	// Fallback to in-memory (for migration period)
	c.mu.Lock()
	defer c.mu.Unlock()

	return fmt.Errorf("provider with UUID '%s' not found", uuid)
}

// DeleteProvider removes a provider by UUID
func (c *Config) DeleteProvider(uuid string) error {
	// Use provider store if available
	if c.providerStore == nil {
		panic("[db] Provider store missing")
	}

	if err := c.providerStore.Delete(uuid); err != nil {
		return err
	}

	// Delete the associated model file
	if c.modelManager != nil {
		_ = c.modelManager.RemoveProvider(uuid)
	}

	// Remove services referencing this provider from all rules before notifying hooks
	// This ensures data consistency between providers and rules
	c.removeProviderServicesFromRules(uuid)

	// Notify hooks after successful deletion
	c.notifyProviderDelete(uuid)

	return nil
}

// removeProviderServicesFromRules removes all services that reference the deleted provider
// from all rules (both regular services and smart routing services). This maintains
// data consistency when a provider is deleted. Rules with no services will be handled
// by the server during request processing (returning "no service available" error).
// Also clears stale CurrentServiceID entries that reference the deleted provider.
func (c *Config) removeProviderServicesFromRules(providerUUID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	needsSave := false
	var updatedRuleUUIDs []string

	for i := range c.Rules {
		rule := &c.Rules[i]
		ruleModified := false
		originalServiceCount := len(rule.Services)

		// Filter out regular services that reference the deleted provider
		var filteredServices []*loadbalance.Service
		for _, svc := range rule.Services {
			if svc.Provider != providerUUID {
				filteredServices = append(filteredServices, svc)
			}
		}

		if len(filteredServices) != originalServiceCount {
			needsSave = true
			rule.Services = filteredServices
			ruleModified = true
			logrus.WithFields(logrus.Fields{
				"provider":      providerUUID,
				"rule":          rule.UUID,
				"services_left": len(filteredServices),
			}).Info("Removed services from rule after provider deletion")
		}

		// Filter out smart routing services that reference the deleted provider
		for srIdx := range rule.SmartRouting {
			sr := &rule.SmartRouting[srIdx]
			originalSRServiceCount := len(sr.Services)

			var filteredSRServices []*loadbalance.Service
			for _, svc := range sr.Services {
				if svc.Provider != providerUUID {
					filteredSRServices = append(filteredSRServices, svc)
				}
			}

			if len(filteredSRServices) != originalSRServiceCount {
				needsSave = true
				ruleModified = true
				sr.Services = filteredSRServices
				logrus.WithFields(logrus.Fields{
					"provider":      providerUUID,
					"rule":          rule.UUID,
					"smart_routing": srIdx,
					"services_left": len(filteredSRServices),
				}).Info("Removed smart routing services after provider deletion")
			}
		}

		if ruleModified {
			updatedRuleUUIDs = append(updatedRuleUUIDs, rule.UUID)

			// Clear stale CurrentServiceID if it references the deleted provider
			if rule.CurrentServiceID != "" {
				// CurrentServiceID is in "provider:model" format
				// Check if it starts with the deleted provider UUID
				if len(rule.CurrentServiceID) > len(providerUUID) &&
					rule.CurrentServiceID[:len(providerUUID)] == providerUUID &&
					rule.CurrentServiceID[len(providerUUID)] == ':' {
					rule.CurrentServiceID = ""
					logrus.WithFields(logrus.Fields{
						"provider":           providerUUID,
						"rule":               rule.UUID,
						"current_service_id": rule.CurrentServiceID,
					}).Info("Cleared stale CurrentServiceID after provider deletion")
				}
			}

			// Also clear from the rule state store (SQLite)
			if c.ruleStateStore != nil {
				if err := c.ruleStateStore.ClearServiceIDForProvider(rule.UUID, providerUUID); err != nil {
					logrus.WithError(err).WithFields(logrus.Fields{
						"provider": providerUUID,
						"rule":     rule.UUID,
					}).Warn("Failed to clear stale CurrentServiceID from rule state store")
				}
			}
		}
	}

	// Save if any rules were modified
	if needsSave {
		if err := c.Save(); err != nil {
			logrus.WithError(err).Error("Failed to save config after removing provider services from rules")
		} else {
			logrus.WithField("rules_updated", len(updatedRuleUUIDs)).Info("Successfully cleaned up rules after provider deletion")
		}
	}
}
