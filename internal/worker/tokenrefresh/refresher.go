package tokenrefresh

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/oauth"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

const (
	// defaultCheckInterval is how often to check for tokens needing refresh.
	//
	// This MUST stay well below defaultRefreshBuffer. The refresher is the only
	// thing that keeps a token alive — nothing refreshes on the request path --
	// so a token that falls due between two ticks is a hard 401 for every client
	// request until the next tick. With interval > buffer that window is
	// guaranteed once per token lifetime, however healthy the credential is.
	defaultCheckInterval = 2 * time.Minute
	// defaultRefreshBuffer is how long before expiry a token is refreshed.
	//
	// Sized as a multiple of defaultCheckInterval, not as "just before expiry":
	// it buys ~7 refresh attempts before the token actually dies, so a transient
	// token-endpoint failure costs a retry instead of a client-visible 401.
	defaultRefreshBuffer = 15 * time.Minute
	// maxExpiryDuration is the maximum time after token expiry to attempt refresh
	// Tokens expired longer than this will not be refreshed (72 hours = 3 days)
	maxExpiryDuration = 72 * time.Hour
	// jitterPercent is the maximum jitter percentage to add to the check interval
	jitterPercent = 0.10 // 10% jitter
	// minPlausibleExpiryYear guards against a poisoned expires_at. A refresh
	// response without expires_in used to be persisted as a zero time
	// ("0001-01-01T00:00:00Z"), which reads as "expired ~2000 years ago" and
	// therefore trips maxExpiryDuration — permanently parking the credential in
	// "skipping refresh" while every request 401s. Such timestamps are treated
	// as unknown (refresh now) instead of as ancient.
	minPlausibleExpiryYear = 2000
)

// tokenManager defines the interface for token refresh operations
type tokenManager interface {
	RefreshToken(ctx context.Context, userID string, issuer ai.Issuer, refreshToken string, opts ...oauth.Option) (*oauth.Token, error)
}

// providerConfig defines the interface for provider config operations used by OAuthRefresher
type providerConfig interface {
	ListOAuthProviders() ([]*typ.Provider, error)
	UpdateProvider(uuid string, provider *typ.Provider) error
}

// OAuthRefresher handles periodic OAuth token refresh with jitter to distribute
// load across multiple instances
type OAuthRefresher struct {
	manager       tokenManager
	serverConfig  providerConfig
	checkInterval time.Duration
	refreshBuffer time.Duration
	cancelFunc    context.CancelFunc
	mu            sync.RWMutex
	running       bool
	rng           *rand.Rand // Random number generator for jitter
}

type refresherOptions struct {
	manager       tokenManager
	serverConfig  providerConfig
	checkInterval time.Duration
	refreshBuffer time.Duration
	rng           *rand.Rand
}

// RefresherOption configures an OAuth token refresher.
type RefresherOption func(*refresherOptions)

// WithTokenManager sets the token manager used for refresh operations.
func WithTokenManager(manager tokenManager) RefresherOption {
	return func(o *refresherOptions) {
		o.manager = manager
	}
}

// WithProviderConfig sets the provider config used for persisted OAuth providers.
func WithProviderConfig(config providerConfig) RefresherOption {
	return func(o *refresherOptions) {
		o.serverConfig = config
	}
}

// WithCheckInterval sets how often the refresher checks for expiring tokens.
func WithCheckInterval(interval time.Duration) RefresherOption {
	return func(o *refresherOptions) {
		o.checkInterval = interval
	}
}

// WithRefreshBuffer sets how soon before expiry a token should be refreshed.
func WithRefreshBuffer(buffer time.Duration) RefresherOption {
	return func(o *refresherOptions) {
		o.refreshBuffer = buffer
	}
}

// WithRandSource sets the random generator used for interval jitter.
func WithRandSource(rng *rand.Rand) RefresherOption {
	return func(o *refresherOptions) {
		o.rng = rng
	}
}

// NewTokenRefresher creates a new token refresher.
func NewTokenRefresher(opts ...RefresherOption) *OAuthRefresher {
	options := &refresherOptions{
		checkInterval: defaultCheckInterval,
		refreshBuffer: defaultRefreshBuffer,
		rng:           rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	for _, opt := range opts {
		opt(options)
	}
	if options.checkInterval == 0 {
		options.checkInterval = defaultCheckInterval
	}
	if options.refreshBuffer == 0 {
		options.refreshBuffer = defaultRefreshBuffer
	}
	if options.rng == nil {
		options.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	return &OAuthRefresher{
		manager:       options.manager,
		serverConfig:  options.serverConfig,
		checkInterval: options.checkInterval,
		refreshBuffer: options.refreshBuffer,
		rng:           options.rng,
	}
}

// SetCheckInterval sets the check interval
func (tr *OAuthRefresher) SetCheckInterval(interval time.Duration) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.checkInterval = interval
}

// SetRefreshBuffer sets the refresh buffer
func (tr *OAuthRefresher) SetRefreshBuffer(buffer time.Duration) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.refreshBuffer = buffer
}

// Start begins the background token refresh loop
func (tr *OAuthRefresher) Start(ctx context.Context) {
	tr.mu.Lock()
	if tr.running {
		tr.mu.Unlock()
		return
	}
	tr.running = true
	tr.mu.Unlock()

	defer func() {
		tr.mu.Lock()
		tr.running = false
		tr.mu.Unlock()
	}()

	// Create a cancellable context for this run
	ctx, tr.cancelFunc = context.WithCancel(ctx)
	defer func() {
		tr.mu.Lock()
		tr.cancelFunc = nil
		tr.mu.Unlock()
	}()

	// Add jitter to distribute load across multiple instances
	jitter := time.Duration(tr.rng.Float64() * float64(tr.checkInterval) * jitterPercent)
	ticker := time.NewTicker(tr.checkInterval + jitter)
	defer ticker.Stop()

	logger := logrus.WithField("component", "OAuthRefresher")
	logger.WithField("checkInterval", tr.checkInterval+jitter).
		WithField("refreshBuffer", tr.refreshBuffer).
		Info("Starting OAuth token refresher")

	// Initial check on start
	tr.CheckAndRefreshTokens()

	for {
		select {
		case <-ctx.Done():
			logger.Info("OAuth refresher stopped")
			return
		case <-ticker.C:
			tr.CheckAndRefreshTokens()
		}
	}
}

// Stop stops the background token refresh loop
func (tr *OAuthRefresher) Stop() {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	if tr.running && tr.cancelFunc != nil {
		tr.cancelFunc()
	}
}

// Running returns true if the refresher is currently running
func (tr *OAuthRefresher) Running() bool {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	return tr.running
}

// CheckAndRefreshTokens checks all OAuth providers and refreshes tokens if needed
func (tr *OAuthRefresher) CheckAndRefreshTokens() {
	logger := logrus.WithField("component", "OAuthRefresher")

	// Recover from panics to prevent background goroutine crashes
	defer func() {
		if r := recover(); r != nil {
			logger.WithField("panic", r).Error("Panic in CheckAndRefreshTokens")
		}
	}()

	providers, err := tr.serverConfig.ListOAuthProviders()
	if err != nil {
		logger.Errorf("Failed to list providers: %v", err)
		return
	}

	tr.mu.RLock()
	buffer := tr.refreshBuffer
	tr.mu.RUnlock()

	now := time.Now()
	refreshCount := 0

	for _, provider := range providers {
		if provider.OAuthDetail == nil {
			continue
		}

		if provider.OAuthDetail.ExpiresAt == "" {
			continue
		}

		expiresAt, err := time.Parse(time.RFC3339, provider.OAuthDetail.ExpiresAt)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"provider": provider.Name,
				"error":    err,
			}).Error("Invalid expires_at format")
			continue
		}

		// Check if token needs refresh (sequential, not concurrent)
		// Refresh if token is expired OR will expire within buffer window
		// BUT skip if token expired too long ago (more than maxExpiryDuration)
		timeSinceExpiry := now.Sub(expiresAt)
		if expiresAt.Before(now.Add(buffer)) {
			// A timestamp that predates OAuth itself is a poisoned or unknown
			// expiry, not a credential abandoned for years. Refreshing it is how
			// such a row heals; skipping it is how a provider ends up 401ing
			// forever with nothing but a warning in the log.
			expiryIsPlausible := expiresAt.Year() >= minPlausibleExpiryYear
			if expiryIsPlausible && timeSinceExpiry > maxExpiryDuration {
				logger.WithFields(logrus.Fields{
					"provider":          provider.Name,
					"expiresAt":         provider.OAuthDetail.ExpiresAt,
					"timeSinceExpiry":   timeSinceExpiry,
					"maxExpiryDuration": maxExpiryDuration,
				}).Warn("Token expired too long ago, skipping refresh")
				continue
			}
			if !expiryIsPlausible {
				logger.WithFields(logrus.Fields{
					"provider":  provider.Name,
					"expiresAt": provider.OAuthDetail.ExpiresAt,
				}).Warn("Implausible expires_at, treating as unknown and refreshing")
			}
			tr.refreshProviderToken(provider)
			refreshCount++
		}
	}

	if refreshCount > 0 {
		logger.WithFields(logrus.Fields{
			"totalProviders": len(providers),
			"refreshed":      refreshCount,
		}).Info("OAuth token refresh completed")
	}
}

// refreshProviderToken refreshes a single provider's token
func (tr *OAuthRefresher) refreshProviderToken(provider *typ.Provider) {
	logger := logrus.WithFields(logrus.Fields{
		"component": "OAuthRefresher",
		"provider":  provider.Name,
	})

	issuer, err := oauth.ParseIssuer(provider.OAuthDetail.GetIssuer())
	if err != nil {
		logger.WithError(err).Error("Invalid provider type")
		return
	}

	refreshOpts := []oauth.Option{oauth.WithProxyString(provider.ProxyURL)}
	if issuer == ai.IssuerKimiCode && provider.OAuthDetail.DeviceID != "" {
		refreshOpts = append(refreshOpts, oauth.WithKimiDeviceID(provider.OAuthDetail.DeviceID))
	}
	token, err := tr.manager.RefreshToken(
		context.Background(),
		provider.OAuthDetail.UserID,
		issuer,
		provider.OAuthDetail.RefreshToken,
		refreshOpts...,
	)

	if err != nil {
		logger.WithField("issuer", issuer).
			WithField("expiresAt", provider.OAuthDetail.ExpiresAt).
			WithError(err).Error("Failed to refresh token")
		return
	}

	// Never persist a blank credential over a working one: writing "" here turns
	// a token that is merely near expiry into a guaranteed 401 on every request,
	// and the next cycle would happily overwrite it again.
	if token.AccessToken == "" {
		logger.WithField("issuer", issuer).Error("Refresh returned an empty access token, keeping the current credential")
		return
	}

	// Update provider with new token
	provider.OAuthDetail.AccessToken = token.AccessToken
	if token.RefreshToken != "" {
		provider.OAuthDetail.RefreshToken = token.RefreshToken
	}
	provider.OAuthDetail.ExpiresAt = formatExpiry(token.Expiry)
	// Keep the Codex id_token in lockstep with the access token, the same way
	// the manual POST /oauth/refresh path does — a background refresh that
	// leaves a stale identity assertion behind is an auth failure waiting to
	// happen on the next native-config export.
	if token.IDToken != "" {
		if provider.OAuthDetail.ExtraFields == nil {
			provider.OAuthDetail.ExtraFields = map[string]interface{}{}
		}
		provider.OAuthDetail.ExtraFields["id_token"] = token.IDToken
	}

	if err := tr.serverConfig.UpdateProvider(provider.UUID, provider); err != nil {
		logger.WithError(err).Error("Failed to update provider")
		return
	}

	// Note: Client pool invalidation is handled automatically by Config.UpdateProvider() via hooks
	logger.WithField("expiresAt", provider.OAuthDetail.ExpiresAt).Info("Token refreshed successfully")
}

// formatExpiry renders a token expiry for storage in OAuthDetail.ExpiresAt.
//
// A zero Expiry means the issuer returned no expires_in. Formatting it verbatim
// yields "0001-01-01T00:00:00Z", which every consumer then reads as "expired
// two millennia ago": IsExpired() is permanently true, and CheckAndRefreshTokens
// used to write the credential off as beyond maxExpiryDuration. The create path
// (createProviderFromToken) already stores "" — meaning "no known expiry" — so
// the refresh paths store the same thing.
func formatExpiry(expiry time.Time) string {
	if expiry.IsZero() {
		return ""
	}
	return expiry.Format(time.RFC3339)
}
