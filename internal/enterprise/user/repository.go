package user

import (
	"errors"
	"time"

	"github.com/tingly-dev/tingly-box/internal/enterprise/db"
	"gorm.io/gorm"
)

var (
	// ErrUserNotFound is returned when a user is not found
	ErrUserNotFound = errors.New("user not found")
	// ErrUserAlreadyExists is returned when trying to create a user with duplicate credentials
	ErrUserAlreadyExists = errors.New("user already exists")
	// ErrInvalidCredentials is returned when credentials are invalid
	ErrInvalidCredentials = errors.New("invalid credentials")
)

// Repository defines the user data access interface
type Repository interface {
	// Create creates a new user
	Create(user *db.User) error
	// GetByID retrieves a user by ID
	GetByID(id int64) (*db.User, error)
	// GetByUUID retrieves a user by UUID
	GetByUUID(uuid string) (*db.User, error)
	// GetByUsername retrieves a user by username
	GetByUsername(username string) (*db.User, error)
	// GetByEmail retrieves a user by email
	GetByEmail(email string) (*db.User, error)
	// List retrieves all users with pagination
	List(offset, limit int) ([]*db.User, int64, error)
	// Update updates a user
	Update(user *db.User) error
	// Delete deletes a user by ID
	Delete(id int64) error
	// UpdateLastLogin updates the last login timestamp
	UpdateLastLogin(id int64) error
	// SetActive sets the active status of a user
	SetActive(id int64, active bool) error
	// ExistsByUsername checks if a username exists
	ExistsByUsername(username string) (bool, error)
	// ExistsByEmail checks if an email exists
	ExistsByEmail(email string) (bool, error)
}

// repositoryImpl implements the Repository interface
type repositoryImpl struct {
	db *gorm.DB
}

// NewRepository creates a new user repository
func NewRepository(gormDB *gorm.DB) Repository {
	return &repositoryImpl{db: gormDB}
}

func (r *repositoryImpl) Create(user *db.User) error {
	// Check if username already exists
	exists, err := r.ExistsByUsername(user.Username)
	if err != nil {
		return err
	}
	if exists {
		return ErrUserAlreadyExists
	}

	// Check if email already exists
	exists, err = r.ExistsByEmail(user.Email)
	if err != nil {
		return err
	}
	if exists {
		return ErrUserAlreadyExists
	}

	return r.db.Create(user).Error
}

func (r *repositoryImpl) GetByID(id int64) (*db.User, error) {
	var user db.User
	err := r.db.Where("id = ?", id).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

func (r *repositoryImpl) GetByUUID(uuid string) (*db.User, error) {
	var user db.User
	err := r.db.Where("uuid = ?", uuid).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

func (r *repositoryImpl) GetByUsername(username string) (*db.User, error) {
	var user db.User
	err := r.db.Where("username = ?", username).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

func (r *repositoryImpl) GetByEmail(email string) (*db.User, error) {
	var user db.User
	err := r.db.Where("email = ?", email).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

func (r *repositoryImpl) List(offset, limit int) ([]*db.User, int64, error) {
	var users []*db.User
	var total int64

	// Get total count
	if err := r.db.Model(&db.User{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	err := r.db.Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&users).Error

	return users, total, err
}

func (r *repositoryImpl) Update(user *db.User) error {
	return r.db.Save(user).Error
}

func (r *repositoryImpl) Delete(id int64) error {
	return r.db.Delete(&db.User{}, id).Error
}

func (r *repositoryImpl) UpdateLastLogin(id int64) error {
	now := time.Now()
	return r.db.Model(&db.User{}).Where("id = ?", id).Update("last_login_at", now).Error
}

func (r *repositoryImpl) SetActive(id int64, active bool) error {
	return r.db.Model(&db.User{}).Where("id = ?", id).Update("is_active", active).Error
}

func (r *repositoryImpl) ExistsByUsername(username string) (bool, error) {
	var count int64
	err := r.db.Model(&db.User{}).Where("username = ?", username).Count(&count).Error
	return count > 0, err
}

func (r *repositoryImpl) ExistsByEmail(email string) (bool, error) {
	var count int64
	err := r.db.Model(&db.User{}).Where("email = ?", email).Count(&count).Error
	return count > 0, err
}
