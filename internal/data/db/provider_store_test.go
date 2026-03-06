package db

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func setupTestProviderStore(t *testing.T) (*ProviderStore, string) {
	t.Helper()

	// Create a temporary directory for the test database
	tmpDir := t.TempDir()

	store, err := NewProviderStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create provider store: %v", err)
	}

	return store, tmpDir
}

func TestNewProviderStore(t *testing.T) {
	store, tmpDir := setupTestProviderStore(t)
	defer store.Close()

	// Verify database file was created
	dbPath := filepath.Join(tmpDir, "db", "tingly.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Errorf("Database file was not created at %s", dbPath)
	}

	// Verify store can count providers
	count, err := store.Count()
	if err != nil {
		t.Errorf("Failed to count providers: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 providers, got %d", count)
	}
}

func TestProviderSaveAndGetByUUID(t *testing.T) {
	store, _ := setupTestProviderStore(t)
	defer store.Close()

	// Create a test provider with API key auth
	provider := &typ.Provider{
		UUID:          "test-uuid-1",
		Name:          "test-provider",
		APIBase:       "https://api.openai.com/v1",
		APIStyle:      protocol.APIStyleOpenAI,
		AuthType:      typ.AuthTypeAPIKey,
		Token:         "sk-test-key-12345678",
		NoKeyRequired: false,
		Enabled:       true,
		Tags:          []string{"test", "openai"},
		Timeout:       300,
		ProxyURL:      "http://localhost:8080",
		LastUpdated:   time.Now().Format(time.RFC3339),
	}

	// Save provider
	if err := store.Save(provider); err != nil {
		t.Fatalf("Failed to save provider: %v", err)
	}

	// Get provider by UUID
	retrieved, err := store.GetByUUID("test-uuid-1")
	if err != nil {
		t.Fatalf("Failed to get provider: %v", err)
	}

	// Verify all fields
	if retrieved.UUID != provider.UUID {
		t.Errorf("UUID mismatch: got %s, want %s", retrieved.UUID, provider.UUID)
	}
	if retrieved.Name != provider.Name {
		t.Errorf("Name mismatch: got %s, want %s", retrieved.Name, provider.Name)
	}
	if retrieved.APIBase != provider.APIBase {
		t.Errorf("APIBase mismatch: got %s, want %s", retrieved.APIBase, provider.APIBase)
	}
	if retrieved.Token != provider.Token {
		t.Errorf("Token mismatch: got %s, want %s", retrieved.Token, provider.Token)
	}
	if retrieved.Enabled != provider.Enabled {
		t.Errorf("Enabled mismatch: got %v, want %v", retrieved.Enabled, provider.Enabled)
	}
	if retrieved.ProxyURL != provider.ProxyURL {
		t.Errorf("ProxyURL mismatch: got %s, want %s", retrieved.ProxyURL, provider.ProxyURL)
	}
	if retrieved.Timeout != provider.Timeout {
		t.Errorf("Timeout mismatch: got %d, want %d", retrieved.Timeout, provider.Timeout)
	}

	// Verify tags
	if len(retrieved.Tags) != len(provider.Tags) {
		t.Errorf("Tags length mismatch: got %d, want %d", len(retrieved.Tags), len(provider.Tags))
	}
}

func TestProviderSaveOAuth(t *testing.T) {
	store, _ := setupTestProviderStore(t)
	defer store.Close()

	// Create a test provider with OAuth auth
	expiresAt := time.Now().Add(1 * time.Hour)
	provider := &typ.Provider{
		UUID:     "test-oauth-uuid",
		Name:     "test-oauth-provider",
		APIBase:  "https://api.anthropic.com",
		APIStyle: protocol.APIStyleAnthropic,
		AuthType: typ.AuthTypeOAuth,
		OAuthDetail: &typ.OAuthDetail{
			AccessToken:  "test-access-token",
			ProviderType: "anthropic",
			UserID:       "test-user-id",
			RefreshToken: "test-refresh-token",
			ExpiresAt:    expiresAt.Format(time.RFC3339),
			ExtraFields:  map[string]interface{}{"scopes": []string{"read", "write"}},
		},
		Enabled:       true,
		NoKeyRequired: false,
	}

	// Save provider
	if err := store.Save(provider); err != nil {
		t.Fatalf("Failed to save OAuth provider: %v", err)
	}

	// Get provider by UUID
	retrieved, err := store.GetByUUID("test-oauth-uuid")
	if err != nil {
		t.Fatalf("Failed to get OAuth provider: %v", err)
	}

	// Verify OAuth fields
	if retrieved.AuthType != typ.AuthTypeOAuth {
		t.Errorf("AuthType mismatch: got %s, want %s", retrieved.AuthType, typ.AuthTypeOAuth)
	}
	if retrieved.OAuthDetail == nil {
		t.Fatal("OAuthDetail is nil")
	}
	if retrieved.OAuthDetail.AccessToken != provider.OAuthDetail.AccessToken {
		t.Errorf("AccessToken mismatch: got %s, want %s", retrieved.OAuthDetail.AccessToken, provider.OAuthDetail.AccessToken)
	}
	if retrieved.OAuthDetail.ProviderType != provider.OAuthDetail.ProviderType {
		t.Errorf("ProviderType mismatch: got %s, want %s", retrieved.OAuthDetail.ProviderType, provider.OAuthDetail.ProviderType)
	}
	if retrieved.OAuthDetail.UserID != provider.OAuthDetail.UserID {
		t.Errorf("UserID mismatch: got %s, want %s", retrieved.OAuthDetail.UserID, provider.OAuthDetail.UserID)
	}
	if retrieved.OAuthDetail.RefreshToken != provider.OAuthDetail.RefreshToken {
		t.Errorf("RefreshToken mismatch: got %s, want %s", retrieved.OAuthDetail.RefreshToken, provider.OAuthDetail.RefreshToken)
	}
	if retrieved.OAuthDetail.ExpiresAt != provider.OAuthDetail.ExpiresAt {
		t.Errorf("ExpiresAt mismatch: got %s, want %s", retrieved.OAuthDetail.ExpiresAt, provider.OAuthDetail.ExpiresAt)
	}
}

func TestProviderUpdate(t *testing.T) {
	store, _ := setupTestProviderStore(t)
	defer store.Close()

	// Create and save a provider
	provider := &typ.Provider{
		UUID:     "test-update-uuid",
		Name:     "original-name",
		APIBase:  "https://api.openai.com/v1",
		APIStyle: protocol.APIStyleOpenAI,
		AuthType: typ.AuthTypeAPIKey,
		Token:    "original-token",
		Enabled:  true,
	}

	if err := store.Save(provider); err != nil {
		t.Fatalf("Failed to save provider: %v", err)
	}

	// Update the provider
	provider.Name = "updated-name"
	provider.Token = "updated-token"
	provider.Enabled = false

	if err := store.Save(provider); err != nil {
		t.Fatalf("Failed to update provider: %v", err)
	}

	// Verify the update
	retrieved, err := store.GetByUUID("test-update-uuid")
	if err != nil {
		t.Fatalf("Failed to get updated provider: %v", err)
	}

	if retrieved.Name != "updated-name" {
		t.Errorf("Name not updated: got %s, want %s", retrieved.Name, "updated-name")
	}
	if retrieved.Token != "updated-token" {
		t.Errorf("Token not updated: got %s, want %s", retrieved.Token, "updated-token")
	}
	if retrieved.Enabled != false {
		t.Errorf("Enabled not updated: got %v, want %v", retrieved.Enabled, false)
	}
}

func TestProviderDelete(t *testing.T) {
	store, _ := setupTestProviderStore(t)
	defer store.Close()

	// Create and save a provider
	provider := &typ.Provider{
		UUID:     "test-delete-uuid",
		Name:     "test-delete-provider",
		APIBase:  "https://api.openai.com/v1",
		APIStyle: protocol.APIStyleOpenAI,
		AuthType: typ.AuthTypeAPIKey,
		Token:    "test-token",
		Enabled:  true,
	}

	if err := store.Save(provider); err != nil {
		t.Fatalf("Failed to save provider: %v", err)
	}

	// Verify it exists
	if !store.Exists("test-delete-uuid") {
		t.Error("Provider should exist before delete")
	}

	// Delete the provider
	if err := store.Delete("test-delete-uuid"); err != nil {
		t.Fatalf("Failed to delete provider: %v", err)
	}

	// Verify it's gone
	if store.Exists("test-delete-uuid") {
		t.Error("Provider should not exist after delete")
	}

	// Verify we can't get it
	_, err := store.GetByUUID("test-delete-uuid")
	if err == nil {
		t.Error("Expected error when getting deleted provider")
	}
}

func TestProviderList(t *testing.T) {
	store, _ := setupTestProviderStore(t)
	defer store.Close()

	// Create multiple providers
	providers := []*typ.Provider{
		{
			UUID:     "list-uuid-1",
			Name:     "provider-1",
			APIBase:  "https://api.openai.com/v1",
			APIStyle: protocol.APIStyleOpenAI,
			AuthType: typ.AuthTypeAPIKey,
			Token:    "token-1",
			Enabled:  true,
		},
		{
			UUID:     "list-uuid-2",
			Name:     "provider-2",
			APIBase:  "https://api.anthropic.com",
			APIStyle: protocol.APIStyleAnthropic,
			AuthType: typ.AuthTypeAPIKey,
			Token:    "token-2",
			Enabled:  true,
		},
		{
			UUID:     "list-uuid-3",
			Name:     "provider-3",
			APIBase:  "https://api.openai.com/v1",
			APIStyle: protocol.APIStyleOpenAI,
			AuthType: typ.AuthTypeOAuth,
			OAuthDetail: &typ.OAuthDetail{
				AccessToken:  "oauth-token-3",
				ProviderType: "anthropic",
			},
			Enabled: true,
		},
	}

	for _, p := range providers {
		if err := store.Save(p); err != nil {
			t.Fatalf("Failed to save provider: %v", err)
		}
	}

	// List all providers
	list, err := store.List()
	if err != nil {
		t.Fatalf("Failed to list providers: %v", err)
	}

	if len(list) != 3 {
		t.Errorf("Expected 3 providers, got %d", len(list))
	}

	// List OAuth providers
	oauthList, err := store.ListOAuth()
	if err != nil {
		t.Fatalf("Failed to list OAuth providers: %v", err)
	}

	if len(oauthList) != 1 {
		t.Errorf("Expected 1 OAuth provider, got %d", len(oauthList))
	}

	// List enabled providers
	enabledList, err := store.ListEnabled()
	if err != nil {
		t.Fatalf("Failed to list enabled providers: %v", err)
	}

	if len(enabledList) != 3 {
		t.Errorf("Expected 3 enabled providers, got %d", len(enabledList))
	}
}

func TestProviderGetByName(t *testing.T) {
	store, _ := setupTestProviderStore(t)
	defer store.Close()

	// Create a provider
	provider := &typ.Provider{
		UUID:     "getbyuuid-1",
		Name:     "unique-provider-name",
		APIBase:  "https://api.openai.com/v1",
		APIStyle: protocol.APIStyleOpenAI,
		AuthType: typ.AuthTypeAPIKey,
		Token:    "test-token",
		Enabled:  true,
	}

	if err := store.Save(provider); err != nil {
		t.Fatalf("Failed to save provider: %v", err)
	}

	// Get by name
	retrieved, err := store.GetByName("unique-provider-name")
	if err != nil {
		t.Fatalf("Failed to get provider by name: %v", err)
	}

	if retrieved.UUID != provider.UUID {
		t.Errorf("UUID mismatch: got %s, want %s", retrieved.UUID, provider.UUID)
	}

	// Try to get non-existent provider
	_, err = store.GetByName("non-existent-provider")
	if err == nil {
		t.Error("Expected error when getting non-existent provider by name")
	}
}

func TestProviderUpdateCredential(t *testing.T) {
	store, _ := setupTestProviderStore(t)
	defer store.Close()

	// Create a provider
	provider := &typ.Provider{
		UUID:     "update-cred-uuid",
		Name:     "credential-test-provider",
		APIBase:  "https://api.openai.com/v1",
		APIStyle: protocol.APIStyleOpenAI,
		AuthType: typ.AuthTypeAPIKey,
		Token:    "original-token",
		Enabled:  true,
	}

	if err := store.Save(provider); err != nil {
		t.Fatalf("Failed to save provider: %v", err)
	}

	// Update credential
	if err := store.UpdateCredential("update-cred-uuid", "new-token", nil); err != nil {
		t.Fatalf("Failed to update credential: %v", err)
	}

	// Verify the update
	retrieved, err := store.GetByUUID("update-cred-uuid")
	if err != nil {
		t.Fatalf("Failed to get provider: %v", err)
	}

	if retrieved.Token != "new-token" {
		t.Errorf("Token not updated: got %s, want %s", retrieved.Token, "new-token")
	}
}

func TestProviderUpdateOAuthCredential(t *testing.T) {
	store, _ := setupTestProviderStore(t)
	defer store.Close()

	// Create an OAuth provider
	expiresAt := time.Now().Add(1 * time.Hour)
	provider := &typ.Provider{
		UUID:     "update-oauth-uuid",
		Name:     "oauth-credential-test",
		APIBase:  "https://api.anthropic.com",
		APIStyle: protocol.APIStyleAnthropic,
		AuthType: typ.AuthTypeOAuth,
		OAuthDetail: &typ.OAuthDetail{
			AccessToken:  "original-access-token",
			ProviderType: "anthropic",
			UserID:       "user-123",
			RefreshToken: "original-refresh-token",
			ExpiresAt:    expiresAt.Format(time.RFC3339),
		},
		Enabled: true,
	}

	if err := store.Save(provider); err != nil {
		t.Fatalf("Failed to save OAuth provider: %v", err)
	}

	// Update OAuth credential
	newExpiresAt := time.Now().Add(2 * time.Hour)
	newOAuthDetail := &typ.OAuthDetail{
		AccessToken:  "new-access-token",
		ProviderType: "anthropic",
		UserID:       "user-123",
		RefreshToken: "new-refresh-token",
		ExpiresAt:    newExpiresAt.Format(time.RFC3339),
	}

	if err := store.UpdateCredential("update-oauth-uuid", "", newOAuthDetail); err != nil {
		t.Fatalf("Failed to update OAuth credential: %v", err)
	}

	// Verify the update
	retrieved, err := store.GetByUUID("update-oauth-uuid")
	if err != nil {
		t.Fatalf("Failed to get provider: %v", err)
	}

	if retrieved.OAuthDetail.AccessToken != "new-access-token" {
		t.Errorf("AccessToken not updated: got %s, want %s", retrieved.OAuthDetail.AccessToken, "new-access-token")
	}
	if retrieved.OAuthDetail.RefreshToken != "new-refresh-token" {
		t.Errorf("RefreshToken not updated: got %s, want %s", retrieved.OAuthDetail.RefreshToken, "new-refresh-token")
	}
}

func TestProviderToolInterceptor(t *testing.T) {
	store, _ := setupTestProviderStore(t)
	defer store.Close()

	// Create a provider with tool interceptor config
	provider := &typ.Provider{
		UUID:     "tool-interceptor-uuid",
		Name:     "tool-interceptor-test",
		APIBase:  "https://api.openai.com/v1",
		APIStyle: protocol.APIStyleOpenAI,
		AuthType: typ.AuthTypeAPIKey,
		Token:    "test-token",
		Enabled:  true,
		ToolInterceptor: &typ.ToolInterceptorConfig{
			PreferLocalSearch: true,
			SearchAPI:         "brave",
			SearchKey:         "brave-api-key",
			MaxResults:        20,
			ProxyURL:          "http://localhost:8888",
			MaxFetchSize:      2 * 1024 * 1024,
			FetchTimeout:      60,
			MaxURLLength:      3000,
		},
	}

	if err := store.Save(provider); err != nil {
		t.Fatalf("Failed to save provider: %v", err)
	}

	// Retrieve and verify
	retrieved, err := store.GetByUUID("tool-interceptor-uuid")
	if err != nil {
		t.Fatalf("Failed to get provider: %v", err)
	}

	if retrieved.ToolInterceptor == nil {
		t.Fatal("ToolInterceptor is nil")
	}

	if retrieved.ToolInterceptor.SearchAPI != "brave" {
		t.Errorf("SearchAPI mismatch: got %s, want %s", retrieved.ToolInterceptor.SearchAPI, "brave")
	}
	if retrieved.ToolInterceptor.SearchKey != "brave-api-key" {
		t.Errorf("SearchKey mismatch: got %s, want %s", retrieved.ToolInterceptor.SearchKey, "brave-api-key")
	}
	if retrieved.ToolInterceptor.MaxResults != 20 {
		t.Errorf("MaxResults mismatch: got %d, want %d", retrieved.ToolInterceptor.MaxResults, 20)
	}
}

func TestProviderIsOAuthExpired(t *testing.T) {
	store, _ := setupTestProviderStore(t)
	defer store.Close()

	// Create an OAuth provider with expired token
	pastTime := time.Now().Add(-1 * time.Hour)
	provider := &typ.Provider{
		UUID:     "expired-oauth-uuid",
		Name:     "expired-oauth-test",
		APIBase:  "https://api.anthropic.com",
		APIStyle: protocol.APIStyleAnthropic,
		AuthType: typ.AuthTypeOAuth,
		OAuthDetail: &typ.OAuthDetail{
			AccessToken:  "test-token",
			ProviderType: "anthropic",
			UserID:       "user-123",
			ExpiresAt:    pastTime.Format(time.RFC3339),
		},
		Enabled: true,
	}

	if err := store.Save(provider); err != nil {
		t.Fatalf("Failed to save provider: %v", err)
	}

	// Check if expired
	expired, err := store.IsOAuthExpired("expired-oauth-uuid")
	if err != nil {
		t.Fatalf("Failed to check OAuth expiration: %v", err)
	}

	if !expired {
		t.Error("Expected OAuth token to be expired")
	}

	// Update with future expiration
	futureTime := time.Now().Add(1 * time.Hour)
	newOAuthDetail := &typ.OAuthDetail{
		AccessToken:  "new-token",
		ProviderType: "anthropic",
		UserID:       "user-123",
		ExpiresAt:    futureTime.Format(time.RFC3339),
	}

	if err := store.UpdateCredential("expired-oauth-uuid", "", newOAuthDetail); err != nil {
		t.Fatalf("Failed to update OAuth credential: %v", err)
	}

	// Check if not expired
	expired, err = store.IsOAuthExpired("expired-oauth-uuid")
	if err != nil {
		t.Fatalf("Failed to check OAuth expiration: %v", err)
	}

	if expired {
		t.Error("Expected OAuth token to not be expired")
	}
}

func TestProviderUpdateOAuthAccessToken(t *testing.T) {
	store, _ := setupTestProviderStore(t)
	defer store.Close()

	// Create an OAuth provider
	provider := &typ.Provider{
		UUID:     "update-access-token-uuid",
		Name:     "update-access-token-test",
		APIBase:  "https://api.anthropic.com",
		APIStyle: protocol.APIStyleAnthropic,
		AuthType: typ.AuthTypeOAuth,
		OAuthDetail: &typ.OAuthDetail{
			AccessToken:  "old-access-token",
			ProviderType: "anthropic",
			UserID:       "user-123",
		},
		Enabled: true,
	}

	if err := store.Save(provider); err != nil {
		t.Fatalf("Failed to save provider: %v", err)
	}

	// Update only the access token
	if err := store.UpdateOAuthAccessToken("update-access-token-uuid", "new-access-token"); err != nil {
		t.Fatalf("Failed to update OAuth access token: %v", err)
	}

	// Verify the update
	retrieved, err := store.GetByUUID("update-access-token-uuid")
	if err != nil {
		t.Fatalf("Failed to get provider: %v", err)
	}

	if retrieved.OAuthDetail.AccessToken != "new-access-token" {
		t.Errorf("AccessToken not updated: got %s, want %s", retrieved.OAuthDetail.AccessToken, "new-access-token")
	}

	// Verify other OAuth fields are preserved
	if retrieved.OAuthDetail.ProviderType != "anthropic" {
		t.Errorf("ProviderType not preserved: got %s, want %s", retrieved.OAuthDetail.ProviderType, "anthropic")
	}
	if retrieved.OAuthDetail.UserID != "user-123" {
		t.Errorf("UserID not preserved: got %s, want %s", retrieved.OAuthDetail.UserID, "user-123")
	}
}

func TestProviderCount(t *testing.T) {
	store, _ := setupTestProviderStore(t)
	defer store.Close()

	// Initially 0 providers
	count, err := store.Count()
	if err != nil {
		t.Fatalf("Failed to count providers: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 providers, got %d", count)
	}

	// Add some providers
	for i := 0; i < 5; i++ {
		provider := &typ.Provider{
			UUID:     fmt.Sprintf("count-uuid-%d", i),
			Name:     fmt.Sprintf("provider-%d", i),
			APIBase:  "https://api.openai.com/v1",
			APIStyle: protocol.APIStyleOpenAI,
			AuthType: typ.AuthTypeAPIKey,
			Token:    fmt.Sprintf("token-%d", i),
			Enabled:  true,
		}
		if err := store.Save(provider); err != nil {
			t.Fatalf("Failed to save provider: %v", err)
		}
	}

	// Now should have 5 providers
	count, err = store.Count()
	if err != nil {
		t.Fatalf("Failed to count providers: %v", err)
	}
	if count != 5 {
		t.Errorf("Expected 5 providers, got %d", count)
	}
}
