package db

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// APITokenRecord represents a user API token for multi-tenant authentication
type APITokenRecord struct {
	ID           uint       `gorm:"primaryKey;autoIncrement;column:id"`
	TokenID      string     `gorm:"uniqueIndex;column:token_id;not null;size:64"` // Token identifier (jti)
	UserID       string     `gorm:"index:idx_api_token_user_id;not null;column:user_id;size:64"`
	TeamID       string     `gorm:"index:idx_api_token_team_id;not null;default:'00000000-0000-0000-0000-000000000001';column:team_id;size:36"`
	DisplayName  string     `gorm:"column:display_name;size:256"`
	Enabled      bool       `gorm:"column:enabled;default:true"`
	ExpiresAt    *time.Time `gorm:"column:expires_at;index"`
	LastUsedAt   *time.Time `gorm:"column:last_used_at"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
	CreatedBy    string     `gorm:"column:created_by;size:64"`
	RevokedAt    *time.Time `gorm:"column:revoked_at"`
	RevokeReason string     `gorm:"column:revoke_reason;size:512"`
}

// TableName specifies the table name for GORM
func (APITokenRecord) TableName() string {
	return "api_tokens"
}

// defaultLastUsedDebounce is how often UpdateLastUsed will actually persist
// a new last_used_at. Both frontend renderers of this field show minute- or
// second-level absolute timestamps a human reads occasionally (an admin
// token table), not a live/polled value -- nothing in the codebase depends
// on it being fresher than "roughly recent". See UpdateLastUsed.
const defaultLastUsedDebounce = 10 * time.Minute

// APITokenStore manages API tokens for multi-tenant authentication.
//
// ValidateToken is on the request hot path (every "tb-share-"-prefixed
// request, see internal/middleware/auth.go), so cache mirrors the
// api_tokens table by TokenID the same way ProviderStore mirrors providers.
type APITokenStore struct {
	storeConn
	mu        sync.RWMutex
	cache     map[string]*APITokenRecord
	teamStore *TeamStore

	// lastUsedDebounce is the minimum interval between persisted
	// last_used_at writes for the same token; see UpdateLastUsed.
	lastUsedDebounce time.Duration
}

// NewAPITokenStore creates or loads an API token store over its own
// connection to the shared tingly.db.
func NewAPITokenStore(baseDir string) (*APITokenStore, error) {
	db, err := openTinglyDB(baseDir)
	if err != nil {
		return nil, fmt.Errorf("api token store: %w", err)
	}
	conn := ownedConn(db)
	teamStore, err := newTeamStore(conn)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	store, err := newAPITokenStore(conn, teamStore)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return store, nil
}

// newAPITokenStore finishes setting up an APITokenStore (migrate + cache
// load) over an already-open connection -- see newProviderStore.
func newAPITokenStore(conn storeConn, teamStore *TeamStore) (*APITokenStore, error) {
	if err := conn.db.AutoMigrate(&APITokenRecord{}); err != nil {
		return nil, fmt.Errorf("failed to migrate API token database: %w", err)
	}

	// Rename user_uuid column to user_id for consistency
	if err := ensureAPITokenSchema(conn.db); err != nil {
		return nil, fmt.Errorf("failed to align API token schema: %w", err)
	}

	store := &APITokenStore{
		storeConn:        conn,
		cache:            make(map[string]*APITokenRecord),
		teamStore:        teamStore,
		lastUsedDebounce: defaultLastUsedDebounce,
	}
	if err := store.loadCache(); err != nil {
		return nil, fmt.Errorf("failed to load API token cache: %w", err)
	}

	return store, nil
}

// loadCache (re)populates the in-memory mirror from SQLite. Called at
// construction and from CleanupExpiredTokens (under s.mu already held),
// where re-deriving a bulk delete's predicate against the cache isn't
// worth it.
func (s *APITokenStore) loadCache() error {
	var records []APITokenRecord
	if err := s.db.Find(&records).Error; err != nil {
		return err
	}
	s.cache = make(map[string]*APITokenRecord, len(records))
	for i := range records {
		r := records[i]
		s.cache[r.TokenID] = &r
	}
	return nil
}

// ensureAPITokenSchema ensures the API token table schema is up to date
func ensureAPITokenSchema(db *gorm.DB) error {
	// Migrate user_uuid column to user_id for consistency
	if db.Migrator().HasColumn(&APITokenRecord{}, "user_uuid") {
		// Check if user_id already exists (both columns present - migration incomplete)
		if db.Migrator().HasColumn(&APITokenRecord{}, "user_id") {
			// Copy data from user_uuid to user_id where user_id is empty
			if err := db.Exec(`UPDATE api_tokens SET user_id = user_uuid WHERE user_id IS NULL OR user_id = ''`).Error; err != nil {
				logrus.WithError(err).Warn("Failed to copy user_uuid to user_id")
			}
			// Drop the old user_uuid column
			if err := db.Migrator().DropColumn(&APITokenRecord{}, "user_uuid"); err != nil {
				logrus.WithError(err).Warn("Failed to drop user_uuid column")
			}
			// Drop old index
			db.Exec(`DROP INDEX IF EXISTS idx_api_token_user_uuid`)
		} else {
			// Only user_uuid exists - rename it to user_id
			if err := db.Migrator().RenameColumn(&APITokenRecord{}, "user_uuid", "user_id"); err != nil {
				return fmt.Errorf("failed to rename user_uuid to user_id: %w", err)
			}
			// Drop old index and create new one
			db.Exec(`DROP INDEX IF EXISTS idx_api_token_user_uuid`)
		}
	}
	// Existing sharing keys predate teams. They all represented the one legacy
	// team, so bind empty rows to the stable default team without rotating keys.
	if err := db.Model(&APITokenRecord{}).
		Where("team_id IS NULL OR team_id = ''").
		Update("team_id", DefaultTeamID).Error; err != nil {
		return fmt.Errorf("failed to backfill API token team IDs: %w", err)
	}
	return nil
}

// createTokenRecord is a private helper that creates a token record with the given parameters.
// The caller must hold s.mu.Lock() before calling this function.
func (s *APITokenStore) createTokenRecord(userID, tokenID, teamID, displayName, createdBy string, expiresAt *time.Time) (*APITokenRecord, error) {
	now := time.Now()
	record := &APITokenRecord{
		TokenID:     tokenID,
		UserID:      userID,
		TeamID:      teamID,
		DisplayName: displayName,
		Enabled:     true,
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
		CreatedBy:   createdBy,
	}

	if err := s.db.Create(record).Error; err != nil {
		return nil, fmt.Errorf("failed to create API token record: %w", err)
	}

	s.cache[tokenID] = record
	logrus.Debugf("Created API token: %s for user: %s", tokenID, userID)
	return record, nil
}

// CreateTokenWithTokenID creates a new API token record with a specific token ID
func (s *APITokenStore) CreateTokenWithTokenID(userID, tokenID, displayName, createdBy string, expiresAt *time.Time) (*APITokenRecord, error) {
	return s.CreateTokenForTeam(userID, tokenID, DefaultTeamID, displayName, createdBy, expiresAt)
}

// CreateTokenForTeam creates a sharing key bound to exactly one enabled team.
func (s *APITokenStore) CreateTokenForTeam(userID, tokenID, teamID, displayName, createdBy string, expiresAt *time.Time) (*APITokenRecord, error) {
	if userID == "" {
		return nil, errors.New("user ID cannot be empty")
	}
	if tokenID == "" {
		return nil, errors.New("token ID cannot be empty")
	}
	if teamID == "" {
		return nil, errors.New("team ID cannot be empty")
	}
	if s.teamStore == nil {
		return nil, errors.New("team store is not initialized")
	}
	team, err := s.teamStore.Get(teamID)
	if err != nil {
		return nil, err
	}
	if !team.Enabled {
		return nil, fmt.Errorf("team '%s' is disabled", teamID)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.createTokenRecord(userID, tokenID, teamID, displayName, createdBy, expiresAt)
}

// ValidateToken validates a token ID and returns the associated token record
func (s *APITokenStore) ValidateToken(tokenID string) (*APITokenRecord, error) {
	if tokenID == "" {
		return nil, errors.New("token ID cannot be empty")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	record, ok := s.cache[tokenID]
	if !ok || !record.Enabled {
		return nil, fmt.Errorf("token not found or disabled")
	}
	if s.teamStore == nil {
		return nil, errors.New("team store is not initialized")
	}
	team, err := s.teamStore.Get(record.TeamID)
	if err != nil || !team.Enabled {
		return nil, fmt.Errorf("token team not found or disabled")
	}

	clone := *record
	return &clone, nil
}

// MoveTokenToTeam rebinds a sharing key without changing its raw token. The
// cache is updated before the method returns, so the next request observes the
// new authorization scope immediately.
func (s *APITokenStore) MoveTokenToTeam(tokenID, teamID string) error {
	if tokenID == "" || teamID == "" {
		return errors.New("token ID and team ID are required")
	}
	if s.teamStore == nil {
		return errors.New("team store is not initialized")
	}
	team, err := s.teamStore.Get(teamID)
	if err != nil {
		return err
	}
	if !team.Enabled {
		return fmt.Errorf("team '%s' is disabled", teamID)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	result := s.db.Model(&APITokenRecord{}).Where("token_id = ?", tokenID).Update("team_id", teamID)
	if result.Error != nil {
		return fmt.Errorf("failed to move token: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("token with ID '%s' not found", tokenID)
	}
	if record, ok := s.cache[tokenID]; ok {
		record.TeamID = teamID
	}
	return nil
}

// RevokeToken revokes a token by setting enabled to false
func (s *APITokenStore) RevokeToken(tokenID, reason string) error {
	if tokenID == "" {
		return errors.New("token ID cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	result := s.db.Model(&APITokenRecord{}).
		Where("token_id = ?", tokenID).
		Updates(map[string]interface{}{
			"enabled":       false,
			"revoked_at":    now,
			"revoke_reason": reason,
		})

	if result.Error != nil {
		return fmt.Errorf("failed to revoke token: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("token with ID '%s' not found", tokenID)
	}

	if record, ok := s.cache[tokenID]; ok {
		record.Enabled = false
		record.RevokedAt = &now
		record.RevokeReason = reason
	}

	logrus.Debugf("Revoked API token: %s, reason: %s", tokenID, reason)
	return nil
}

// ListTokens returns tokens matching filters
func (s *APITokenStore) ListTokens(userID string, enabled *bool, limit, offset int) ([]APITokenRecord, int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	db := s.db.Model(&APITokenRecord{})

	if userID != "" {
		db = db.Where("user_id = ?", userID)
	}
	if enabled != nil {
		db = db.Where("enabled = ?", *enabled)
	}

	// Get total count
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count tokens: %w", err)
	}

	// Get records with pagination
	var records []APITokenRecord
	if err := db.
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&records).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list tokens: %w", err)
	}

	return records, total, nil
}

// GetToken retrieves a token by token ID
func (s *APITokenStore) GetToken(tokenID string) (*APITokenRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record, ok := s.cache[tokenID]
	if !ok {
		return nil, fmt.Errorf("token with ID '%s' not found", tokenID)
	}

	clone := *record
	return &clone, nil
}

// debounceWindow returns lastUsedDebounce, falling back to the package
// default for any store built without going through newAPITokenStoreOverDB.
func (s *APITokenStore) debounceWindow() time.Duration {
	if s.lastUsedDebounce > 0 {
		return s.lastUsedDebounce
	}
	return defaultLastUsedDebounce
}

// UpdateLastUsed updates the last_used_at timestamp for a token. Called on
// every authenticated request, so writes within debounceWindow() of the
// last persisted value are coalesced away instead of costing one SQLite
// UPDATE per request.
func (s *APITokenStore) UpdateLastUsed(tokenID string) error {
	now := time.Now()
	window := s.debounceWindow()

	s.mu.RLock()
	if record, ok := s.cache[tokenID]; ok && record.LastUsedAt != nil && now.Sub(*record.LastUsedAt) < window {
		s.mu.RUnlock()
		return nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Re-check under the write lock: another goroutine may have already
	// flushed while this one was waiting.
	record, ok := s.cache[tokenID]
	if ok && record.LastUsedAt != nil && now.Sub(*record.LastUsedAt) < window {
		return nil
	}

	result := s.db.Model(&APITokenRecord{}).
		Where("token_id = ?", tokenID).
		Update("last_used_at", now)

	if result.Error != nil {
		return fmt.Errorf("failed to update last used: %w", result.Error)
	}

	if ok {
		record.LastUsedAt = &now
	}

	return nil
}

// SetTokenEnabled enables or disables a token
func (s *APITokenStore) SetTokenEnabled(tokenID string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := s.db.Model(&APITokenRecord{}).
		Where("token_id = ?", tokenID).
		Update("enabled", enabled)

	if result.Error != nil {
		return fmt.Errorf("failed to update token enabled state: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("token with ID '%s' not found", tokenID)
	}

	if record, ok := s.cache[tokenID]; ok {
		record.Enabled = enabled
	}

	logrus.Debugf("Token %s enabled state set to: %v", tokenID, enabled)
	return nil
}

// UpdateTokenString updates the token string for a token (for regeneration)
func (s *APITokenStore) UpdateTokenString(tokenID, newTokenString string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := s.db.Model(&APITokenRecord{}).
		Where("token_id = ?", tokenID).
		Update("token_id", newTokenString)

	if result.Error != nil {
		return fmt.Errorf("failed to update token string: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("token with ID '%s' not found", tokenID)
	}

	if record, ok := s.cache[tokenID]; ok {
		delete(s.cache, tokenID)
		record.TokenID = newTokenString
		s.cache[newTokenString] = record
	}

	logrus.Debugf("Token regenerated, old ID: %s, new ID: %s", tokenID, newTokenString)
	return nil
}

// DeleteToken permanently deletes a token record
func (s *APITokenStore) DeleteToken(tokenID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := s.db.Where("token_id = ?", tokenID).Delete(&APITokenRecord{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete token: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("token with ID '%s' not found", tokenID)
	}

	delete(s.cache, tokenID)

	logrus.Debugf("Deleted API token: %s", tokenID)
	return nil
}

// CleanupExpiredTokens removes expired tokens older than the specified duration
func (s *APITokenStore) CleanupExpiredTokens(olderThan time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)
	result := s.db.Where("expires_at < ? AND enabled = ?", cutoff, false).Delete(&APITokenRecord{})

	if result.Error != nil {
		return 0, fmt.Errorf("failed to cleanup expired tokens: %w", result.Error)
	}

	// Bulk, predicate-based delete — simplest to resync the whole mirror from
	// SQLite rather than re-deriving the same predicate against the cache.
	if result.RowsAffected > 0 {
		if err := s.loadCache(); err != nil {
			return result.RowsAffected, fmt.Errorf("cleaned up %d tokens but failed to refresh cache: %w", result.RowsAffected, err)
		}
	}

	logrus.Debugf("Cleaned up %d expired tokens", result.RowsAffected)
	return result.RowsAffected, nil
}

// GetDB returns the underlying GORM DB instance (for testing)
func (s *APITokenStore) GetDB() *gorm.DB {
	return s.db
}
