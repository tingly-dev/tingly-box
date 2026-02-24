package migrations

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/constant"
)

// MigrateImBotCredentials migrates ImBot credentials from the old remote_coder database
// to the standard tingly.db database.
//
// Parameters:
//   - remoteCoderDBPath: Path to the remote_coder.db file (source)
//   - configDir: Path to the tingly config directory (destination)
//
// Returns:
//   - count: Number of records migrated
//   - err: Any error encountered during migration
func MigrateImBotCredentials(remoteCoderDBPath, configDir string) (count int, err error) {
	// Check if source database exists
	if _, err := os.Stat(remoteCoderDBPath); os.IsNotExist(err) {
		return 0, fmt.Errorf("source database does not exist: %s", remoteCoderDBPath)
	}

	// Open source database
	srcDB, err := sql.Open("sqlite3", remoteCoderDBPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open source database: %w", err)
	}
	defer srcDB.Close()

	// Check if source table exists
	var tableExists int
	err = srcDB.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type='table' AND name='remote_coder_bot_settings_v2'
	`).Scan(&tableExists)
	if err != nil {
		return 0, fmt.Errorf("failed to check source table: %w", err)
	}
	if tableExists == 0 {
		return 0, fmt.Errorf("source table remote_coder_bot_settings_v2 does not exist")
	}

	// Get destination database path
	dstDBPath := constant.GetDBFile(configDir)

	// Check if destination database exists, if not, we'll create it
	if _, err := os.Stat(dstDBPath); os.IsNotExist(err) {
		// Ensure directory exists
		if err := os.MkdirAll(configDir, 0700); err != nil {
			return 0, fmt.Errorf("failed to create destination directory: %w", err)
		}
	}

	// Open destination database
	dstDB, err := sql.Open("sqlite3", dstDBPath+"?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=1")
	if err != nil {
		return 0, fmt.Errorf("failed to open destination database: %w", err)
	}
	defer dstDB.Close()

	// Create destination table if not exists
	_, err = dstDB.Exec(`
		CREATE TABLE IF NOT EXISTS imbot_settings (
			bot_uuid TEXT PRIMARY KEY,
			name TEXT,
			platform TEXT NOT NULL,
			auth_type TEXT,
			auth_config TEXT,
			proxy_url TEXT,
			chat_id_lock TEXT,
			bash_allowlist TEXT,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_imbot_platform ON imbot_settings(platform);
		CREATE INDEX IF NOT EXISTS idx_imbot_enabled ON imbot_settings(enabled);
	`)
	if err != nil {
		return 0, fmt.Errorf("failed to create destination table: %w", err)
	}

	// Query all records from source table
	rows, err := srcDB.Query(`
		SELECT uuid, name, platform, auth_type, auth_config,
		       proxy_url, chat_id_lock, bash_allowlist, enabled,
		       created_at, updated_at
		FROM remote_coder_bot_settings_v2
		ORDER BY created_at DESC
	`)
	if err != nil {
		return 0, fmt.Errorf("failed to query source data: %w", err)
	}
	defer rows.Close()

	// Migrate each record
	migrated := 0
	for rows.Next() {
		var uuid, name, platform, authType, authConfig, proxyURL, chatIDLock, bashAllowlist sql.NullString
		var enabled int
		var createdAt, updatedAt string

		if err := rows.Scan(&uuid, &name, &platform, &authType, &authConfig, &proxyURL, &chatIDLock, &bashAllowlist, &enabled, &createdAt, &updatedAt); err != nil {
			logrus.WithError(err).Warn("Failed to scan row, skipping")
			continue
		}

		// Skip if UUID is empty
		if !uuid.Valid || uuid.String == "" {
			continue
		}

		// Check if record already exists in destination
		var existingCount int
		err = dstDB.QueryRow("SELECT COUNT(*) FROM imbot_settings WHERE bot_uuid = ?", uuid.String).Scan(&existingCount)
		if err != nil {
			logrus.WithError(err).Warn("Failed to check existing record, skipping")
			continue
		}

		// Skip if already migrated
		if existingCount > 0 {
			logrus.WithField("uuid", uuid.String).Info("Record already migrated, skipping")
			migrated++
			continue
		}

		// Validate and normalize timestamp strings
		if createdAt == "" {
			createdAt = time.Now().UTC().Format(time.RFC3339)
		}
		if updatedAt == "" {
			updatedAt = createdAt
		}

		// Insert into destination table
		_, err = dstDB.Exec(`
			INSERT INTO imbot_settings (
				bot_uuid, name, platform, auth_type, auth_config,
				proxy_url, chat_id_lock, bash_allowlist, enabled,
				created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, uuid.String, name.String, platform.String, authType.String,
			authConfig.String, proxyURL.String, chatIDLock.String,
			bashAllowlist.String, enabled, createdAt, updatedAt)

		if err != nil {
			logrus.WithError(err).WithField("uuid", uuid.String).Error("Failed to insert record")
			continue
		}

		logrus.WithField("uuid", uuid.String).WithField("platform", platform.String).Info("Migrated ImBot settings")
		migrated++
	}

	if err := rows.Err(); err != nil {
		return migrated, fmt.Errorf("error iterating rows: %w", err)
	}

	logrus.Infof("ImBot credentials migration completed: %d records migrated", migrated)
	return migrated, nil
}

// VerifyImBotMigration verifies that the migration was successful by comparing record counts
func VerifyImBotMigration(remoteCoderDBPath, configDir string) (sourceCount, destCount int, err error) {
	// Count source records
	srcDB, err := sql.Open("sqlite3", remoteCoderDBPath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to open source database: %w", err)
	}
	defer srcDB.Close()

	err = srcDB.QueryRow("SELECT COUNT(*) FROM remote_coder_bot_settings_v2").Scan(&sourceCount)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to count source records: %w", err)
	}

	// Count destination records
	dstDBPath := constant.GetDBFile(configDir)
	dstDB, err := sql.Open("sqlite3", dstDBPath)
	if err != nil {
		return sourceCount, 0, fmt.Errorf("failed to open destination database: %w", err)
	}
	defer dstDB.Close()

	err = dstDB.QueryRow("SELECT COUNT(*) FROM imbot_settings").Scan(&destCount)
	if err != nil {
		return sourceCount, 0, fmt.Errorf("failed to count destination records: %w", err)
	}

	return sourceCount, destCount, nil
}

// MigrateImBotCredentialsFromConfigDir is a convenience function that migrates
// from the default remote_coder database location for a given config directory.
func MigrateImBotCredentialsFromConfigDir(configDir string) (int, error) {
	remoteCoderDBPath := configDir + "/remote_coder.db"
	return MigrateImBotCredentials(remoteCoderDBPath, configDir)
}

// BackupRemoteCoderDB creates a backup of the remote_coder database before migration
func BackupRemoteCoderDB(dbPath string) (string, error) {
	backupPath := dbPath + ".backup." + time.Now().Format("20060102-150405")

	// Read source file
	data, err := os.ReadFile(dbPath)
	if err != nil {
		return "", fmt.Errorf("failed to read database: %w", err)
	}

	// Write backup
	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write backup: %w", err)
	}

	logrus.WithField("backup", backupPath).Info("Created backup of remote_coder database")
	return backupPath, nil
}

// ValidateMigratedData validates that the migrated data can be unmarshaled correctly
func ValidateMigratedData(configDir string) error {
	dstDBPath := constant.GetDBFile(configDir)
	dstDB, err := sql.Open("sqlite3", dstDBPath)
	if err != nil {
		return fmt.Errorf("failed to open destination database: %w", err)
	}
	defer dstDB.Close()

	// Query all records
	rows, err := dstDB.Query(`
		SELECT bot_uuid, auth_config, bash_allowlist
		FROM imbot_settings
	`)
	if err != nil {
		return fmt.Errorf("failed to query records: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var uuid, authConfig, bashAllowlist sql.NullString

		if err := rows.Scan(&uuid, &authConfig, &bashAllowlist); err != nil {
			return fmt.Errorf("failed to scan row: %w", err)
		}

		// Validate auth_config JSON
		if authConfig.Valid && authConfig.String != "" {
			var authMap map[string]string
			if err := json.Unmarshal([]byte(authConfig.String), &authMap); err != nil {
				return fmt.Errorf("invalid auth_config JSON for %s: %w", uuid.String, err)
			}
		}

		// Validate bash_allowlist JSON
		if bashAllowlist.Valid && bashAllowlist.String != "" {
			var allowlist []string
			if err := json.Unmarshal([]byte(bashAllowlist.String), &allowlist); err != nil {
				return fmt.Errorf("invalid bash_allowlist JSON for %s: %w", uuid.String, err)
			}
		}
	}

	return nil
}
