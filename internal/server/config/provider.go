package config

import (
	"errors"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/data/db"
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

// migrateProvidersToDB migrates providers from JSON config to database
// This is a one-time migration that runs on startup if the database is empty
// Note: Providers are kept in JSON config as backup for now, will be cleared in a future version
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
		// Database already has providers, skip migration
		logrus.Debugf("Database already has %d providers, skipping migration", count)
		return nil
	}

	// Check if JSON config has providers to migrate
	if len(c.Providers) == 0 {
		return nil // No providers to migrate
	}

	logrus.Infof("Migrating %d providers from JSON config to database (keeping JSON as backup)...", len(c.Providers))

	// Migrate each provider to database
	for _, provider := range c.Providers {
		if err := c.providerStore.Save(provider); err != nil {
			return fmt.Errorf("failed to migrate provider %s: %w", provider.UUID, err)
		}
	}

	logrus.Infof("Successfully migrated %d providers to database", len(c.Providers))
	// Note: We keep c.Providers in JSON config as backup for now
	// In a future version, we will clear: c.Providers = nil; c.ProvidersV1 = nil; c.Save()

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

	// Notify hooks after successful deletion
	c.notifyProviderDelete(uuid)

	return nil
}
