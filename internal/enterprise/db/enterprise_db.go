package db

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/constant"
)

// EnterpriseDBConfig holds enterprise database configuration
type EnterpriseDBConfig struct {
	BaseDir    string
	Log        *logrus.Logger
	SQLiteOpts map[string]string // SQLite connection options
}

// DefaultEnterpriseDBConfig returns default configuration
func DefaultEnterpriseDBConfig(baseDir string) *EnterpriseDBConfig {
	return &EnterpriseDBConfig{
		BaseDir: baseDir,
		Log:     logrus.StandardLogger(),
		SQLiteOpts: map[string]string{
			"_busy_timeout":    "5000",
			"_journal_mode":    "WAL",
			"_foreign_keys":    "1",
			"_cache_size":      "-2000", // 2MB private cache
			"_synchronous":     "NORMAL",
		},
	}
}

// NewEnterpriseDB creates a COMPLETELY ISOLATED enterprise database
// Uses separate database file: ~/.tingly-box/db/tingly_enterprise.db
// This database is ONLY accessed through enterprise module interfaces
func NewEnterpriseDB(config *EnterpriseDBConfig) (*EnterpriseDB, error) {
	if config == nil {
		return nil, fmt.Errorf("database config cannot be nil")
	}

	// Ensure db directory exists with secure permissions
	dbDir := filepath.Join(config.BaseDir, constant.DBDirName)
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create enterprise db directory: %w", err)
	}

	// Use COMPLETELY SEPARATE database file
	dbPath := constant.GetEnterpriseDBFile(config.BaseDir)

	config.Log.WithFields(logrus.Fields{
		"database": dbPath,
		"isolated": true,
	}).Info("Initializing enterprise database (COMPLETELY ISOLATED)")

	// Build DSN with options
	dsn := dbPath
	for k, v := range config.SQLiteOpts {
		dsn += fmt.Sprintf("&%s=%s", k, v)
	}
	// Remove leading &
	dsn = fmt.Sprintf("?%s", dsn[1:])

	// Open database connection
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
		NowFunc: func() time.Time {
			return time.Now().UTC()
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open enterprise database: %w", err)
	}

	// Get underlying SQL DB for connection pool configuration
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}

	// Set connection pool settings for SQLite
	sqlDB.SetMaxOpenConns(1) // SQLite doesn't support multiple writers
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	edb := &EnterpriseDB{db: db}

	// Run auto-migration for all enterprise models
	if err := edb.AutoMigrate(); err != nil {
		return nil, fmt.Errorf("failed to migrate enterprise database: %w", err)
	}

	// Initialize repositories
	userRepo := NewUserRepository(db)
	sessionRepo := NewSessionRepository(db)
	auditRepo := NewAuditLogRepository(db)

	// Create default admin user if no users exist
	if err := edb.ensureDefaultUser(userRepo, auditRepo, config.Log); err != nil {
		config.Log.WithError(err).Warn("Failed to create default admin user")
	}

	config.Log.WithField("database", dbPath).Info("Enterprise database initialized successfully")

	return edb, nil
}

// ensureDefaultUser creates a default admin user if no users exist
func (edb *EnterpriseDB) ensureDefaultUser(userRepo UserRepository, auditRepo AuditLogRepository, log *logrus.Logger) error {
	// Check if any users exist - use a simple count check
	var count int64
	if err := edb.db.Model(&User{}).Count(&count).Error; err != nil {
		return err
	}

	if count > 0 {
		log.Debug("Users already exist, skipping default admin creation")
		return nil
	}

	log.Info("No users found, creating default admin user")

	// Import password service locally for this initialization
	// to avoid circular dependency
	defaultAdmin := &User{
		UUID:         "00000000-0000-0000-0000-000000000001",
		Username:     "admin",
		Email:        "admin@tingly-box.local",
		PasswordHash: "$CHANGE_REQUIRED$", // Must be changed on first login
		Role:         RoleAdmin,
		FullName:     "Default Administrator",
		IsActive:     true,
	}

	if err := edb.db.Create(defaultAdmin).Error; err != nil {
		return fmt.Errorf("failed to create default admin: %w", err)
	}

	// Log the creation
	if auditRepo != nil {
		_ = auditRepo.Create(&AuditLog{
			Action:       "system.init",
			ResourceType: "user",
			ResourceID:   defaultAdmin.UUID,
			Details:      "Created default admin user - password change required",
			Status:       "success",
		})
	}

	log.WithFields(logrus.Fields{
		"username": defaultAdmin.Username,
		"email":    defaultAdmin.Email,
	}).Warn("Default admin user created - PASSWORD CHANGE REQUIRED ON FIRST LOGIN")

	return nil
}

// GetDBPath returns the enterprise database file path
func (edb *EnterpriseDB) GetDBPath() string {
	return constant.GetEnterpriseDBFile(edb.BaseDir)
}

// Close closes the database connection
func (edb *EnterpriseDB) Close() error {
	sqlDB, err := edb.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// HealthCheck performs a health check on the database
func (edb *EnterpriseDB) HealthCheck() error {
	sqlDB, err := edb.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}

// GetStats returns database statistics
func (edb *EnterpriseDB) GetStats() (*EnterpriseDBStats, error) {
	var userCount int64
	var tokenCount int64
	var sessionCount int64
	var auditLogCount int64

	edb.db.Model(&User{}).Count(&userCount)
	edb.db.Model(&APIToken{}).Count(&tokenCount)
	edb.db.Model(&Session{}).Count(&sessionCount)
	edb.db.Model(&AuditLog{}).Count(&auditLogCount)

	return &EnterpriseDBStats{
		UserCount:       int(userCount),
		TokenCount:      int(tokenCount),
		SessionCount:    int(sessionCount),
		AuditLogCount:   int(auditLogCount),
		DatabasePath:    constant.GetEnterpriseDBFile(edb.BaseDir),
		DatabaseSizeMB:  edb.getDatabaseSize(),
	}, nil
}

func (edb *EnterpriseDB) getDatabaseSize() float64 {
	dbPath := constant.GetEnterpriseDBFile(edb.BaseDir)
	info, err := os.Stat(dbPath)
	if err != nil {
		return 0
	}
	return float64(info.Size()) / (1024 * 1024) // Size in MB
}

// EnterpriseDBStats holds database statistics
type EnterpriseDBStats struct {
	UserCount      int    `json:"user_count"`
	TokenCount     int    `json:"token_count"`
	SessionCount   int    `json:"session_count"`
	AuditLogCount  int    `json:"audit_log_count"`
	DatabasePath   string `json:"database_path"`
	DatabaseSizeMB float64 `json:"database_size_mb"`
}
