package db

import (
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
)

// APITokenRecord represents a user API token for multi-tenant authentication
type APITokenRecord struct {
	ID           uint       `gorm:"primaryKey;autoIncrement;column:id"`
	TokenID      string     `gorm:"uniqueIndex;column:token_id;not null;size:64"` // Token identifier (jti)
	UserUUID     string     `gorm:"index:idx_api_token_user_uuid;column:user_uuid;not null;size:64"`
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

// APITokenStore manages API tokens for multi-tenant authentication
type APITokenStore struct {
	db     *gorm.DB
	dbPath string
	mu     sync.Mutex
}

// NewAPITokenStore creates or loads an API token store using SQLite database.
func NewAPITokenStore(baseDir string) (*APITokenStore, error) {
	logrus.Debugf("Initializing API token store in directory: %s", baseDir)
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create API token store directory: %w", err)
	}

	dbPath := constant.GetDBFile(baseDir)
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	logrus.Debugf("Opening SQLite database for API token store: %s", dbPath)
	dsn := dbPath + "?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=1"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open API token database: %w", err)
	}
	logrus.Debugf("SQLite database opened successfully for API token store")

	store := &APITokenStore{
		db:     db,
		dbPath: dbPath,
	}

	// Auto-migrate schema
	if err := db.AutoMigrate(&APITokenRecord{}); err != nil {
		return nil, fmt.Errorf("failed to migrate API token database: %w", err)
	}
	logrus.Debugf("API token store initialization completed")

	return store, nil
}

// createTokenRecord is a private helper that creates a token record with the given parameters.
// The caller must hold s.mu.Lock() before calling this function.
func (s *APITokenStore) createTokenRecord(userUUID, tokenID, displayName, createdBy string, expiresAt *time.Time) (*APITokenRecord, error) {
	now := time.Now()
	record := &APITokenRecord{
		TokenID:     tokenID,
		UserUUID:    userUUID,
		DisplayName: displayName,
		Enabled:     true,
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
		CreatedBy:   createdBy,
	}

	if err := s.db.Create(record).Error; err != nil {
		return nil, fmt.Errorf("failed to create API token record: %w", err)
	}

	logrus.Debugf("Created API token: %s for user: %s", tokenID, userUUID)
	return record, nil
}

// CreateTokenWithTokenID creates a new API token record with a specific token ID
func (s *APITokenStore) CreateTokenWithTokenID(userUUID, tokenID, displayName, createdBy string, expiresAt *time.Time) (*APITokenRecord, error) {
	if userUUID == "" {
		return nil, errors.New("user UUID cannot be empty")
	}
	if tokenID == "" {
		return nil, errors.New("token ID cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.createTokenRecord(userUUID, tokenID, displayName, createdBy, expiresAt)
}

// ValidateToken validates a token ID and returns the associated token record
func (s *APITokenStore) ValidateToken(tokenID string) (*APITokenRecord, error) {
	if tokenID == "" {
		return nil, errors.New("token ID cannot be empty")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var record APITokenRecord
	if err := s.db.Where("token_id = ? AND enabled = ?", tokenID, true).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("token not found or disabled")
		}
		return nil, fmt.Errorf("failed to validate token: %w", err)
	}

	return &record, nil
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

	logrus.Debugf("Revoked API token: %s, reason: %s", tokenID, reason)
	return nil
}

// ListTokens returns tokens matching filters
func (s *APITokenStore) ListTokens(userUUID string, enabled *bool, limit, offset int) ([]APITokenRecord, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	db := s.db.Model(&APITokenRecord{})

	if userUUID != "" {
		db = db.Where("user_uuid = ?", userUUID)
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
	s.mu.Lock()
	defer s.mu.Unlock()

	var record APITokenRecord
	if err := s.db.Where("token_id = ?", tokenID).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("token with ID '%s' not found", tokenID)
		}
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	return &record, nil
}

// UpdateLastUsed updates the last_used_at timestamp for a token
func (s *APITokenStore) UpdateLastUsed(tokenID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	result := s.db.Model(&APITokenRecord{}).
		Where("token_id = ?", tokenID).
		Update("last_used_at", now)

	if result.Error != nil {
		return fmt.Errorf("failed to update last used: %w", result.Error)
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

	logrus.Debugf("Cleaned up %d expired tokens", result.RowsAffected)
	return result.RowsAffected, nil
}

// GetDB returns the underlying GORM DB instance (for testing)
func (s *APITokenStore) GetDB() *gorm.DB {
	return s.db
}

// Close closes the database connection
func (s *APITokenStore) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
