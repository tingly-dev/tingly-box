package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"tingly-box/internal/config"
	oauth2 "tingly-box/pkg/oauth"
)

// TokenRefresher handles periodic OAuth token refresh
type TokenRefresher struct {
	manager       *oauth2.Manager
	serverConfig  *config.Config
	checkInterval time.Duration // Check every 10 minutes
	refreshBuffer time.Duration // Refresh if expires within 5 minutes
	stopChan      chan struct{}
	mu            sync.RWMutex
	running       bool
}

// NewTokenRefresher creates a new token refresher
func NewTokenRefresher(manager *oauth2.Manager, serverConfig *config.Config) *TokenRefresher {
	return &TokenRefresher{
		manager:       manager,
		serverConfig:  serverConfig,
		checkInterval: 10 * time.Minute,
		refreshBuffer: 30 * time.Minute,
		stopChan:      make(chan struct{}),
	}
}

// SetCheckInterval sets the check interval
func (tr *TokenRefresher) SetCheckInterval(interval time.Duration) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.checkInterval = interval
}

// SetRefreshBuffer sets the refresh buffer
func (tr *TokenRefresher) SetRefreshBuffer(buffer time.Duration) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.refreshBuffer = buffer
}

// Start begins the background token refresh loop
func (tr *TokenRefresher) Start(ctx context.Context) {
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

	ticker := time.NewTicker(tr.checkInterval)
	defer ticker.Stop()

	// Initial check on start
	tr.CheckAndRefreshTokens()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tr.stopChan:
			return
		case <-ticker.C:
			tr.CheckAndRefreshTokens()
		}
	}
}

// Stop stops the background token refresh loop
func (tr *TokenRefresher) Stop() {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	if tr.running {
		close(tr.stopChan)
		tr.stopChan = make(chan struct{})
	}
}

// CheckAndRefreshTokens checks all OAuth providers and refreshes tokens if needed
func (tr *TokenRefresher) CheckAndRefreshTokens() {
	providers, err := tr.serverConfig.ListOAuthProviders()
	if err != nil {
		fmt.Printf("[TokenRefresher] Failed to list providers: %v\n", err)
		return
	}

	now := time.Now()
	refreshCount := 0

	for _, provider := range providers {
		if provider.OAuthDetail == nil {
			continue
		}

		expiresAt, err := time.Parse(time.RFC3339, provider.OAuthDetail.ExpiresAt)
		if err != nil {
			fmt.Printf("[TokenRefresher] Invalid expires_at for %s: %v\n", provider.Name, err)
			continue
		}

		// Check if token needs refresh (sequential, not concurrent)
		if expiresAt.Before(now.Add(tr.refreshBuffer)) {
			tr.refreshProviderToken(provider)
			refreshCount++
		}
	}

	if refreshCount > 0 {
		fmt.Printf("[TokenRefresher] Checked %d OAuth providers, refreshed %d tokens\n", len(providers), refreshCount)
	}
}

// refreshProviderToken refreshes a single provider's token
func (tr *TokenRefresher) refreshProviderToken(provider *config.Provider) {
	providerType, err := oauth2.ParseProviderType(provider.OAuthDetail.ProviderType)
	if err != nil {
		fmt.Printf("[TokenRefresher] Invalid provider type for %s: %v\n", provider.Name, err)
		return
	}

	token, err := tr.manager.RefreshToken(
		context.Background(),
		provider.OAuthDetail.UserID,
		providerType,
		provider.OAuthDetail.RefreshToken,
	)

	if err != nil {
		fmt.Printf("[TokenRefresher] Failed to refresh %s: %v\n", provider.Name, err)
		return
	}

	// Update provider with new token
	provider.OAuthDetail.AccessToken = token.AccessToken
	if token.RefreshToken != "" {
		provider.OAuthDetail.RefreshToken = token.RefreshToken
	}
	provider.OAuthDetail.ExpiresAt = token.Expiry.Format(time.RFC3339)

	if err := tr.serverConfig.UpdateProvider(provider.UUID, provider); err != nil {
		fmt.Printf("[TokenRefresher] Failed to update %s: %v\n", provider.Name, err)
		return
	}

	fmt.Printf("[TokenRefresher] Refreshed token for %s (expires at %s)\n", provider.Name, provider.OAuthDetail.ExpiresAt)
}
