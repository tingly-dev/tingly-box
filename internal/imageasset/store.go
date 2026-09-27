package imageasset

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	dbFileName       = "assets.db"
	referenceDirName = "references"
)

// pieceRow and referenceRow are the tables. Timestamps are Unix milliseconds
// set by this package; GORM's own time tracking is turned off for them.
type pieceRow struct {
	ID        string `gorm:"primaryKey"`
	Kind      string `gorm:"not null;index"`
	Title     string `gorm:"not null;default:''"`
	Text      string `gorm:"not null"`
	Tags      string `gorm:"not null;default:'[]'"` // JSON array
	SourceID  string `gorm:"not null;default:'';index"`
	CreatedAt int64  `gorm:"not null;autoCreateTime:false"`
	UpdatedAt int64  `gorm:"not null;index;autoUpdateTime:false"`
}

func (pieceRow) TableName() string { return "pieces" }

type referenceRow struct {
	ID        string `gorm:"primaryKey"`
	Name      string `gorm:"not null"`
	MIME      string `gorm:"column:mime;not null"`
	File      string `gorm:"not null"` // name within the references directory
	SHA256    string `gorm:"column:sha256;not null;uniqueIndex"`
	Width     int    `gorm:"not null;default:0"`
	Height    int    `gorm:"not null;default:0"`
	Bytes     int64  `gorm:"not null"`
	CreatedAt int64  `gorm:"not null;index;autoCreateTime:false"`
}

// Not "references": that is an SQL keyword.
func (referenceRow) TableName() string { return "reference_images" }

// Store keeps pieces and references under one directory: assets.db for the
// records, references/ for the image files. It opens lazily on first use, so
// constructing one (for example while only generating the API schema)
// touches nothing on disk.
type Store struct {
	dir string
	now func() time.Time

	mu sync.Mutex
	db *gorm.DB
}

// NewStore returns a Store rooted at dir. Nothing is opened until first use.
func NewStore(dir string) *Store {
	return &Store{dir: dir, now: time.Now}
}

func (s *Store) conn() (*gorm.DB, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return s.db, nil
	}
	if err := os.MkdirAll(filepath.Join(s.dir, referenceDirName), 0o700); err != nil {
		return nil, fmt.Errorf("create image assets directory: %w", err)
	}
	dsn := filepath.Join(s.dir, dbFileName) + "?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=1"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open image assets database: %w", err)
	}
	if err := db.AutoMigrate(&pieceRow{}, &referenceRow{}); err != nil {
		if sqlDB, closeErr := db.DB(); closeErr == nil {
			_ = sqlDB.Close()
		}
		return nil, fmt.Errorf("migrate image assets database: %w", err)
	}
	s.db = db
	return db, nil
}

// Close releases the database if it was opened.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	sqlDB, err := s.db.DB()
	s.db = nil
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (s *Store) millis() int64 { return s.now().UnixMilli() }

// --- Pieces ------------------------------------------------------------------

func (r pieceRow) piece() PromptPiece {
	tags := []string{}
	_ = json.Unmarshal([]byte(r.Tags), &tags)
	return PromptPiece{
		ID:        r.ID,
		Kind:      Kind(r.Kind),
		Title:     r.Title,
		Text:      r.Text,
		Tags:      tags,
		SourceID:  r.SourceID,
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
}

// ListPieces returns every piece, most recently edited first.
func (s *Store) ListPieces() ([]PromptPiece, error) {
	db, err := s.conn()
	if err != nil {
		return nil, err
	}
	var rows []pieceRow
	if err := db.Order("updated_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	pieces := make([]PromptPiece, 0, len(rows))
	for _, row := range rows {
		pieces = append(pieces, row.piece())
	}
	return pieces, nil
}

// SavePieces creates pieces, and updates the ones whose ID is set (keeping
// when they were created), in one transaction: all of them are saved or none
// is. Pieces in a batch get consecutive timestamps, so a batch keeps its order.
func (s *Store) SavePieces(inputs []PromptPieceInput) ([]PromptPiece, error) {
	if len(inputs) == 0 {
		return nil, invalid("no pieces to save")
	}
	if len(inputs) > maxBatch {
		return nil, invalid("too many pieces in one request")
	}
	cleaned := make([]PromptPieceInput, len(inputs))
	for i, in := range inputs {
		var err error
		if cleaned[i], err = normalizePiece(in); err != nil {
			return nil, err
		}
	}
	db, err := s.conn()
	if err != nil {
		return nil, err
	}
	now := s.millis()
	saved := make([]PromptPiece, 0, len(cleaned))
	err = db.Transaction(func(tx *gorm.DB) error {
		for i, in := range cleaned {
			at := now + int64(i)
			row := pieceRow{ID: in.ID, CreatedAt: at}
			if in.ID == "" {
				row.ID = uuid.NewString()
			} else if err := tx.First(&row, "id = ?", in.ID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return fmt.Errorf("piece %s: %w", in.ID, ErrNotFound)
				}
				return err
			}
			tags, _ := json.Marshal(in.Tags)
			row.Kind, row.Title, row.Text, row.Tags, row.SourceID, row.UpdatedAt = string(in.Kind), in.Title, in.Text, string(tags), in.SourceID, at
			if err := tx.Save(&row).Error; err != nil {
				return err
			}
			saved = append(saved, row.piece())
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return saved, nil
}

// DeletePiece removes one piece. Pieces split from it are kept.
func (s *Store) DeletePiece(id string) error {
	db, err := s.conn()
	if err != nil {
		return err
	}
	result := db.Delete(&pieceRow{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// --- References --------------------------------------------------------------

func (r referenceRow) reference() ReferenceImage {
	return ReferenceImage{
		ID:        r.ID,
		Name:      r.Name,
		MIME:      r.MIME,
		Width:     r.Width,
		Height:    r.Height,
		Bytes:     r.Bytes,
		CreatedAt: r.CreatedAt,
	}
}

// ListReferences returns every reference, newest first.
func (s *Store) ListReferences() ([]ReferenceImage, error) {
	db, err := s.conn()
	if err != nil {
		return nil, err
	}
	var rows []referenceRow
	if err := db.Order("created_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	references := make([]ReferenceImage, 0, len(rows))
	for _, row := range rows {
		references = append(references, row.reference())
	}
	return references, nil
}

// AddedReferenceImage is one result of AddReferences: the kept reference, and
// whether it was already kept (the same bytes are never stored twice).
type AddedReferenceImage struct {
	ReferenceImage
	Existing bool `json:"existing"`
}

// AddReferences keeps images. Every input is validated before anything is
// written. An image whose bytes are already kept is not stored again: its
// existing record comes back, marked Existing.
func (s *Store) AddReferences(inputs []ReferenceInput) ([]AddedReferenceImage, error) {
	if len(inputs) == 0 {
		return nil, invalid("no images to add")
	}
	if len(inputs) > maxImagesPerCall {
		return nil, invalid("too many images in one request")
	}
	type prepared struct {
		name, mime, sum string
		width, height   int
		data            []byte
	}
	items := make([]prepared, len(inputs))
	for i, in := range inputs {
		name, err := normalizeName(in.Name)
		if err != nil {
			return nil, err
		}
		if len(in.Data) > maxImageBytes {
			return nil, invalid("image is too large")
		}
		mime, width, height, err := sniffImage(in.Data)
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(in.Data)
		items[i] = prepared{name, mime, hex.EncodeToString(sum[:]), width, height, in.Data}
	}

	db, err := s.conn()
	if err != nil {
		return nil, err
	}
	now := s.millis()
	added := make([]AddedReferenceImage, 0, len(items))
	// The same image twice in one request is kept once, like an image
	// already in the store.
	inBatch := make(map[string]ReferenceImage, len(items))
	for i, item := range items {
		if ref, ok := inBatch[item.sum]; ok {
			added = append(added, AddedReferenceImage{ReferenceImage: ref, Existing: true})
			continue
		}
		var existing referenceRow
		err := db.First(&existing, "sha256 = ?", item.sum).Error
		if err == nil {
			added = append(added, AddedReferenceImage{ReferenceImage: existing.reference(), Existing: true})
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		row := referenceRow{
			ID:        uuid.NewString(),
			Name:      item.name,
			MIME:      item.mime,
			SHA256:    item.sum,
			Width:     item.width,
			Height:    item.height,
			Bytes:     int64(len(item.data)),
			CreatedAt: now + int64(i),
		}
		row.File = row.ID + imageExtensions[item.mime]
		path := filepath.Join(s.dir, referenceDirName, row.File)
		// The file first, then the record: a record never points at a file
		// that is not there. A failed insert removes the file again.
		if err := os.WriteFile(path, item.data, 0o600); err != nil {
			return nil, fmt.Errorf("write image: %w", err)
		}
		if err := db.Create(&row).Error; err != nil {
			_ = os.Remove(path)
			return nil, err
		}
		inBatch[item.sum] = row.reference()
		added = append(added, AddedReferenceImage{ReferenceImage: row.reference()})
	}
	return added, nil
}

func (s *Store) findReference(id string) (*gorm.DB, referenceRow, error) {
	var row referenceRow
	db, err := s.conn()
	if err != nil {
		return nil, row, err
	}
	if err := db.First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, row, ErrNotFound
		}
		return nil, row, err
	}
	return db, row, nil
}

// RenameReference changes the name a reference is shown and downloaded under.
func (s *Store) RenameReference(id, name string) (ReferenceImage, error) {
	name, err := normalizeName(name)
	if err != nil {
		return ReferenceImage{}, err
	}
	db, row, err := s.findReference(id)
	if err != nil {
		return ReferenceImage{}, err
	}
	if err := db.Model(&row).Update("name", name).Error; err != nil {
		return ReferenceImage{}, err
	}
	row.Name = name
	return row.reference(), nil
}

// DeleteReference removes a reference and its file.
func (s *Store) DeleteReference(id string) error {
	db, row, err := s.findReference(id)
	if err != nil {
		return err
	}
	if err := db.Delete(&row).Error; err != nil {
		return err
	}
	// The record is what makes the image exist; a file left behind by a
	// failed remove is only disk space.
	_ = os.Remove(filepath.Join(s.dir, referenceDirName, row.File))
	return nil
}

// ReferenceFile returns the path of a reference's image file and its MIME type.
func (s *Store) ReferenceFile(id string) (string, string, error) {
	_, row, err := s.findReference(id)
	if err != nil {
		return "", "", err
	}
	return filepath.Join(s.dir, referenceDirName, row.File), row.MIME, nil
}
