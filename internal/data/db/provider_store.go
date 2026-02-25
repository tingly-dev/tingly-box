package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ProviderRecord is the GORM model for persisting a complete provider
// This includes both configuration and credentials as one logical entity
type ProviderRecord struct {
	UUID     string `gorm:"primaryKey;column:uuid"`
	Name     string `gorm:"column:name;not null;index"`
	APIBase  string `gorm:"column:api_base;not null"`
	APIStyle string `gorm:"column:api_style;not null"` // "openai" or "anthropic"
	AuthType string `gorm:"column:auth_type;not null"` // "api_key" or "oauth"

	// Configuration fields
	NoKeyRequired bool   `gorm:"column:no_key_required;default:false"`
	Enabled       bool   `gorm:"column:enabled;default:true"`
	ProxyURL      string `gorm:"column:proxy_url"`
	Timeout       int64  `gorm:"column:timeout"`
	Tags          string `gorm:"column:tags;type:text"` // JSON array
	LastUpdated   string `gorm:"column:last_updated"`

	// Credential fields - stored with provider as a unit
	// For api_key auth: stores the API key
	// For oauth auth: stores OAuth access token
	Token             string `gorm:"column:token"`                        // API key or access token
	OAuthProviderType string `gorm:"column:oauth_provider_type"`          // For oauth: provider type
	OAuthUserID       string `gorm:"column:oauth_user_id"`                // For oauth: user ID
	OAuthRefreshToken string `gorm:"column:oauth_refresh_token"`          // For oauth: refresh token
	OAuthExpiresAt    string `gorm:"column:oauth_expires_at"`             // For oauth: token expiration (RFC3339)
	OAuthExtraFields  string `gorm:"column:oauth_extra_fields;type:text"` // For oauth: JSON

	// Tool interceptor config
	ToolInterceptorPreferLocalSearch bool   `gorm:"column:tool_interceptor_prefer_local_search;default:false"`
	ToolInterceptorSearchAPI         string `gorm:"column:tool_interceptor_search_api"`
	ToolInterceptorSearchKey         string `gorm:"column:tool_interceptor_search_key"`
	ToolInterceptorMaxResults        int    `gorm:"column:tool_interceptor_max_results"`
	ToolInterceptorProxyURL          string `gorm:"column:tool_interceptor_proxy_url"`
	ToolInterceptorMaxFetchSize      int64  `gorm:"column:tool_interceptor_max_fetch_size"`
	ToolInterceptorFetchTimeout      int64  `gorm:"column:tool_interceptor_fetch_timeout"`
	ToolInterceptorMaxURLLength      int    `gorm:"column:tool_interceptor_max_url_length"`
	ToolInterceptorDisabled          bool   `gorm:"column:tool_interceptor_disabled;default:false"`

	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// TableName specifies the table name for GORM
func (ProviderRecord) TableName() string {
	return "providers"
}

// toProvider converts a ProviderRecord to typ.Provider
func (r *ProviderRecord) toProvider() *typ.Provider {
	provider := &typ.Provider{
		UUID:          r.UUID,
		Name:          r.Name,
		APIBase:       r.APIBase,
		APIStyle:      protocol.APIStyle(r.APIStyle),
		AuthType:      typ.AuthType(r.AuthType),
		NoKeyRequired: r.NoKeyRequired,
		Enabled:       r.Enabled,
		ProxyURL:      r.ProxyURL,
		Timeout:       r.Timeout,
		LastUpdated:   r.LastUpdated,
	}

	// Parse tags JSON
	if r.Tags != "" {
		json.Unmarshal([]byte(r.Tags), &provider.Tags)
	}

	// Set credentials based on auth type
	switch provider.AuthType {
	case typ.AuthTypeOAuth:
		provider.OAuthDetail = &typ.OAuthDetail{
			AccessToken:  r.Token,
			ProviderType: r.OAuthProviderType,
			UserID:       r.OAuthUserID,
			RefreshToken: r.OAuthRefreshToken,
			ExpiresAt:    r.OAuthExpiresAt,
		}
		if r.OAuthExtraFields != "" {
			json.Unmarshal([]byte(r.OAuthExtraFields), &provider.OAuthDetail.ExtraFields)
		}
	case typ.AuthTypeAPIKey, "":
		provider.Token = r.Token
	}

	// Parse tool interceptor config
	if r.ToolInterceptorSearchAPI != "" || r.ToolInterceptorSearchKey != "" || r.ToolInterceptorMaxResults != 0 {
		provider.ToolInterceptor = &typ.ToolInterceptorConfig{
			PreferLocalSearch: r.ToolInterceptorPreferLocalSearch,
			SearchAPI:         r.ToolInterceptorSearchAPI,
			SearchKey:         r.ToolInterceptorSearchKey,
			MaxResults:        r.ToolInterceptorMaxResults,
			ProxyURL:          r.ToolInterceptorProxyURL,
			MaxFetchSize:      r.ToolInterceptorMaxFetchSize,
			FetchTimeout:      r.ToolInterceptorFetchTimeout,
			MaxURLLength:      r.ToolInterceptorMaxURLLength,
		}
	}

	if r.ToolInterceptorDisabled {
		if provider.ToolInterceptorOverride == nil {
			provider.ToolInterceptorOverride = &typ.ToolInterceptorOverride{}
		}
		provider.ToolInterceptorOverride.Disabled = true
	}

	return provider
}

// toRecord converts a typ.Provider to ProviderRecord
func toRecord(p *typ.Provider) *ProviderRecord {
	now := time.Now()

	record := &ProviderRecord{
		UUID:          p.UUID,
		Name:          p.Name,
		APIBase:       p.APIBase,
		APIStyle:      string(p.APIStyle),
		AuthType:      string(p.AuthType),
		NoKeyRequired: p.NoKeyRequired,
		Enabled:       p.Enabled,
		ProxyURL:      p.ProxyURL,
		Timeout:       p.Timeout,
		LastUpdated:   p.LastUpdated,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	// Initialize OAuth fields if OAuthDetail exists
	if p.OAuthDetail != nil {
		record.OAuthProviderType = p.OAuthDetail.ProviderType
		record.OAuthUserID = p.OAuthDetail.UserID
		record.OAuthExpiresAt = p.OAuthDetail.ExpiresAt
	}

	// Marshal tags to JSON
	if len(p.Tags) > 0 {
		tagsJSON, _ := json.Marshal(p.Tags)
		record.Tags = string(tagsJSON)
	}

	// Set credentials based on auth type
	switch p.AuthType {
	case typ.AuthTypeOAuth:
		if p.OAuthDetail != nil {
			record.Token = p.OAuthDetail.AccessToken
			record.OAuthRefreshToken = p.OAuthDetail.RefreshToken
			if p.OAuthDetail.ExtraFields != nil {
				extraJSON, _ := json.Marshal(p.OAuthDetail.ExtraFields)
				record.OAuthExtraFields = string(extraJSON)
			}
		}
	case typ.AuthTypeAPIKey, "":
		record.Token = p.Token
	}

	// Marshal tool interceptor config
	if p.ToolInterceptor != nil {
		record.ToolInterceptorPreferLocalSearch = p.ToolInterceptor.PreferLocalSearch
		record.ToolInterceptorSearchAPI = p.ToolInterceptor.SearchAPI
		record.ToolInterceptorSearchKey = p.ToolInterceptor.SearchKey
		record.ToolInterceptorMaxResults = p.ToolInterceptor.MaxResults
		record.ToolInterceptorProxyURL = p.ToolInterceptor.ProxyURL
		record.ToolInterceptorMaxFetchSize = p.ToolInterceptor.MaxFetchSize
		record.ToolInterceptorFetchTimeout = p.ToolInterceptor.FetchTimeout
		record.ToolInterceptorMaxURLLength = p.ToolInterceptor.MaxURLLength
	}

	if p.ToolInterceptorOverride != nil {
		record.ToolInterceptorDisabled = p.ToolInterceptorOverride.Disabled
	}

	return record
}

// updateRecordFromProvider updates an existing ProviderRecord from typ.Provider
func updateRecordFromProvider(record *ProviderRecord, p *typ.Provider) {
	record.Name = p.Name
	record.APIBase = p.APIBase
	record.APIStyle = string(p.APIStyle)
	record.AuthType = string(p.AuthType)
	record.NoKeyRequired = p.NoKeyRequired
	record.Enabled = p.Enabled
	record.ProxyURL = p.ProxyURL
	record.Timeout = p.Timeout
	record.LastUpdated = p.LastUpdated
	record.UpdatedAt = time.Now()

	// Marshal tags to JSON
	if len(p.Tags) > 0 {
		tagsJSON, _ := json.Marshal(p.Tags)
		record.Tags = string(tagsJSON)
	} else {
		record.Tags = ""
	}

	// Set credentials based on auth type
	switch p.AuthType {
	case typ.AuthTypeOAuth:
		if p.OAuthDetail != nil {
			record.Token = p.OAuthDetail.AccessToken
			record.OAuthProviderType = p.OAuthDetail.ProviderType
			record.OAuthUserID = p.OAuthDetail.UserID
			record.OAuthRefreshToken = p.OAuthDetail.RefreshToken
			record.OAuthExpiresAt = p.OAuthDetail.ExpiresAt
			if p.OAuthDetail.ExtraFields != nil {
				extraJSON, _ := json.Marshal(p.OAuthDetail.ExtraFields)
				record.OAuthExtraFields = string(extraJSON)
			} else {
				record.OAuthExtraFields = ""
			}
		}
	case typ.AuthTypeAPIKey, "":
		record.Token = p.Token
		record.OAuthProviderType = ""
		record.OAuthUserID = ""
		record.OAuthRefreshToken = ""
		record.OAuthExpiresAt = ""
		record.OAuthExtraFields = ""
	}

	// Marshal tool interceptor config
	if p.ToolInterceptor != nil {
		record.ToolInterceptorPreferLocalSearch = p.ToolInterceptor.PreferLocalSearch
		record.ToolInterceptorSearchAPI = p.ToolInterceptor.SearchAPI
		record.ToolInterceptorSearchKey = p.ToolInterceptor.SearchKey
		record.ToolInterceptorMaxResults = p.ToolInterceptor.MaxResults
		record.ToolInterceptorProxyURL = p.ToolInterceptor.ProxyURL
		record.ToolInterceptorMaxFetchSize = p.ToolInterceptor.MaxFetchSize
		record.ToolInterceptorFetchTimeout = p.ToolInterceptor.FetchTimeout
		record.ToolInterceptorMaxURLLength = p.ToolInterceptor.MaxURLLength
	} else {
		record.ToolInterceptorSearchAPI = ""
		record.ToolInterceptorSearchKey = ""
		record.ToolInterceptorMaxResults = 0
		record.ToolInterceptorProxyURL = ""
		record.ToolInterceptorMaxFetchSize = 0
		record.ToolInterceptorFetchTimeout = 0
		record.ToolInterceptorMaxURLLength = 0
	}

	if p.ToolInterceptorOverride != nil {
		record.ToolInterceptorDisabled = p.ToolInterceptorOverride.Disabled
	} else {
		record.ToolInterceptorDisabled = false
	}
}

// ProviderStore manages providers as complete units (configuration + credentials)
type ProviderStore struct {
	db     *gorm.DB
	dbPath string
	mu     sync.Mutex
}

// NewProviderStore creates or loads a provider store using SQLite database.
func NewProviderStore(baseDir string) (*ProviderStore, error) {
	logrus.Debugf("Initializing provider store in directory: %s", baseDir)
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create provider store directory: %w", err)
	}

	dbPath := constant.GetDBFile(baseDir)
	// Ensure the db subdirectory exists
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	logrus.Debugf("Opening SQLite database for provider store: %s", dbPath)
	dsn := dbPath + "?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=1"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open provider database: %w", err)
	}
	logrus.Debugf("SQLite database opened successfully for provider store")

	store := &ProviderStore{
		db:     db,
		dbPath: dbPath,
	}

	// Auto-migrate schema
	if err := db.AutoMigrate(&ProviderRecord{}); err != nil {
		return nil, fmt.Errorf("failed to migrate provider database: %w", err)
	}
	logrus.Debugf("Provider store initialization completed")

	return store, nil
}

// Save saves a provider (create or update)
func (ps *ProviderStore) Save(provider *typ.Provider) error {
	if provider == nil {
		return errors.New("provider cannot be nil")
	}
	if provider.UUID == "" {
		return errors.New("provider UUID cannot be empty")
	}

	ps.mu.Lock()
	defer ps.mu.Unlock()

	var existing ProviderRecord
	err := ps.db.Where("uuid = ?", provider.UUID).First(&existing).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Create new record
		record := toRecord(provider)
		if err := ps.db.Create(record).Error; err != nil {
			return fmt.Errorf("failed to create provider record: %w", err)
		}
		logrus.Debugf("Created new provider: %s (%s)", provider.Name, provider.UUID)
	} else if err != nil {
		return fmt.Errorf("failed to query existing provider: %w", err)
	} else {
		// Update existing record
		updateRecordFromProvider(&existing, provider)
		if err := ps.db.Save(&existing).Error; err != nil {
			return fmt.Errorf("failed to update provider record: %w", err)
		}
		logrus.Debugf("Updated provider: %s (%s)", provider.Name, provider.UUID)
	}

	return nil
}

// GetByUUID returns a provider by UUID
func (ps *ProviderStore) GetByUUID(uuid string) (*typ.Provider, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	var record ProviderRecord
	if err := ps.db.Where("uuid = ?", uuid).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("provider with UUID '%s' not found", uuid)
		}
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}

	return record.toProvider(), nil
}

// GetByName returns a provider by name
func (ps *ProviderStore) GetByName(name string) (*typ.Provider, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	var record ProviderRecord
	if err := ps.db.Where("name = ?", name).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("provider with name '%s' not found", name)
		}
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}

	return record.toProvider(), nil
}

// List returns all providers
func (ps *ProviderStore) List() ([]*typ.Provider, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	var records []ProviderRecord
	if err := ps.db.Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to list providers: %w", err)
	}

	providers := make([]*typ.Provider, 0, len(records))
	for _, record := range records {
		providers = append(providers, record.toProvider())
	}

	return providers, nil
}

// ListOAuth returns all OAuth-enabled providers
func (ps *ProviderStore) ListOAuth() ([]*typ.Provider, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	var records []ProviderRecord
	if err := ps.db.Where("auth_type = ?", typ.AuthTypeOAuth).Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to list oauth providers: %w", err)
	}

	providers := make([]*typ.Provider, 0, len(records))
	for _, record := range records {
		providers = append(providers, record.toProvider())
	}

	return providers, nil
}

// Delete removes a provider by UUID
func (ps *ProviderStore) Delete(uuid string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	result := ps.db.Where("uuid = ?", uuid).Delete(&ProviderRecord{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete provider: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("provider with UUID '%s' not found", uuid)
	}

	logrus.Debugf("Deleted provider: %s", uuid)
	return nil
}

// Exists checks if a provider exists by UUID
func (ps *ProviderStore) Exists(uuid string) bool {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	var count int64
	if err := ps.db.Model(&ProviderRecord{}).Where("uuid = ?", uuid).Count(&count).Error; err != nil {
		return false
	}

	return count > 0
}

// Count returns the total number of providers
func (ps *ProviderStore) Count() (int64, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	var count int64
	if err := ps.db.Model(&ProviderRecord{}).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count providers: %w", err)
	}

	return count, nil
}

// UpdateCredential updates only the credential fields of a provider
func (ps *ProviderStore) UpdateCredential(uuid string, token string, oauthDetail *typ.OAuthDetail) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	var record ProviderRecord
	if err := ps.db.Where("uuid = ?", uuid).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("provider with UUID '%s' not found", uuid)
		}
		return fmt.Errorf("failed to get provider: %w", err)
	}

	// Update credential fields based on auth type
	if record.AuthType == string(typ.AuthTypeOAuth) && oauthDetail != nil {
		record.Token = oauthDetail.AccessToken
		record.OAuthProviderType = oauthDetail.ProviderType
		record.OAuthUserID = oauthDetail.UserID
		record.OAuthRefreshToken = oauthDetail.RefreshToken
		record.OAuthExpiresAt = oauthDetail.ExpiresAt
		if oauthDetail.ExtraFields != nil {
			extraJSON, _ := json.Marshal(oauthDetail.ExtraFields)
			record.OAuthExtraFields = string(extraJSON)
		}
	} else {
		record.Token = token
	}

	record.UpdatedAt = time.Now()

	if err := ps.db.Save(&record).Error; err != nil {
		return fmt.Errorf("failed to update provider credential: %w", err)
	}

	logrus.Debugf("Updated credential for provider: %s", uuid)
	return nil
}

// GetAccessToken returns the access token for a provider (convenience method)
func (ps *ProviderStore) GetAccessToken(uuid string) (string, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	var record ProviderRecord
	if err := ps.db.Where("uuid = ?", uuid).First(&record).Error; err != nil {
		return "", fmt.Errorf("failed to get provider: %w", err)
	}

	return record.Token, nil
}

// UpdateOAuthAccessToken updates only the OAuth access token for a provider
func (ps *ProviderStore) UpdateOAuthAccessToken(uuid, accessToken string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	result := ps.db.Model(&ProviderRecord{}).
		Where("uuid = ?", uuid).
		Update("token", accessToken)

	if result.Error != nil {
		return fmt.Errorf("failed to update oauth access token: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("provider with UUID '%s' not found", uuid)
	}

	logrus.Debugf("Updated OAuth access token for provider: %s", uuid)
	return nil
}

// IsOAuthExpired checks if the OAuth token for a provider is expired
func (ps *ProviderStore) IsOAuthExpired(uuid string) (bool, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	var record ProviderRecord
	if err := ps.db.Where("uuid = ?", uuid).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, fmt.Errorf("provider with UUID '%s' not found", uuid)
		}
		return false, fmt.Errorf("failed to get provider: %w", err)
	}

	if record.AuthType != string(typ.AuthTypeOAuth) || record.OAuthExpiresAt == "" {
		return false, nil
	}

	// Parse RFC3339 timestamp and check if expired
	expiryTime, err := time.Parse(time.RFC3339, record.OAuthExpiresAt)
	if err != nil {
		return false, nil
	}

	return time.Now().Add(5 * time.Minute).After(expiryTime), nil
}

// ListEnabled returns all enabled providers
func (ps *ProviderStore) ListEnabled() ([]*typ.Provider, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	var records []ProviderRecord
	if err := ps.db.Where("enabled = ?", true).Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to list enabled providers: %w", err)
	}

	providers := make([]*typ.Provider, 0, len(records))
	for _, record := range records {
		providers = append(providers, record.toProvider())
	}

	return providers, nil
}

// GetDB returns the underlying GORM DB instance (for testing/advanced usage)
func (ps *ProviderStore) GetDB() *gorm.DB {
	return ps.db
}

// Close closes the database connection
func (ps *ProviderStore) Close() error {
	sqlDB, err := ps.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
