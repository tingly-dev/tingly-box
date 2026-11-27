package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/tingly-dev/tingly-box/ai/quota"
)

// ExportSQLite publishes a consistent snapshot of the existing source database.
// It includes committed changes in the WAL and never replaces an existing file.
func ExportSQLite(source, output string, excludeUsage bool) error {
	source, err := filepath.EvalSymlinks(source)
	if err != nil {
		return fmt.Errorf("locate source database: %w", err)
	}
	source, err = filepath.Abs(source)
	if err != nil {
		return err
	}
	outputDir, err := filepath.EvalSymlinks(filepath.Dir(output))
	if err != nil {
		return fmt.Errorf("locate export directory: %w", err)
	}
	outputDir, err = filepath.Abs(outputDir)
	if err != nil {
		return err
	}
	output = filepath.Join(outputDir, filepath.Base(output))
	for _, suffix := range []string{"", "-journal", "-wal", "-shm"} {
		if strings.EqualFold(output, source+suffix) {
			return fmt.Errorf("export path belongs to the source database: %s", output)
		}
	}
	if _, err := os.Lstat(output); err == nil {
		return fmt.Errorf("export file already exists: %s", output)
	} else if !os.IsNotExist(err) {
		return err
	}

	dir, err := os.MkdirTemp(outputDir, ".tingly-db-export-")
	if err != nil {
		return fmt.Errorf("create export staging directory: %w", err)
	}
	defer os.RemoveAll(dir)
	staged := filepath.Join(dir, "snapshot.db")
	file, err := os.OpenFile(staged, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}

	conn, err := openExportDatabase(source, fmt.Sprintf("mode=ro&_busy_timeout=%d", defaultBusyTimeoutMs))
	if err != nil {
		return err
	}
	_, snapshotErr := conn.Exec("VACUUM INTO ?", staged)
	conn.Close()
	if snapshotErr != nil {
		return fmt.Errorf("snapshot source database: %w", snapshotErr)
	}
	if excludeUsage {
		if err := scrubExportUsage(staged); err != nil {
			return fmt.Errorf("remove usage from database export: %w", err)
		}
	}
	if err := os.Link(staged, output); err != nil {
		return fmt.Errorf("publish database export: %w", err)
	}
	return nil
}

func openExportDatabase(filename, query string) (*sql.DB, error) {
	path := filepath.ToSlash(filename)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	dsn := url.URL{Scheme: "file", Path: path, RawQuery: query}
	return sql.Open("sqlite3", dsn.String())
}

func scrubExportUsage(path string) error {
	conn, err := openExportDatabase(path, "mode=rw&_journal_mode=DELETE")
	if err != nil {
		return err
	}
	defer conn.Close()

	tables := []string{
		UsageRecord{}.TableName(),
		UsageDailyRecord{}.TableName(),
		ServiceStatsRecord{}.TableName(),
		quota.ProviderUsageRecord{}.TableName(),
		quota.ProviderUsageHistoryRecord{}.TableName(),
	}
	var hasSequence bool
	if err := conn.QueryRow("SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE name = 'sqlite_sequence')").Scan(&hasSequence); err != nil {
		return err
	}
	for _, table := range tables {
		var exists bool
		if err := conn.QueryRow("SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?)", table).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			continue
		}
		if _, err := conn.Exec("DELETE FROM " + table); err != nil {
			return err
		}
		if hasSequence {
			if _, err := conn.Exec("DELETE FROM sqlite_sequence WHERE name = ?", table); err != nil {
				return err
			}
		}
	}
	var hasLastUsed bool
	if err := conn.QueryRow("SELECT EXISTS(SELECT 1 FROM pragma_table_info('api_tokens') WHERE name = 'last_used_at')").Scan(&hasLastUsed); err != nil {
		return err
	}
	if hasLastUsed {
		if _, err := conn.Exec("UPDATE api_tokens SET last_used_at = NULL WHERE last_used_at IS NOT NULL"); err != nil {
			return err
		}
	}
	// Rebuild the file so deleted usage is absent from free pages as well.
	_, err = conn.Exec("VACUUM")
	return err
}
