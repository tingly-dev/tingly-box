package token

import (
	"errors"
	"time"

	"github.com/tingly-dev/tingly-box/internal/enterprise/db"
	"gorm.io/gorm"
)

var (
	// ErrTokenNotFound is returned when a token is not found
	ErrTokenNotFound = errors.New("token not found")
	// ErrTokenExpired is returned when a token has expired
	ErrTokenExpired = errors.New("token expired")
	// ErrTokenInactive is returned when a token is inactive
	ErrTokenInactive = errors.New("token inactive")
)

// Repository defines the token data access interface
type Repository interface {
	// Create creates a new API token
	Create(token *db.APIToken) error
	// GetByID retrieves a token by ID
	GetByID(id int64) (*db.APIToken, error)
	// GetByUUID retrieves a token by UUID
	GetByUUID(uuid string) (*db.APIToken, error)
	// GetByTokenHash retrieves a token by its hash
	GetByTokenHash(tokenHash string) (*db.APIToken, error)
	// ListByUserID retrieves all tokens for a user
	ListByUserID(userID int64) ([]*db.APIToken, error)
	// List retrieves all tokens with pagination
	List(offset, limit int) ([]*db.APIToken, int64, error)
	// Update updates a token
	Update(token *db.APIToken) error
	// Delete deletes a token by ID
	Delete(id int64) error
	// DeleteByUUID deletes a token by UUID
	DeleteByUUID(uuid string) error
	// UpdateLastUsed updates the last used timestamp
	UpdateLastUsed(id int64) error
	// Deactivate deactivates a token by ID
	Deactivate(id int64) error
	// DeactivateByUUID deactivates a token by UUID
	DeactivateByUUID(uuid string) error
	// CleanupExpired deletes all expired tokens
	CleanupExpired() (int64, error)
}

// repositoryImpl implements the Repository interface
type repositoryImpl struct {
	db *gorm.DB
}

// NewRepository creates a new token repository
func NewRepository(gormDB *gorm.DB) Repository {
	return &repositoryImpl{db: gormDB}
}

func (r *repositoryImpl) Create(token *db.APIToken) error {
	return r.db.Create(token).Error
}

func (r *repositoryImpl) GetByID(id int64) (*db.APIToken, error) {
	var token db.APIToken
	err := r.db.Preload("User").Where("id = ?", id).First(&token).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTokenNotFound
		}
		return nil, err
	}
	return &token, nil
}

func (r *repositoryImpl) GetByUUID(uuid string) (*db.APIToken, error) {
	var token db.APIToken
	err := r.db.Preload("User").Where("uuid = ?", uuid).First(&token).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTokenNotFound
		}
		return nil, err
	}
	return &token, nil
}

func (r *repositoryImpl) GetByTokenHash(tokenHash string) (*db.APIToken, error) {
	var token db.APIToken
	err := r.db.Preload("User").Where("token_hash = ?", tokenHash).First(&token).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTokenNotFound
		}
		return nil, err
	}
	return &token, nil
}

func (r *repositoryImpl) ListByUserID(userID int64) ([]*db.APIToken, error) {
	var tokens []*db.APIToken
	err := r.db.Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&tokens).Error
	return tokens, err
}

func (r *repositoryImpl) List(offset, limit int) ([]*db.APIToken, int64, error) {
	var tokens []*db.APIToken
	var total int64

	// Get total count
	if err := r.db.Model(&db.APIToken{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	err := r.db.Preload("User").
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&tokens).Error

	return tokens, total, err
}

func (r *repositoryImpl) Update(token *db.APIToken) error {
	return r.db.Save(token).Error
}

func (r *repositoryImpl) Delete(id int64) error {
	return r.db.Delete(&db.APIToken{}, id).Error
}

func (r *repositoryImpl) DeleteByUUID(uuid string) error {
	return r.db.Where("uuid = ?", uuid).Delete(&db.APIToken{}).Error
}

func (r *repositoryImpl) UpdateLastUsed(id int64) error {
	now := time.Now()
	return r.db.Model(&db.APIToken{}).Where("id = ?", id).Update("last_used_at", now).Error
}

func (r *repositoryImpl) Deactivate(id int64) error {
	return r.db.Model(&db.APIToken{}).Where("id = ?", id).Update("is_active", false).Error
}

func (r *repositoryImpl) DeactivateByUUID(uuid string) error {
	return r.db.Model(&db.APIToken{}).Where("uuid = ?", uuid).Update("is_active", false).Error
}

func (r *repositoryImpl) CleanupExpired() (int64, error) {
	now := time.Now()
	result := r.db.Where("expires_at < ?", now).Delete(&db.APIToken{})
	return result.RowsAffected, result.Error
}
