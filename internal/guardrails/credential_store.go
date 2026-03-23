package guardrails

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var ErrProtectedCredentialNotFound = errors.New("protected credential not found")

type protectedCredentialRecord struct {
	ID          string    `gorm:"primaryKey;column:id"`
	Name        string    `gorm:"column:name;not null;index"`
	Type        string    `gorm:"column:type;not null"`
	Secret      string    `gorm:"column:secret;not null"`
	AliasToken  string    `gorm:"column:alias_token;not null;uniqueIndex"`
	Description string    `gorm:"column:description"`
	Tags        string    `gorm:"column:tags;type:text"`
	Enabled     bool      `gorm:"column:enabled;default:true"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (protectedCredentialRecord) TableName() string {
	return "protected_credentials"
}

func (r protectedCredentialRecord) toCredential() ProtectedCredential {
	return ProtectedCredential{
		ID:          r.ID,
		Name:        r.Name,
		Type:        r.Type,
		Secret:      r.Secret,
		AliasToken:  r.AliasToken,
		Description: r.Description,
		Tags:        normalizeStringSlice(strings.Split(r.Tags, "\n")),
		Enabled:     r.Enabled,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

func protectedCredentialRecordFromValue(credential ProtectedCredential) protectedCredentialRecord {
	return protectedCredentialRecord{
		ID:          credential.ID,
		Name:        credential.Name,
		Type:        credential.Type,
		Secret:      credential.Secret,
		AliasToken:  credential.AliasToken,
		Description: credential.Description,
		Tags:        strings.Join(normalizeStringSlice(credential.Tags), "\n"),
		Enabled:     credential.Enabled,
		CreatedAt:   credential.CreatedAt,
		UpdatedAt:   credential.UpdatedAt,
	}
}

type ProtectedCredentialStore struct {
	path string
	db   *gorm.DB
	mu   sync.Mutex
}

func NewProtectedCredentialStore(path string) *ProtectedCredentialStore {
	return &ProtectedCredentialStore{path: path}
}

func (s *ProtectedCredentialStore) List() ([]ProtectedCredential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	db, err := s.ensureDB()
	if err != nil {
		return nil, err
	}

	var records []protectedCredentialRecord
	if err := db.Order("created_at desc").Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list protected credentials: %w", err)
	}

	credentials := make([]ProtectedCredential, 0, len(records))
	for _, record := range records {
		credentials = append(credentials, record.toCredential())
	}
	return credentials, nil
}

func (s *ProtectedCredentialStore) Create(credential ProtectedCredential) (ProtectedCredential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	db, err := s.ensureDB()
	if err != nil {
		return ProtectedCredential{}, err
	}

	record := protectedCredentialRecordFromValue(credential)
	if err := db.Create(&record).Error; err != nil {
		return ProtectedCredential{}, fmt.Errorf("create protected credential: %w", err)
	}
	return record.toCredential(), nil
}

func (s *ProtectedCredentialStore) Update(id string, update func(*ProtectedCredential) error) (ProtectedCredential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	db, err := s.ensureDB()
	if err != nil {
		return ProtectedCredential{}, err
	}

	var record protectedCredentialRecord
	if err := db.Where("id = ?", id).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ProtectedCredential{}, ErrProtectedCredentialNotFound
		}
		return ProtectedCredential{}, fmt.Errorf("load protected credential: %w", err)
	}

	credential := record.toCredential()
	if err := update(&credential); err != nil {
		return ProtectedCredential{}, err
	}
	credential.UpdatedAt = time.Now().UTC()
	record = protectedCredentialRecordFromValue(credential)
	if err := db.Save(&record).Error; err != nil {
		return ProtectedCredential{}, fmt.Errorf("update protected credential: %w", err)
	}
	return record.toCredential(), nil
}

func (s *ProtectedCredentialStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	db, err := s.ensureDB()
	if err != nil {
		return err
	}

	result := db.Where("id = ?", id).Delete(&protectedCredentialRecord{})
	if result.Error != nil {
		return fmt.Errorf("delete protected credential: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrProtectedCredentialNotFound
	}
	return nil
}

func (s *ProtectedCredentialStore) Resolve(ids []string) ([]ProtectedCredential, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(ids) == 0 {
		return nil, nil
	}

	db, err := s.ensureDB()
	if err != nil {
		return nil, err
	}

	var records []protectedCredentialRecord
	if err := db.Where("id IN ?", ids).Find(&records).Error; err != nil {
		return nil, fmt.Errorf("resolve protected credentials: %w", err)
	}

	byID := make(map[string]ProtectedCredential, len(records))
	for _, record := range records {
		byID[record.ID] = record.toCredential()
	}

	resolved := make([]ProtectedCredential, 0, len(ids))
	for _, id := range ids {
		credential, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("protected credential %q not found", id)
		}
		resolved = append(resolved, credential)
	}
	return resolved, nil
}

func (s *ProtectedCredentialStore) ensureDB() (*gorm.DB, error) {
	if s.db != nil {
		return s.db, nil
	}
	if s.path == "" {
		return nil, fmt.Errorf("protected credential store path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return nil, fmt.Errorf("create protected credential db dir: %w", err)
	}

	db, err := gorm.Open(sqlite.Open(s.path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open protected credential db: %w", err)
	}
	if err := db.AutoMigrate(&protectedCredentialRecord{}); err != nil {
		return nil, fmt.Errorf("migrate protected credential db: %w", err)
	}

	s.db = db
	return s.db, nil
}

func UpdateProtectedCredential(existing *ProtectedCredential, name, credentialType, secret, description string, tags []string, enabled bool) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("credential name is required")
	}
	if !IsSupportedProtectedCredentialType(credentialType) {
		return fmt.Errorf("unsupported credential type %q", credentialType)
	}
	existing.Name = strings.TrimSpace(name)
	existing.Type = strings.TrimSpace(credentialType)
	if secret != "" {
		existing.Secret = secret
	}
	existing.Description = strings.TrimSpace(description)
	existing.Tags = normalizeStringSlice(tags)
	existing.Enabled = enabled
	return nil
}
