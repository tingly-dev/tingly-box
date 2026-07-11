package quota

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	typ "github.com/tingly-dev/tingly-box/ai"
)

const maxConcurrentRefreshes = 5

// Manager coordinates quota fetching, storage, and refreshes.
type Manager struct {
	config      *Config
	store       Store
	registry    *Registry
	providerMgr ProviderManager
	logger      *logrus.Logger
	refresher   *Refresher
}

// ProviderManager provides access to configured providers.
type ProviderManager interface {
	GetProviderByUUID(uuid string) (*typ.Provider, error)
	ListProviders() []*typ.Provider
}

// NewManager creates a quota manager.
func NewManager(config *Config, store Store, providerMgr ProviderManager, logger *logrus.Logger) *Manager {
	if config == nil {
		config = DefaultConfig()
	}

	m := &Manager{
		config:      config,
		store:       store,
		registry:    NewRegistry(),
		providerMgr: providerMgr,
		logger:      logger,
	}

	// Create the background refresher.
	m.refresher = NewRefresher(m, logger)

	return m
}

// RegisterFetcher registers a quota fetcher.
func (m *Manager) RegisterFetcher(fetcher Fetcher) error {
	if err := m.registry.Register(fetcher); err != nil {
		return fmt.Errorf("failed to register fetcher: %w", err)
	}
	m.logger.Infof("registered quota fetcher: %s for provider type: %s", fetcher.Name(), fetcher.ProviderType())
	return nil
}

// Refresh refreshes quota data for every enabled provider.
func (m *Manager) Refresh(ctx context.Context) ([]*ProviderUsage, error) {
	providers := m.providerMgr.ListProviders()
	if len(providers) == 0 {
		return []*ProviderUsage{}, nil
	}

	enabled := make([]*typ.Provider, 0, len(providers))
	for _, provider := range providers {
		if m.isProviderEnabled(provider) {
			enabled = append(enabled, provider)
		}
	}
	if len(enabled) == 0 {
		return []*ProviderUsage{}, nil
	}

	workerCount := min(maxConcurrentRefreshes, len(enabled))
	jobs := make(chan *typ.Provider)
	results := make(chan *ProviderUsage, len(enabled))
	var wg sync.WaitGroup
	wg.Add(workerCount)
	for range workerCount {
		go func() {
			defer wg.Done()
			for provider := range jobs {
				usage, err := m.fetchProviderQuota(ctx, provider)
				if err != nil {
					m.loggerWithError(provider, err).Warn("failed to fetch quota")
					continue
				}
				results <- usage
			}
		}()
	}

	go func() {
		defer close(jobs)
		for _, provider := range enabled {
			select {
			case jobs <- provider:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	collected := make([]*ProviderUsage, 0, len(enabled))
	for usage := range results {
		collected = append(collected, usage)
	}
	return collected, nil
}

// RefreshProvider refreshes quota data for one provider.
func (m *Manager) RefreshProvider(ctx context.Context, providerUUID string) (*ProviderUsage, error) {
	provider, err := m.providerMgr.GetProviderByUUID(providerUUID)
	if err != nil {
		return nil, fmt.Errorf("provider not found: %w", err)
	}

	return m.fetchProviderQuota(ctx, provider)
}

// GetQuota returns cached quota data and refreshes it when expired.
func (m *Manager) GetQuota(ctx context.Context, providerUUID string) (*ProviderUsage, error) {
	usage, err := m.store.Get(ctx, providerUUID)
	if err != nil {
		if err == ErrUsageNotFound {
			return nil, fmt.Errorf("quota not found for provider: %s", providerUUID)
		}
		return nil, err
	}

	// Refresh expired quota data.
	if usage.IsExpired() {
		m.logger.WithField("provider_uuid", providerUUID).Debug("quota expired, fetching fresh data")
		return m.RefreshProvider(ctx, providerUUID)
	}

	return usage, nil
}

// GetQuotaNoCache returns the latest quota data stored in the database.
func (m *Manager) GetQuotaNoCache(ctx context.Context, providerUUID string) (*ProviderUsage, error) {
	usage, err := m.store.Get(ctx, providerUUID)
	if err != nil {
		if err == ErrUsageNotFound {
			return nil, fmt.Errorf("quota not found for provider: %s", providerUUID)
		}
		return nil, err
	}
	return usage, nil
}

// ListQuota returns quota data for all providers.
func (m *Manager) ListQuota(ctx context.Context) ([]*ProviderUsage, error) {
	return m.store.List(ctx)
}

// Summary returns aggregate quota statistics.
func (m *Manager) Summary(ctx context.Context) (*Summary, error) {
	usages, err := m.store.List(ctx)
	if err != nil {
		return nil, err
	}

	summary := &Summary{
		TotalProviders: len(usages),
		ByStatus:       make(map[string]int),
		ByType:         make(map[ProviderType]int),
	}

	for _, usage := range usages {
		// Count providers by status.
		if usage.LastError != "" {
			summary.ErrorProviders++
			summary.ByStatus["error"]++
		} else {
			summary.OKProviders++
			summary.ByStatus["ok"]++
		}

		// Count providers by type.
		summary.ByType[usage.ProviderType]++

		// Count providers with a warning-level window.
		usage.NormalizeWindows()
		for _, window := range usage.Windows {
			if window != nil && window.UsedPercent >= 80 {
				summary.WarningProviders++
				break
			}
		}
	}

	return summary, nil
}

// StartAutoRefresh starts periodic quota refreshes.
func (m *Manager) StartAutoRefresh(ctx context.Context) {
	if !m.config.Enabled {
		m.logger.Info("auto-refresh disabled by config")
		return
	}
	m.refresher.Start(ctx, m.config.RefreshInterval)
}

// StopAutoRefresh stops periodic quota refreshes.
func (m *Manager) StopAutoRefresh() {
	m.refresher.Stop()
}

// isProviderEnabled reports whether quota fetching is enabled for a provider.
func (m *Manager) isProviderEnabled(provider *typ.Provider) bool {
	// Honor the global switch.
	if !m.config.Enabled {
		return false
	}

	// The provider itself must be enabled.
	if !provider.Enabled {
		return false
	}

	// Apply provider-specific configuration when present.
	if cfg, ok := m.config.Providers[provider.Name]; ok {
		return cfg.Enabled
	}

	// Enable providers by default.
	return true
}

// IsProviderSupported reports whether the provider has a registered quota
// fetcher. When false, the caller should skip quota fetching rather than
// emitting a misleading "unsupported provider type" error for the response.
func (m *Manager) IsProviderSupported(providerUUID string) bool {
	provider, err := m.providerMgr.GetProviderByUUID(providerUUID)
	if err != nil || provider == nil {
		return false
	}
	providerType := inferProviderType(provider)
	_, ok := m.registry.Get(providerType)
	return ok
}

// fetchProviderQuota fetches and stores quota data for one provider.
// It always returns ProviderUsage, either successful or containing error details.
func (m *Manager) fetchProviderQuota(ctx context.Context, provider *typ.Provider) (*ProviderUsage, error) {
	providerType := inferProviderType(provider)
	now := time.Now()

	// Verify that a fetcher is registered.
	f, ok := m.registry.Get(providerType)
	if !ok {
		usage := &ProviderUsage{
			ProviderUUID: provider.UUID,
			ProviderName: provider.Name,
			ProviderType: providerType,
			FetchedAt:    now,
			ExpiresAt:    now.Add(m.config.CacheTTL),
			LastError:    fmt.Sprintf("unsupported provider type: %q", providerType),
			LastErrorAt:  ptrTime(now),
		}
		_ = m.store.Save(ctx, usage)
		return usage, nil
	}

	// Validate the provider configuration.
	if err := f.Validate(provider); err != nil {
		usage := &ProviderUsage{
			ProviderUUID: provider.UUID,
			ProviderName: provider.Name,
			ProviderType: providerType,
			FetchedAt:    now,
			ExpiresAt:    now.Add(m.config.CacheTTL),
			LastError:    fmt.Sprintf("validation failed: %v", err),
			LastErrorAt:  ptrTime(now),
		}
		_ = m.store.Save(ctx, usage)
		return usage, nil
	}

	// Fetch quota data.
	usage, err := f.Fetch(ctx, provider)
	if err != nil {
		usage = &ProviderUsage{
			ProviderUUID: provider.UUID,
			ProviderName: provider.Name,
			ProviderType: providerType,
			FetchedAt:    now,
			ExpiresAt:    now.Add(m.config.CacheTTL),
			LastError:    err.Error(),
			LastErrorAt:  ptrTime(now),
		}
	}

	// Persist the result.
	if saveErr := m.store.Save(ctx, usage); saveErr != nil {
		m.logger.WithError(saveErr).Error("failed to save quota")
	}

	return usage, nil
}

// inferProviderType infers the provider type from OAuth metadata or the API base URL.
func inferProviderType(provider *typ.Provider) ProviderType {
	// OAuth providers: use OAuthDetail.GetIssuer() which handles backward compatibility
	if provider.AuthType == typ.AuthTypeOAuth && provider.OAuthDetail != nil {
		issuer := provider.OAuthDetail.GetIssuer()
		if issuer != "" {
			switch issuer {
			case typ.IssuerAnthropic, typ.IssuerClaudeCode:
				return ProviderTypeAnthropic
			case typ.IssuerGoogle:
				return ProviderTypeGemini
			case typ.IssuerOpenAI:
				return ProviderTypeOpenAI
			case typ.IssuerCopilot:
				return ProviderTypeCopilot
			case typ.IssuerCursor:
				return ProviderTypeCursor
			case typ.IssuerCodex:
				return ProviderTypeCodex
			}
		}
	}

	// Fallback: infer from APIBase domain
	apiBase := strings.ToLower(provider.APIBase)
	switch {
	case strings.Contains(apiBase, "anthropic.com"):
		return ProviderTypeAnthropic
	case strings.Contains(apiBase, "openai.com"), strings.Contains(apiBase, "openai.azure.com"):
		return ProviderTypeOpenAI
	case strings.Contains(apiBase, "googleapis.com"), strings.Contains(apiBase, "gemini"):
		return ProviderTypeGemini
	case strings.Contains(apiBase, "cursor"):
		return ProviderTypeCursor
	case strings.Contains(apiBase, "copilot"):
		return ProviderTypeCopilot
	case strings.Contains(apiBase, "vertex"):
		return ProviderTypeVertexAI
	case strings.Contains(apiBase, "zai.app"):
		return ProviderTypeZai
	case strings.Contains(apiBase, "bigmodel.cn"):
		return ProviderTypeGLM
	case strings.Contains(apiBase, "moonshot.cn"):
		return ProviderTypeKimiK2
	case strings.Contains(apiBase, "openrouter.ai"):
		return ProviderTypeOpenRouter
	case strings.Contains(apiBase, "minimaxi.com"):
		return ProviderTypeMiniMaxCN
	case strings.Contains(apiBase, "minimax"):
		return ProviderTypeMiniMax
	case strings.Contains(apiBase, "chatgpt.com"), strings.Contains(apiBase, "codex"):
		return ProviderTypeCodex
	}
	return ""
}

// Summary contains aggregate quota statistics.
type Summary struct {
	TotalProviders   int                  `json:"total_providers"`
	OKProviders      int                  `json:"ok_providers"`
	ErrorProviders   int                  `json:"error_providers"`
	WarningProviders int                  `json:"warning_providers"`
	ByStatus         map[string]int       `json:"by_status"`
	ByType           map[ProviderType]int `json:"by_type"`
}

// loggerWithError creates a provider-scoped log entry.
func (m *Manager) loggerWithError(provider *typ.Provider, err error) *logrus.Entry {
	return m.logger.WithFields(logrus.Fields{
		"provider_uuid": provider.UUID,
		"provider_name": provider.Name,
		"error":         err.Error(),
	})
}

// ptrTime returns a pointer to t.
func ptrTime(t time.Time) *time.Time {
	return &t
}
