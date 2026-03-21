package background

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/typ"
	oauth2 "github.com/tingly-dev/tingly-box/pkg/oauth"
)

const (
	// defaultCheckInterval is how often to check for tokens needing refresh
	defaultCheckInterval = 10 * time.Minute
	// defaultRefreshBuffer is how long before expiry to refresh a token (matches OAuth package default)
	defaultRefreshBuffer = 5 * time.Minute
	// jitterPercent is the maximum jitter percentage to add to the check interval
	jitterPercent = 0.10 // 10% jitter
)

// tokenManager defines the interface for token refresh operations
type tokenManager interface {
	RefreshToken(ctx context.Context, userID string, providerType oauth2.ProviderType, refreshToken string, opts ...oauth2.Option) (*oauth2.Token, error)
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

// NewTokenRefresher creates a new token refresher
func NewTokenRefresher(manager *oauth2.Manager, serverConfig providerConfig) *OAuthRefresher {
	return &OAuthRefresher{
		manager:       manager,
		serverConfig:  serverConfig,
		checkInterval: defaultCheckInterval,
		refreshBuffer: defaultRefreshBuffer,
		rng:           rand.New(rand.NewSource(time.Now().UnixNano())),
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

		expiresAt, err := time.Parse(time.RFC3339, provider.OAuthDetail.ExpiresAt)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"provider": provider.Name,
				"error":    err,
			}).Error("Invalid expires_at format")
			continue
		}

		// Check if token needs refresh (sequential, not concurrent)
		if expiresAt.Before(now.Add(buffer)) {
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

	providerType, err := oauth2.ParseProviderType(provider.OAuthDetail.ProviderType)
	if err != nil {
		logger.WithError(err).Error("Invalid provider type")
		return
	}

	token, err := tr.manager.RefreshToken(
		context.Background(),
		provider.OAuthDetail.UserID,
		providerType,
		provider.OAuthDetail.RefreshToken,
		oauth2.WithProxyString(provider.ProxyURL),
	)

	if err != nil {
		logger.WithError(err).Error("Failed to refresh token")
		return
	}

	// Update provider with new token
	provider.OAuthDetail.AccessToken = token.AccessToken
	if token.RefreshToken != "" {
		provider.OAuthDetail.RefreshToken = token.RefreshToken
	}
	provider.OAuthDetail.ExpiresAt = token.Expiry.Format(time.RFC3339)

	if err := tr.serverConfig.UpdateProvider(provider.UUID, provider); err != nil {
		logger.WithError(err).Error("Failed to update provider")
		return
	}

	logger.WithField("expiresAt", provider.OAuthDetail.ExpiresAt).Info("Token refreshed successfully")
}
