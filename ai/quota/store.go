package quota

import (
	"context"
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
	gormlogger "gorm.io/gorm/logger"

	"github.com/tingly-dev/tingly-box/internal/constant"
)

var (
	ErrUsageNotFound = errors.New("usage not found")
)

// ProviderUsageRecord is the GORM persistence model for provider usage.
type ProviderUsageRecord struct {
	ProviderUUID string `gorm:"primaryKey;column:provider_uuid"`
	ProviderName string `gorm:"column:provider_name"`
	ProviderType string `gorm:"column:provider_type;index:idx_provider_usage_type"`

	// Primary window
	PrimaryUsed       *float64   `gorm:"column:primary_used"`
	PrimaryLimit      *float64   `gorm:"column:primary_limit"`
	PrimaryType       *string    `gorm:"column:primary_type"`
	PrimaryUnit       *string    `gorm:"column:primary_unit"`
	PrimaryResetsAt   *time.Time `gorm:"column:primary_resets_at"`
	PrimaryWindowMins *int       `gorm:"column:primary_window_mins"`
	PrimaryLabel      *string    `gorm:"column:primary_label"`
	PrimaryDesc       *string    `gorm:"column:primary_desc"`

	// Secondary window
	SecondaryUsed       *float64   `gorm:"column:secondary_used"`
	SecondaryLimit      *float64   `gorm:"column:secondary_limit"`
	SecondaryType       *string    `gorm:"column:secondary_type"`
	SecondaryUnit       *string    `gorm:"column:secondary_unit"`
	SecondaryResetsAt   *time.Time `gorm:"column:secondary_resets_at"`
	SecondaryWindowMins *int       `gorm:"column:secondary_window_mins"`
	SecondaryLabel      *string    `gorm:"column:secondary_label"`
	SecondaryDesc       *string    `gorm:"column:secondary_desc"`

	// Tertiary window
	TertiaryUsed       *float64   `gorm:"column:tertiary_used"`
	TertiaryLimit      *float64   `gorm:"column:tertiary_limit"`
	TertiaryType       *string    `gorm:"column:tertiary_type"`
	TertiaryUnit       *string    `gorm:"column:tertiary_unit"`
	TertiaryResetsAt   *time.Time `gorm:"column:tertiary_resets_at"`
	TertiaryWindowMins *int       `gorm:"column:tertiary_window_mins"`
	TertiaryLabel      *string    `gorm:"column:tertiary_label"`
	TertiaryDesc       *string    `gorm:"column:tertiary_desc"`

	// Cost
	CostUsed     *float64   `gorm:"column:cost_used"`
	CostLimit    *float64   `gorm:"column:cost_limit"`
	CostCurrency *string    `gorm:"column:cost_currency"`
	CostResetsAt *time.Time `gorm:"column:cost_resets_at"`
	CostLabel    *string    `gorm:"column:cost_label"`

	// Account
	AccountID    *string `gorm:"column:account_id"`
	AccountName  *string `gorm:"column:account_name"`
	AccountEmail *string `gorm:"column:account_email"`
	AccountTier  *string `gorm:"column:account_tier"`
	AccountOrgID *string `gorm:"column:account_org_id"`

	// Variable quota data
	WindowsJSON    *string `gorm:"column:windows;type:text"`
	BreakdownsJSON *string `gorm:"column:breakdowns;type:text"`

	// Metadata
	FetchedAt   time.Time  `gorm:"column:fetched_at;index:idx_provider_usage_fetched"`
	ExpiresAt   time.Time  `gorm:"column:expires_at"`
	LastError   *string    `gorm:"column:last_error"`
	LastErrorAt *time.Time `gorm:"column:last_error_at"`
	RawResponse *string    `gorm:"column:raw_response;type:text"` // Raw API response JSON.
}

func (ProviderUsageRecord) TableName() string {
	return "provider_usage"
}

// toProviderUsage converts record to domain model
func (r *ProviderUsageRecord) toProviderUsage() *ProviderUsage {
	usage := &ProviderUsage{
		ProviderUUID: r.ProviderUUID,
		ProviderName: r.ProviderName,
		ProviderType: ProviderType(r.ProviderType),
		FetchedAt:    r.FetchedAt,
		ExpiresAt:    r.ExpiresAt,
		RawResponse:  getString(r.RawResponse),
	}

	if r.LastError != nil {
		usage.LastError = *r.LastError
	}
	if r.LastErrorAt != nil {
		usage.LastErrorAt = r.LastErrorAt
	}

	// Cost
	if r.CostUsed != nil || r.CostLimit != nil {
		usage.Cost = &UsageCost{
			Used:         getFloat64(r.CostUsed),
			Limit:        getFloat64(r.CostLimit),
			CurrencyCode: getString(r.CostCurrency),
			ResetsAt:     r.CostResetsAt,
			Label:        getString(r.CostLabel),
		}
	}

	// Account
	if r.AccountID != nil || r.AccountName != nil {
		usage.Account = &UsageAccount{
			ID:             getString(r.AccountID),
			Name:           getString(r.AccountName),
			Email:          getString(r.AccountEmail),
			Tier:           getString(r.AccountTier),
			OrganizationID: getString(r.AccountOrgID),
		}
	}

	// Variable quota data
	usage.Windows = decodeJSONField[[]*UsageWindow](r.WindowsJSON)
	usage.Breakdowns = decodeJSONField[[]*UsageBreakdown](r.BreakdownsJSON)
	usage.NormalizeWindows()

	return usage
}

// toRecord converts domain model to record
func toRecord(usage *ProviderUsage) *ProviderUsageRecord {
	record := &ProviderUsageRecord{
		ProviderUUID: usage.ProviderUUID,
		ProviderName: usage.ProviderName,
		ProviderType: string(usage.ProviderType),
		FetchedAt:    usage.FetchedAt,
		ExpiresAt:    usage.ExpiresAt,
	}

	if usage.LastError != "" {
		record.LastError = &usage.LastError
	}
	if usage.LastErrorAt != nil {
		record.LastErrorAt = usage.LastErrorAt
	}
	if usage.RawResponse != "" {
		record.RawResponse = &usage.RawResponse
	}

	// Cost
	if usage.Cost != nil {
		record.CostUsed = &usage.Cost.Used
		record.CostLimit = &usage.Cost.Limit
		record.CostCurrency = &usage.Cost.CurrencyCode
		record.CostResetsAt = usage.Cost.ResetsAt
		record.CostLabel = &usage.Cost.Label
	}

	// Account
	if usage.Account != nil {
		record.AccountID = &usage.Account.ID
		record.AccountName = &usage.Account.Name
		if usage.Account.Email != "" {
			record.AccountEmail = &usage.Account.Email
		}
		if usage.Account.Tier != "" {
			record.AccountTier = &usage.Account.Tier
		}
		if usage.Account.OrganizationID != "" {
			record.AccountOrgID = &usage.Account.OrganizationID
		}
	}

	// Variable quota data
	usage.NormalizeWindows()
	record.WindowsJSON = encodeJSONField(usage.Windows)
	record.BreakdownsJSON = encodeJSONField(usage.Breakdowns)

	return record
}

// Helper functions
func decodeJSONField[T any](value *string) T {
	var zero T
	if value == nil || *value == "" {
		return zero
	}
	if err := json.Unmarshal([]byte(*value), &zero); err != nil {
		return *new(T)
	}
	return zero
}

func encodeJSONField[T any](value T) *string {
	data, err := json.Marshal(value)
	if err != nil || string(data) == "null" || string(data) == "[]" {
		return nil
	}
	encoded := string(data)
	return &encoded
}

func getFloat64(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

func getString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func getInt(i *int) int {
	if i == nil {
		return 0
	}
	return *i
}

// Store persists provider quota data.
type Store interface {
	// Save persists a quota snapshot.
	Save(ctx context.Context, usage *ProviderUsage) error

	// Get returns the latest quota data for a provider.
	Get(ctx context.Context, providerUUID string) (*ProviderUsage, error)

	// List returns quota data for all providers.
	List(ctx context.Context) ([]*ProviderUsage, error)

	// Delete removes quota data for a provider.
	Delete(ctx context.Context, providerUUID string) error

	// CleanupExpired removes expired quota data.
	CleanupExpired(ctx context.Context) (int64, error)

	// Close closes the store.
	Close() error
}

// GormStore implements Store with GORM.
type GormStore struct {
	db     *gorm.DB
	dbPath string
	mu     sync.RWMutex
	logger *logrus.Logger
}

// NewGormStore creates a GORM-backed quota store.
func NewGormStore(baseDir string, logger *logrus.Logger) (*GormStore, error) {
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create quota store directory: %w", err)
	}

	dbPath := constant.GetDBFile(baseDir)
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create db directory: %w", err)
	}

	dsn := dbPath + "?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=1"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open quota database: %w", err)
	}

	store := &GormStore{
		db:     db,
		dbPath: dbPath,
		logger: logger,
	}

	if err := store.migrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate quota database: %w", err)
	}

	return store, nil
}

func (s *GormStore) migrate() error {
	return s.db.AutoMigrate(&ProviderUsageRecord{})
}

func (s *GormStore) Save(ctx context.Context, usage *ProviderUsage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record := toRecord(usage)
	return s.db.WithContext(ctx).Save(record).Error
}

func (s *GormStore) Get(ctx context.Context, providerUUID string) (*ProviderUsage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var record ProviderUsageRecord
	err := s.db.WithContext(ctx).
		Where("provider_uuid = ?", providerUUID).
		First(&record).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUsageNotFound
		}
		return nil, err
	}

	return record.toProviderUsage(), nil
}

func (s *GormStore) List(ctx context.Context) ([]*ProviderUsage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var records []ProviderUsageRecord
	err := s.db.WithContext(ctx).Find(&records).Error
	if err != nil {
		return nil, err
	}

	usages := make([]*ProviderUsage, len(records))
	for i, r := range records {
		usages[i] = r.toProviderUsage()
	}

	return usages, nil
}

func (s *GormStore) Delete(ctx context.Context, providerUUID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.db.WithContext(ctx).
		Where("provider_uuid = ?", providerUUID).
		Delete(&ProviderUsageRecord{}).Error
}

func (s *GormStore) CleanupExpired(ctx context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := s.db.WithContext(ctx).
		Where("expires_at < ?", time.Now()).
		Delete(&ProviderUsageRecord{})

	return result.RowsAffected, result.Error
}

func (s *GormStore) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
