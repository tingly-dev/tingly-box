package db

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/tingly-dev/tingly-box/internal/constant"
)

// MemoryStore persists prompt round records in SQLite using GORM.
type MemoryStore struct {
	db     *gorm.DB
	dbPath string
	mu     sync.Mutex
}

// NewMemoryStore creates or loads a prompt store using SQLite database.
func NewMemoryStore(baseDir string) (*MemoryStore, error) {
	log.Printf("Initializing prompt store in directory: %s", baseDir)
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create prompt store directory: %w", err)
	}

	dbPath := constant.GetDBFile(baseDir)
	log.Printf("Opening SQLite database for prompt store: %s", dbPath)
	dsn := dbPath + "?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=1"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open prompt database: %w", err)
	}
	log.Printf("SQLite database opened successfully for prompt store")

	store := &MemoryStore{
		db:     db,
		dbPath: dbPath,
	}

	// Auto-migrate schema for prompt_rounds table
	if err := db.AutoMigrate(&MemoryRoundRecord{}); err != nil {
		return nil, fmt.Errorf("failed to migrate prompt database: %w", err)
	}
	log.Printf("Prompt store initialization completed")

	return store, nil
}

// RecordRound saves a single round to database
func (ps *MemoryStore) RecordRound(record *MemoryRoundRecord) error {
	if record == nil {
		return errors.New("record cannot be nil")
	}

	ps.mu.Lock()
	defer ps.mu.Unlock()

	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	record.UpdatedAt = now
	record.TotalTokens = record.InputTokens + record.OutputTokens

	return ps.db.Create(record).Error
}

// RecordRounds saves multiple rounds in a single transaction
// Supports upsert: updates RoundResult if record exists but RoundResult is empty
func (ps *MemoryStore) RecordRounds(records []*MemoryRoundRecord) error {
	if len(records) == 0 {
		return nil
	}

	ps.mu.Lock()
	defer ps.mu.Unlock()

	now := time.Now().UTC()
	var newRecords []*MemoryRoundRecord
	var updates []*MemoryRoundRecord

	for _, record := range records {
		// Calculate hash if not already set
		if record.UserInputHash == "" && record.UserInput != "" {
			record.UserInputHash = ComputeUserInputHash(record.UserInput)
		}

		// Check if record already exists
		var existing MemoryRoundRecord
		err := ps.db.Where("session_id = ? AND user_input_hash = ?",
			record.SessionID, record.UserInputHash).
			First(&existing).Error

		if err == gorm.ErrRecordNotFound {
			// Record doesn't exist, add to new records
			if record.CreatedAt.IsZero() {
				record.CreatedAt = now
			}
			record.UpdatedAt = now
			record.TotalTokens = record.InputTokens + record.OutputTokens
			newRecords = append(newRecords, record)
		} else if err != nil {
			// Database error
			return fmt.Errorf("failed to check for existing record: %w", err)
		} else {
			// Record exists, check if we need to update RoundResult
			if existing.RoundResult == "" && record.RoundResult != "" {
				// Update RoundResult for existing record
				existing.RoundResult = record.RoundResult
				existing.UpdatedAt = now
				if record.InputTokens > 0 {
					existing.InputTokens = record.InputTokens
				}
				if record.OutputTokens > 0 {
					existing.OutputTokens = record.OutputTokens
				}
				existing.TotalTokens = existing.InputTokens + existing.OutputTokens
				updates = append(updates, &existing)
			}
			// If RoundResult already exists, keep it (don't overwrite)
		}
	}

	// Handle new records
	if len(newRecords) > 0 {
		if len(newRecords) < len(records) {
			log.Printf("Inserting %d new records, skipping %d duplicates",
				len(newRecords), len(records)-len(newRecords))
		}
		if err := ps.db.Create(&newRecords).Error; err != nil {
			return err
		}
	} else if len(records) > 0 {
		log.Printf("All %d records already exist", len(records))
	}

	// Handle updates
	if len(updates) > 0 {
		log.Printf("Updating RoundResult for %d existing records", len(updates))
		for _, update := range updates {
			if err := ps.db.Save(update).Error; err != nil {
				return fmt.Errorf("failed to update record: %w", err)
			}
		}
	}

	return nil
}

// GetRoundsByScenario retrieves rounds for a scenario with pagination
func (ps *MemoryStore) GetRoundsByScenario(scenario string, limit, offset int) ([]MemoryRoundRecord, int64, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	var total int64
	if err := ps.db.Model(&MemoryRoundRecord{}).Where("scenario = ?", scenario).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var records []MemoryRoundRecord
	if err := ps.db.Where("scenario = ?", scenario).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&records).Error; err != nil {
		return nil, 0, err
	}

	return records, total, nil
}

// GetRoundsByProtocol retrieves rounds by protocol type with pagination
func (ps *MemoryStore) GetRoundsByProtocol(protocol ProtocolType, limit, offset int) ([]MemoryRoundRecord, int64, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	var total int64
	if err := ps.db.Model(&MemoryRoundRecord{}).Where("protocol = ?", protocol).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var records []MemoryRoundRecord
	if err := ps.db.Where("protocol = ?", protocol).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&records).Error; err != nil {
		return nil, 0, err
	}

	return records, total, nil
}

// GetRoundsByProjectSession retrieves rounds by project and/or session ID
func (ps *MemoryStore) GetRoundsByProjectSession(projectID, sessionID string, limit int) ([]MemoryRoundRecord, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	query := ps.db.Model(&MemoryRoundRecord{})
	if projectID != "" {
		query = query.Where("project_id = ?", projectID)
	}
	if sessionID != "" {
		query = query.Where("session_id = ?", sessionID)
	}

	var records []MemoryRoundRecord
	if err := query.
		Order("created_at DESC").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, err
	}

	return records, nil
}

// GetRoundsByMetadata retrieves rounds by metadata key-value pairs
// Uses JSON extraction to query the metadata column
func (ps *MemoryStore) GetRoundsByMetadata(key, value string, limit int) ([]MemoryRoundRecord, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// SQLite JSON extraction: json_extract(metadata, '$.key') = value
	var records []MemoryRoundRecord
	if err := ps.db.Where("json_extract(metadata, ?) = ?", "$."+key, value).
		Order("created_at DESC").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, err
	}

	return records, nil
}

// GetRoundsByMultipleMetadata filters by multiple metadata fields
func (ps *MemoryStore) GetRoundsByMultipleMetadata(metadata map[string]string, limit int) ([]MemoryRoundRecord, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	query := ps.db.Model(&MemoryRoundRecord{})
	for key, value := range metadata {
		query = query.Where("json_extract(metadata, ?) = ?", "$."+key, value)
	}

	var records []MemoryRoundRecord
	if err := query.
		Order("created_at DESC").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, err
	}

	return records, nil
}

// GetUserInputs retrieves only user inputs (for prompt user page)
func (ps *MemoryStore) GetUserInputs(scenario string, limit int) ([]MemoryRoundRecord, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	var records []MemoryRoundRecord
	query := ps.db.Model(&MemoryRoundRecord{}).
		Select("id, scenario, provider_uuid, provider_name, model, protocol, request_id, project_id, session_id, round_index, user_input, created_at")

	if scenario != "" {
		query = query.Where("scenario = ?", scenario)
	}

	if err := query.
		Order("created_at DESC").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, err
	}

	return records, nil
}

// SearchRounds searches rounds by user input content
func (ps *MemoryStore) SearchRounds(scenario, query string, limit int) ([]MemoryRoundRecord, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	var records []MemoryRoundRecord
	dbQuery := ps.db.Model(&MemoryRoundRecord{}).Where("user_input LIKE ?", "%"+query+"%")

	if scenario != "" {
		dbQuery = dbQuery.Where("scenario = ?", scenario)
	}

	if err := dbQuery.
		Order("created_at DESC").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, err
	}

	return records, nil
}

// DeleteOlderThan deletes records older than the specified date
func (ps *MemoryStore) DeleteOlderThan(cutoffDate time.Time) (int64, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	result := ps.db.Where("created_at < ?", cutoffDate).Delete(&MemoryRoundRecord{})
	return result.RowsAffected, result.Error
}

// Close closes the database connection
func (ps *MemoryStore) Close() error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	db, err := ps.db.DB()
	if err != nil {
		return err
	}
	return db.Close()
}

// SetMetadata sets the metadata as JSON string on a record
func SetMetadata(record *MemoryRoundRecord, metadata interface{}) error {
	if metadata == nil {
		record.Metadata = ""
		return nil
	}

	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	record.Metadata = string(data)
	return nil
}

// GetMetadata parses the metadata JSON string into a map
func GetMetadata(record *MemoryRoundRecord) (map[string]interface{}, error) {
	if record.Metadata == "" {
		return nil, nil
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(record.Metadata), &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}
	return metadata, nil
}

// ComputeUserInputHash calculates SHA256 hash of user input for deduplication
func ComputeUserInputHash(userInput string) string {
	hash := sha256.Sum256([]byte(userInput))
	return hex.EncodeToString(hash[:])
}
