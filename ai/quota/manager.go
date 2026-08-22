package quota

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	typ "github.com/tingly-dev/tingly-box/ai"
)

const maxConcurrentRefreshes = 5

// warningUsedPercent is where a provider counts as close to running out.
const warningUsedPercent = 80

const (
	initialRetryDelay = 200 * time.Millisecond
	maxRetryDelay     = 2 * time.Second
	retryJitter       = 100 * time.Millisecond
	failedRefreshTTL  = time.Minute
)

// Manager coordinates quota fetching, storage, and refreshes.
type Manager struct {
	config      *Config
	store       Store
	registry    *Registry
	providerMgr ProviderManager
	logger      *logrus.Logger
	refresher   *Refresher
	retryDelay  func(attempt int) time.Duration
	refreshes   sync.Map // provider UUID -> chan struct{}
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
		retryDelay:  quotaRetryDelay,
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
				if errors.Is(err, ErrProviderUnsupported) {
					continue
				}
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
//
// Both ErrUsageNotFound and ErrProviderUnsupported mean "there is nothing to
// show for this provider" and reach the caller unwrapped, so handlers can skip
// them instead of reporting a failure.
//
// A not-found store lookup returns ErrUsageNotFound UNWRAPPED — callers
// (e.g. the provider-quota HTTP handlers) compare against that sentinel
// with == to treat "no data yet" as a skip rather than an error; wrapping it
// here (as this used to do, via fmt.Errorf) silently broke that comparison
// and turned every "no quota data" provider into a hard error upstream.
func (m *Manager) GetQuota(ctx context.Context, providerUUID string) (*ProviderUsage, error) {
	usage, err := m.store.Get(ctx, providerUUID)
	if err != nil {
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
// See GetQuota above for why ErrUsageNotFound must reach the caller unwrapped.
func (m *Manager) GetQuotaNoCache(ctx context.Context, providerUUID string) (*ProviderUsage, error) {
	return m.store.Get(ctx, providerUUID)
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

		// Count providers close to running out. Pct is the tightest window, so
		// this catches the one that will run out first rather than whichever
		// window happens to come first, and skips providers whose usage is
		// unknown instead of reading them as unused.
		if pct, ok := usage.Pct(); ok && pct >= warningUsedPercent {
			summary.WarningProviders++
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	requestedAt := time.Now()
	gate := m.providerRefreshGate(provider.UUID)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case gate <- struct{}{}:
	}
	defer func() { <-gate }()

	// A refresh that started after this caller arrived may have completed while
	// it waited for the gate. Reuse that result rather than repeating the same
	// upstream call. Each caller retains its own context; one canceled request
	// cannot abort another caller's refresh.
	if usage, err := m.store.Get(ctx, provider.UUID); err == nil && refreshedSince(usage, requestedAt) {
		return usage, nil
	}

	return m.fetchProviderQuotaOnce(ctx, provider)
}

func (m *Manager) providerRefreshGate(providerUUID string) chan struct{} {
	gate, _ := m.refreshes.LoadOrStore(providerUUID, make(chan struct{}, 1))
	return gate.(chan struct{})
}

func refreshedSince(usage *ProviderUsage, requestedAt time.Time) bool {
	if usage == nil {
		return false
	}
	if !usage.FetchedAt.Before(requestedAt) {
		return true
	}
	return usage.LastErrorAt != nil && !usage.LastErrorAt.Before(requestedAt)
}

func (m *Manager) fetchProviderQuotaOnce(ctx context.Context, provider *typ.Provider) (*ProviderUsage, error) {
	providerType := inferProviderType(provider)
	now := time.Now()

	// Log here rather than in the callers: a quota read is reached from the
	// background ticker, the manual refresh endpoint, and cache expiry, and
	// every one of those used to fail silently into a stored LastError.
	log := m.logger.WithFields(logrus.Fields{
		"provider_uuid": provider.UUID,
		"provider_name": provider.Name,
		"provider_type": providerType,
	})

	// Verify that a fetcher is registered. No fetcher is the ordinary case for
	// most providers, so it is reported as a skip, not stored as a failure:
	// persisting it put "unsupported provider type" in the record's LastError,
	// which the UI then showed as if the provider had broken. Any such record
	// left by an earlier version is dropped here.
	f, ok := m.registry.Get(providerType)
	if !ok {
		log.Debug("skipping quota fetch: no fetcher for this provider type")
		_ = m.store.Delete(ctx, provider.UUID)
		return nil, ErrProviderUnsupported
	}
	log = log.WithField("fetcher", f.Name())

	// Validate the provider configuration.
	if err := f.Validate(provider); err != nil {
		log.WithError(err).Warn("quota fetch skipped: provider failed validation")
		usage := m.unreadable(provider, providerType, now,
			fmt.Sprintf("validation failed: %v", err))
		_ = m.store.Save(ctx, usage)
		return usage, nil
	}

	// Read the fallback before the remote call while the request context is
	// still usable. If the fetch later exhausts that context, stale quota can
	// still be returned without replacing it with an empty error record.
	cached, cachedErr := m.store.Get(ctx, provider.UUID)

	// Fetch quota data. Quota reads are idempotent, so retry only transient
	// transport failures and keep the retry local to this provider.
	start := time.Now()
	usage, err := m.fetchWithRetry(ctx, f, provider, log)
	elapsed := time.Since(start)
	if err != nil {
		failedAt := time.Now()
		log.WithFields(logrus.Fields{
			"duration_ms": elapsed.Milliseconds(),
			"error":       err.Error(),
		}).Warn("quota fetch failed")
		if ctx.Err() != nil {
			return cached, ctx.Err()
		}
		if cachedErr != nil && !errors.Is(cachedErr, ErrUsageNotFound) {
			return nil, fmt.Errorf("load cached quota after fetch failure: %w", cachedErr)
		}
		usage = m.preserveLastSuccess(cached, provider, providerType, failedAt, err)
	} else {
		fields := logrus.Fields{
			"duration_ms": elapsed.Milliseconds(),
			"windows":     len(usage.Windows),
		}
		if pct, ok := usage.Pct(); ok {
			fields["used_percent"] = pct
		}
		log.WithFields(fields).Debug("quota fetched")
	}

	// Persist the result.
	if saveErr := m.store.Save(ctx, usage); saveErr != nil {
		log.WithError(saveErr).Error("failed to save quota")
	}

	return usage, nil
}

func (m *Manager) fetchWithRetry(ctx context.Context, f Fetcher, provider *typ.Provider, log *logrus.Entry) (*ProviderUsage, error) {
	attempts := 1
	if m.config.RetryOnFailure && m.config.MaxRetries > 0 {
		attempts += m.config.MaxRetries
	}

	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		var usage *ProviderUsage
		usage, err = f.Fetch(ctx, provider)
		if err == nil {
			return usage, nil
		}
		if attempt == attempts || !isTransientQuotaError(err) || ctx.Err() != nil {
			return nil, err
		}

		delay := m.retryDelay(attempt)
		log.WithFields(logrus.Fields{
			"attempt":      attempt,
			"max_attempts": attempts,
			"retry_in_ms":  delay.Milliseconds(),
			"error":        err.Error(),
		}).Warn("transient quota fetch failure; retrying")

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, err
}

func isTransientQuotaError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.EPIPE) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary())
}

func quotaRetryDelay(attempt int) time.Duration {
	delay := initialRetryDelay
	for step := 1; step < attempt && delay < maxRetryDelay; step++ {
		delay = min(delay*2, maxRetryDelay)
	}
	return delay + time.Duration(rand.Int64N(int64(retryJitter)+1))
}

// preserveLastSuccess keeps the last readable snapshot visible while recording
// why its refresh failed. FetchedAt and the quota payload remain untouched, so
// callers can distinguish stale data from a successful refresh.
func (m *Manager) preserveLastSuccess(usage *ProviderUsage, provider *typ.Provider, providerType ProviderType, now time.Time, fetchErr error) *ProviderUsage {
	if !hasSuccessfulSnapshot(usage) {
		return m.unreadable(provider, providerType, now, fetchErr.Error())
	}

	usage.MarkUnreadable(fetchErr.Error(), now)
	retryTTL := min(m.config.CacheTTL, failedRefreshTTL)
	if retryTTL <= 0 {
		retryTTL = failedRefreshTTL
	}
	usage.ExpiresAt = now.Add(retryTTL)
	return usage
}

func hasSuccessfulSnapshot(usage *ProviderUsage) bool {
	if usage == nil || usage.FetchedAt.IsZero() {
		return false
	}
	if usage.LastErrorAt == nil {
		return usage.LastError == ""
	}
	return usage.FetchedAt.Before(*usage.LastErrorAt)
}

// unreadable records a provider whose quota could not be read.
func (m *Manager) unreadable(provider *typ.Provider, providerType ProviderType, now time.Time, reason string) *ProviderUsage {
	return Unreadable(provider.UUID, provider.Name, providerType, reason, now, m.config.CacheTTL)
}

// inferProviderType infers the provider type from OAuth metadata or the API base URL.
func inferProviderType(provider *typ.Provider) ProviderType {
	// OAuth providers: use OAuthDetail.GetIssuer() which handles backward compatibility
	switch provider.OAuthIssuer() {
	case typ.IssuerAnthropic, typ.IssuerClaudeCode:
		return ProviderTypeAnthropic
	case typ.IssuerGoogle:
		return ProviderTypeGemini
	//case typ.IssuerOpenAI:
	//	return ProviderTypeOpenAI
	case typ.IssuerCopilot:
		return ProviderTypeCopilot
	case typ.IssuerCursor:
		return ProviderTypeCursor
	case typ.IssuerCodex:
		return ProviderTypeCodex
	case typ.IssuerKimiCode:
		return ProviderTypeKimiCode
	}

	// Fallback: infer from the APIBase host.
	//
	// The host, never the whole URL. Matching the URL made any path segment
	// speak for the vendor: a local gateway at
	// http://localhost:12581/tingly/codex1 was read as Codex — and the Codex
	// fetcher would then take that provider's token to chatgpt.com. Paths are
	// user-chosen names; only the host says who answers.
	host, path := apiBaseHostPath(provider.APIBase)
	if host == "" || isLocalAPIHost(host) {
		return ""
	}
	switch {
	case hostIs(host, "anthropic.com"):
		return ProviderTypeAnthropic
	case hostIs(host, "api.deepseek.com"):
		return ProviderTypeDeepSeek
	//case hostIs(host, "openai.com", "openai.azure.com"):
	//	return ProviderTypeOpenAI
	case hostIs(host, "googleapis.com"), strings.Contains(host, "gemini"):
		return ProviderTypeGemini
	case strings.Contains(host, "cursor"):
		return ProviderTypeCursor
	case strings.Contains(host, "copilot"):
		return ProviderTypeCopilot
	case strings.Contains(host, "vertex"):
		return ProviderTypeVertexAI
	case hostIs(host, "zai.app"):
		return ProviderTypeZai
	case hostIs(host, "bigmodel.cn"):
		return ProviderTypeGLM
	case hostIs(host, "moonshot.cn"):
		return ProviderTypeKimiK2
	// Kimi's coding product shares a host with the rest of kimi.com, so this
	// one pair genuinely needs the path to tell them apart.
	case hostIs(host, "kimi.com") && strings.HasPrefix(path, "/coding"):
		return ProviderTypeKimiCode
	case hostIs(host, "openrouter.ai"):
		return ProviderTypeOpenRouter
	case hostIs(host, "minimaxi.com"):
		return ProviderTypeMiniMaxCN
	case strings.Contains(host, "minimax"):
		return ProviderTypeMiniMax
	case hostIs(host, "chatgpt.com"), strings.Contains(host, "codex"):
		return ProviderTypeCodex
	// One host serves both OpenCode products: /zen/v1 bills the prepaid
	// balance, /zen/go/v1 the Go subscription. The fetcher reads the
	// subscription either way and says so when the key has no plan, so the
	// path does not need to split them here.
	case hostIs(host, "opencode.ai"):
		return ProviderTypeOpenCode
	}
	return ""
}

// apiBaseHostPath splits a configured API base into its lowercased host and
// path. A base without a scheme ("api.openai.com/v1") is still understood.
func apiBaseHostPath(apiBase string) (host, path string) {
	trimmed := strings.TrimSpace(apiBase)
	if trimmed == "" {
		return "", ""
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "http://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", ""
	}
	return strings.ToLower(parsed.Hostname()), strings.ToLower(parsed.Path)
}

// hostIs reports whether host is one of the domains, or a subdomain of one.
// Suffix matching, not substring: "notopenai.com.evil.test" is not OpenAI.
func hostIs(host string, domains ...string) bool {
	for _, domain := range domains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

// isLocalAPIHost reports whether the host is this machine or a private
// network. A provider pointing there is a local gateway — frequently
// tingly-box itself — and no vendor heuristic may claim it.
func isLocalAPIHost(host string) bool {
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast()
	}
	// A bare name with no dot is a LAN or container hostname, not a vendor.
	return !strings.Contains(host, ".")
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
