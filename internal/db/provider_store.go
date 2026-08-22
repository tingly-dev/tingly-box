package db

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/ai"
	"gorm.io/gorm"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ProviderRecord is the GORM model for persisting a complete provider
// This includes both configuration and credentials as one logical entity
type ProviderRecord struct {
	UUID     string `gorm:"primaryKey;column:uuid"`
	Name     string `gorm:"column:name;not null;index"`
	APIBase  string `gorm:"column:api_base;not null"`
	APIStyle string `gorm:"column:api_style;not null"`    // "openai" or "anthropic"
	AuthType string `gorm:"column:auth_type;not null"`    // "api_key", "oauth", or "vmodel"
	Source   string `gorm:"column:source;default:'user'"` // "user" (default) or "builtin"

	// Configuration fields
	NoKeyRequired bool     `gorm:"column:no_key_required;default:false"`
	Enabled       bool     `gorm:"column:enabled;default:true"`
	ProxyURL      string   `gorm:"column:proxy_url"`
	Timeout       int64    `gorm:"column:timeout"`
	Tags          []string `gorm:"column:tags;type:text;serializer:json"`
	LastUpdated   string   `gorm:"column:last_updated"`

	// Dual-mode optional fields. Independent of APIBase/APIStyle.
	APIBaseOpenAI    string `gorm:"column:api_base_openai"`
	APIBaseAnthropic string `gorm:"column:api_base_anthropic"`

	// OpenAIEndpointMode declares which OpenAI endpoints this provider exposes
	// ("", "chat", "responses", "both"). See ai.OpenAIEndpointMode.
	OpenAIEndpointMode string `gorm:"column:openai_endpoint_mode"`

	// Credential fields - stored with provider as a unit
	// For api_key auth: stores the API key
	// For oauth auth: stores OAuth access token
	Token             string `gorm:"column:token"`               // API key or access token
	OAuthProviderType string `gorm:"column:oauth_provider_type"` // For oauth: provider type
	OAuthUserID       string `gorm:"column:oauth_user_id"`       // For oauth: user ID
	OAuthRefreshToken string `gorm:"column:oauth_refresh_token"` // For oauth: refresh token
	OAuthExpiresAt    string `gorm:"column:oauth_expires_at"`    // For oauth: token expiration (RFC3339)

	OAuthExtraFields map[string]any `gorm:"column:oauth_extra_fields;type:text;serializer:json"`

	// VModelDetail is only populated when AuthType == "vmodel".
	VModelDetail *typ.VModelDetail `gorm:"column:vmodel_detail;type:text;serializer:json"`

	// Credential holds multi-field credentials for non-bearer auth types
	// (aws_sigv4, azure_key, gcp_sa). Nil for api_key/oauth/vmodel. Added
	// additively; AutoMigrate creates the column on existing databases with
	// no backfill required.
	Credential *typ.CredentialBundle `gorm:"column:credential;type:text;serializer:json"`

	// Flags / ModelFlags hold the provider-level options and the per-model
	// overrides. Auth-type independent. Added additively; AutoMigrate creates
	// the columns with no backfill required.
	Flags      typ.ProviderFlags            `gorm:"column:flags;type:text;serializer:json"`
	ModelFlags map[string]typ.ProviderFlags `gorm:"column:model_flags;type:text;serializer:json"`

	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// TableName specifies the table name for GORM
func (ProviderRecord) TableName() string {
	return "providers"
}

// The four JSON-backed columns above used to be `string` fields that every
// caller marshalled and unmarshalled by hand. `serializer:json` does that
// now, which surfaces codec errors that were previously discarded -- but it
// also removes a property the string encoding provided by accident: because
// unmarshalling allocated fresh values on every read, each caller got a
// private copy of the tags/map/struct.
//
// ProviderStore caches *ProviderRecord and hands out typ.Provider built from
// it, so without that copy the cache would be sharing mutable state with
// every caller. That is not hypothetical: the OAuth callback handler does
// `provider.OAuthDetail.ExtraFields["id_token"] = ...` on a provider it read
// from the store (internal/server/module/oauth/handler.go), which would
// otherwise write straight into the cached record -- persisting even if the
// subsequent UpdateProvider failed.
//
// The clone helpers below keep that isolation explicit at both boundaries:
// toProvider clones on the way out, toRecord/updateRecordFromProvider clone
// on the way in. Empty is normalized to nil so the column stays NULL rather
// than "[]"/"{}" -- "absent" must not read back as "present but empty".

func cloneStrings(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return slices.Clone(s)
}

func cloneExtraFields(m map[string]any) map[string]any {
	if len(m) == 0 {
		return nil
	}
	return maps.Clone(m)
}

func cloneVModelDetail(d *typ.VModelDetail) *typ.VModelDetail {
	if d == nil {
		return nil
	}
	out := *d
	out.Models = cloneStrings(d.Models)
	return &out
}

// cloneProviderFlags / cloneModelFlags isolate the cached record from
// callers the same way as the other JSON columns.
func cloneProviderFlags(f typ.ProviderFlags) typ.ProviderFlags {
	if len(f.ExtraHeaders) > 0 {
		f.ExtraHeaders = maps.Clone(f.ExtraHeaders)
	} else {
		f.ExtraHeaders = nil
	}
	return f
}

func cloneModelFlags(m map[string]typ.ProviderFlags) map[string]typ.ProviderFlags {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]typ.ProviderFlags, len(m))
	for model, flags := range m {
		out[model] = cloneProviderFlags(flags)
	}
	return out
}

func cloneCredential(c *typ.CredentialBundle) *typ.CredentialBundle {
	if c == nil {
		return nil
	}
	out := *c
	if len(c.Fields) > 0 {
		out.Fields = maps.Clone(c.Fields)
	} else {
		out.Fields = nil
	}
	return &out
}

// toProvider converts a ProviderRecord to typ.Provider
func (r *ProviderRecord) toProvider() *typ.Provider {
	provider := &typ.Provider{
		UUID:               r.UUID,
		Name:               r.Name,
		APIBase:            r.APIBase,
		APIStyle:           protocol.APIStyle(r.APIStyle),
		APIBaseOpenAI:      r.APIBaseOpenAI,
		APIBaseAnthropic:   r.APIBaseAnthropic,
		AuthType:           typ.AuthType(r.AuthType),
		Source:             typ.ProviderSource(r.Source),
		NoKeyRequired:      r.NoKeyRequired,
		Enabled:            r.Enabled,
		ProxyURL:           r.ProxyURL,
		Timeout:            r.Timeout,
		LastUpdated:        r.LastUpdated,
		OpenAIEndpointMode: ai.OpenAIEndpointMode(r.OpenAIEndpointMode),
	}

	provider.Tags = cloneStrings(r.Tags)
	provider.Flags = cloneProviderFlags(r.Flags)
	provider.ModelFlags = cloneModelFlags(r.ModelFlags)

	// Set credentials based on auth type
	switch provider.AuthType {
	case typ.AuthTypeOAuth:
		provider.OAuthDetail = &typ.OAuthDetail{
			AccessToken:  r.Token,
			Issuer:       ai.Issuer(r.OAuthProviderType),
			UserID:       r.OAuthUserID,
			RefreshToken: r.OAuthRefreshToken,
			ExpiresAt:    r.OAuthExpiresAt,
			ExtraFields:  cloneExtraFields(r.OAuthExtraFields),
		}
	case typ.AuthTypeVirtual:
		provider.VModelDetail = cloneVModelDetail(r.VModelDetail)
	case typ.AuthTypeAWSSigV4, typ.AuthTypeAzureKey, typ.AuthTypeGCPVertex:
		provider.Credential = cloneCredential(r.Credential)
	case typ.AuthTypeAPIKey, "":
		provider.Token = r.Token
		provider.AuthType = typ.AuthTypeAPIKey
	}

	return provider
}

// toRecord converts a typ.Provider to ProviderRecord
func toRecord(p *typ.Provider) *ProviderRecord {
	now := time.Now()

	record := &ProviderRecord{
		UUID:               p.UUID,
		Name:               p.Name,
		APIBase:            p.APIBase,
		APIStyle:           string(p.APIStyle),
		APIBaseOpenAI:      p.APIBaseOpenAI,
		APIBaseAnthropic:   p.APIBaseAnthropic,
		AuthType:           string(p.AuthType),
		Source:             string(p.Source),
		NoKeyRequired:      p.NoKeyRequired,
		Enabled:            p.Enabled,
		ProxyURL:           p.ProxyURL,
		Timeout:            p.Timeout,
		LastUpdated:        p.LastUpdated,
		OpenAIEndpointMode: string(p.OpenAIEndpointMode),
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	// Initialize OAuth fields if OAuthDetail exists
	if p.OAuthDetail != nil {
		record.OAuthProviderType = string(p.OAuthDetail.GetIssuer())
		record.OAuthUserID = p.OAuthDetail.UserID
		record.OAuthExpiresAt = p.OAuthDetail.ExpiresAt
	}

	record.Tags = cloneStrings(p.Tags)
	record.Flags = cloneProviderFlags(p.Flags)
	record.ModelFlags = cloneModelFlags(p.ModelFlags)

	// Set credentials based on auth type
	switch p.AuthType {
	case typ.AuthTypeOAuth:
		if p.OAuthDetail != nil {
			record.Token = p.OAuthDetail.AccessToken
			record.OAuthRefreshToken = p.OAuthDetail.RefreshToken
			record.OAuthExtraFields = cloneExtraFields(p.OAuthDetail.ExtraFields)
		}
	case typ.AuthTypeVirtual:
		record.VModelDetail = cloneVModelDetail(p.VModelDetail)
	case typ.AuthTypeAWSSigV4, typ.AuthTypeAzureKey, typ.AuthTypeGCPVertex:
		record.Credential = cloneCredential(p.Credential)
	case typ.AuthTypeAPIKey, "":
		record.Token = p.Token
	}

	return record
}

// updateRecordFromProvider updates an existing ProviderRecord from typ.Provider
func updateRecordFromProvider(record *ProviderRecord, p *typ.Provider) {
	record.Name = p.Name
	record.APIBase = p.APIBase
	record.APIStyle = string(p.APIStyle)
	record.APIBaseOpenAI = p.APIBaseOpenAI
	record.APIBaseAnthropic = p.APIBaseAnthropic
	record.AuthType = string(p.AuthType)
	record.Source = string(p.Source)
	record.NoKeyRequired = p.NoKeyRequired
	record.Enabled = p.Enabled
	record.ProxyURL = p.ProxyURL
	record.Timeout = p.Timeout
	record.LastUpdated = p.LastUpdated
	record.OpenAIEndpointMode = string(p.OpenAIEndpointMode)
	record.UpdatedAt = time.Now()

	record.Tags = cloneStrings(p.Tags)
	record.Flags = cloneProviderFlags(p.Flags)
	record.ModelFlags = cloneModelFlags(p.ModelFlags)

	// Set credentials based on auth type
	switch p.AuthType {
	case typ.AuthTypeOAuth:
		if p.OAuthDetail != nil {
			record.Token = p.OAuthDetail.AccessToken
			record.OAuthProviderType = string(p.OAuthDetail.GetIssuer())
			record.OAuthUserID = p.OAuthDetail.UserID
			record.OAuthRefreshToken = p.OAuthDetail.RefreshToken
			record.OAuthExpiresAt = p.OAuthDetail.ExpiresAt
			record.OAuthExtraFields = cloneExtraFields(p.OAuthDetail.ExtraFields)
		}
	case typ.AuthTypeVirtual:
		record.VModelDetail = cloneVModelDetail(p.VModelDetail)
		record.Token = ""
		record.OAuthProviderType = ""
		record.OAuthUserID = ""
		record.OAuthRefreshToken = ""
		record.OAuthExpiresAt = ""
		record.OAuthExtraFields = nil
		record.Credential = nil
	case typ.AuthTypeAWSSigV4, typ.AuthTypeAzureKey, typ.AuthTypeGCPVertex:
		record.Credential = cloneCredential(p.Credential)
		record.Token = ""
		record.OAuthProviderType = ""
		record.OAuthUserID = ""
		record.OAuthRefreshToken = ""
		record.OAuthExpiresAt = ""
		record.OAuthExtraFields = nil
		record.VModelDetail = nil
	case typ.AuthTypeAPIKey, "":
		record.Token = p.Token
		record.OAuthProviderType = ""
		record.OAuthUserID = ""
		record.OAuthRefreshToken = ""
		record.OAuthExpiresAt = ""
		record.OAuthExtraFields = nil
		record.VModelDetail = nil
		record.Credential = nil
	}
}

// ProviderStore manages providers as complete units (configuration + credentials).
//
// Reads are served from an in-memory cache (write-through: SQLite is still
// the source of truth) instead of hitting SQLite per call — GetByUUID is on
// the request hot path (see internal/routing/selector.go) and used to
// account for ~47% of Select()'s CPU time.
type ProviderStore struct {
	storeConn
	mu sync.RWMutex

	cache map[string]*ProviderRecord
	// order preserves insertion order for List/ListOAuth/ListEnabled, since
	// map iteration is randomized in Go.
	order []string
}

// NewProviderStore creates or loads a provider store over its own connection
// to the shared tingly.db.
func NewProviderStore(baseDir string) (*ProviderStore, error) {
	db, err := openTinglyDB(baseDir)
	if err != nil {
		return nil, fmt.Errorf("provider store: %w", err)
	}
	return newProviderStore(ownedConn(db))
}

// newProviderStore finishes setting up a ProviderStore (migrate + cache load)
// over an already-open connection, shared by NewProviderStore and
// StoreManager so the cache-init wiring can't drift between them.
func newProviderStore(conn storeConn) (*ProviderStore, error) {
	if err := conn.db.AutoMigrate(&ProviderRecord{}); err != nil {
		return nil, fmt.Errorf("failed to migrate provider database: %w", err)
	}

	store := &ProviderStore{
		storeConn: conn,
		cache:     make(map[string]*ProviderRecord),
	}
	if err := store.loadCache(); err != nil {
		return nil, fmt.Errorf("failed to load provider cache: %w", err)
	}

	return store, nil
}

// loadCache populates the in-memory mirror from SQLite. Called once at
// construction, before the store is shared with any other goroutine.
func (ps *ProviderStore) loadCache() error {
	var records []ProviderRecord
	if err := ps.db.Find(&records).Error; err != nil {
		return err
	}
	for i := range records {
		r := records[i]
		ps.cache[r.UUID] = &r
		ps.order = append(ps.order, r.UUID)
	}
	return nil
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

	if _, ok := ps.cache[provider.UUID]; ok {
		if _, err := ps.writeThroughLocked(provider.UUID, func(r *ProviderRecord) {
			updateRecordFromProvider(r, provider)
		}); err != nil {
			return fmt.Errorf("failed to update provider record: %w", err)
		}
		logrus.Debugf("Updated provider: %s (%s)", provider.Name, provider.UUID)
	} else {
		// Create new record
		record := toRecord(provider)
		if err := ps.db.Create(record).Error; err != nil {
			return fmt.Errorf("failed to create provider record: %w", err)
		}
		ps.cache[provider.UUID] = record
		ps.order = append(ps.order, provider.UUID)
		logrus.Debugf("Created new provider: %s (%s)", provider.Name, provider.UUID)
	}

	return nil
}

// writeThroughLocked mutates a copy of the cached record for uuid, persists
// it, and only then swaps it into ps.cache -- so a failed write can't leave
// the cache holding unpersisted data. Callers must hold ps.mu.Lock().
func (ps *ProviderStore) writeThroughLocked(uuid string, mutate func(*ProviderRecord)) (*ProviderRecord, error) {
	existing, ok := ps.cache[uuid]
	if !ok {
		return nil, fmt.Errorf("provider with UUID '%s' not found", uuid)
	}

	updated := *existing
	mutate(&updated)

	if err := ps.db.Save(&updated).Error; err != nil {
		return nil, err
	}

	ps.cache[uuid] = &updated
	return &updated, nil
}

// GetByUUID returns a provider by UUID
func (ps *ProviderStore) GetByUUID(uuid string) (*typ.Provider, error) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	record, ok := ps.cache[uuid]
	if !ok {
		return nil, fmt.Errorf("provider with UUID '%s' not found", uuid)
	}

	return record.toProvider(), nil
}

// GetByName returns a provider by name
func (ps *ProviderStore) GetByName(name string) (*typ.Provider, error) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	for _, uuid := range ps.order {
		if record := ps.cache[uuid]; record.Name == name {
			return record.toProvider(), nil
		}
	}

	return nil, fmt.Errorf("provider with name '%s' not found", name)
}

// List returns all providers
func (ps *ProviderStore) List() ([]*typ.Provider, error) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	providers := make([]*typ.Provider, 0, len(ps.order))
	for _, uuid := range ps.order {
		providers = append(providers, ps.cache[uuid].toProvider())
	}

	return providers, nil
}

// ListOAuth returns all OAuth-enabled providers
func (ps *ProviderStore) ListOAuth() ([]*typ.Provider, error) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	providers := make([]*typ.Provider, 0, len(ps.order))
	for _, uuid := range ps.order {
		record := ps.cache[uuid]
		if record.AuthType == string(typ.AuthTypeOAuth) {
			providers = append(providers, record.toProvider())
		}
	}

	return providers, nil
}

// Delete removes a provider by UUID
func (ps *ProviderStore) Delete(uuid string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if _, ok := ps.cache[uuid]; !ok {
		return fmt.Errorf("provider with UUID '%s' not found", uuid)
	}

	result := ps.db.Where("uuid = ?", uuid).Delete(&ProviderRecord{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete provider: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("provider with UUID '%s' not found", uuid)
	}

	delete(ps.cache, uuid)
	for i, id := range ps.order {
		if id == uuid {
			ps.order = append(ps.order[:i], ps.order[i+1:]...)
			break
		}
	}

	logrus.Debugf("Deleted provider: %s", uuid)
	return nil
}

// Exists checks if a provider exists by UUID
func (ps *ProviderStore) Exists(uuid string) bool {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	_, ok := ps.cache[uuid]
	return ok
}

// Count returns the total number of providers
func (ps *ProviderStore) Count() (int64, error) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	return int64(len(ps.cache)), nil
}

// UpdateCredential updates only the credential fields of a provider
func (ps *ProviderStore) UpdateCredential(uuid string, token string, oauthDetail *typ.OAuthDetail) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	_, err := ps.writeThroughLocked(uuid, func(r *ProviderRecord) {
		if r.AuthType == string(typ.AuthTypeOAuth) && oauthDetail != nil {
			r.Token = oauthDetail.AccessToken
			r.OAuthProviderType = string(oauthDetail.GetIssuer())
			r.OAuthUserID = oauthDetail.UserID
			r.OAuthRefreshToken = oauthDetail.RefreshToken
			r.OAuthExpiresAt = oauthDetail.ExpiresAt
			if oauthDetail.ExtraFields != nil {
				r.OAuthExtraFields = cloneExtraFields(oauthDetail.ExtraFields)
			}
		} else {
			r.Token = token
		}
		r.UpdatedAt = time.Now()
	})
	if err != nil {
		return fmt.Errorf("failed to update provider credential: %w", err)
	}

	logrus.Debugf("Updated credential for provider: %s", uuid)
	return nil
}

// UpdateCredentialBundle updates only the multi-field credential of a provider
// (auth types aws_sigv4, azure_key, gcp_sa). Added alongside UpdateCredential
// to avoid changing that method's signature and its existing callers.
func (ps *ProviderStore) UpdateCredentialBundle(uuid string, bundle *typ.CredentialBundle) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	_, err := ps.writeThroughLocked(uuid, func(r *ProviderRecord) {
		r.Credential = cloneCredential(bundle)
		r.UpdatedAt = time.Now()
	})
	if err != nil {
		return fmt.Errorf("failed to update provider credential bundle: %w", err)
	}

	logrus.Debugf("Updated credential bundle for provider: %s", uuid)
	return nil
}

// GetAccessToken returns the access token for a provider (convenience method)
func (ps *ProviderStore) GetAccessToken(uuid string) (string, error) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	record, ok := ps.cache[uuid]
	if !ok {
		return "", fmt.Errorf("failed to get provider: provider with UUID '%s' not found", uuid)
	}

	return record.Token, nil
}

// UpdateOAuthAccessToken updates only the OAuth access token for a provider
func (ps *ProviderStore) UpdateOAuthAccessToken(uuid, accessToken string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	record, ok := ps.cache[uuid]
	if !ok {
		return fmt.Errorf("provider with UUID '%s' not found", uuid)
	}

	result := ps.db.Model(&ProviderRecord{}).
		Where("uuid = ?", uuid).
		Update("token", accessToken)

	if result.Error != nil {
		return fmt.Errorf("failed to update oauth access token: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("provider with UUID '%s' not found", uuid)
	}

	record.Token = accessToken
	record.UpdatedAt = time.Now()

	logrus.Debugf("Updated OAuth access token for provider: %s", uuid)
	return nil
}

// IsOAuthExpired checks if the OAuth token for a provider is expired
func (ps *ProviderStore) IsOAuthExpired(uuid string) (bool, error) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	record, ok := ps.cache[uuid]
	if !ok {
		return false, fmt.Errorf("provider with UUID '%s' not found", uuid)
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
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	providers := make([]*typ.Provider, 0, len(ps.order))
	for _, uuid := range ps.order {
		record := ps.cache[uuid]
		if record.Enabled {
			providers = append(providers, record.toProvider())
		}
	}

	return providers, nil
}

// GetDB returns the underlying GORM DB instance (for testing/advanced usage)
func (ps *ProviderStore) GetDB() *gorm.DB {
	return ps.db
}
