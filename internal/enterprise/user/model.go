package user

import (
	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/internal/enterprise/db"
)

// Model provides user domain model operations
type Model struct {
	repo Repository
}

// NewModel creates a new user model
func NewModel(repo Repository) *Model {
	return &Model{repo: repo}
}

// CreateUserData contains data for creating a new user
type CreateUserData struct {
	Username     string
	Email        string
	PasswordHash string
	Role         db.Role
	FullName     string
}

// UpdateUserData contains data for updating a user
type UpdateUserData struct {
	FullName *string
	Role     *db.Role
	IsActive *bool
}

// Create creates a new user
func (m *Model) Create(data CreateUserData) (*db.User, error) {
	user := &db.User{
		UUID:         uuid.New().String(),
		Username:     data.Username,
		Email:        data.Email,
		PasswordHash: data.PasswordHash,
		Role:         data.Role,
		FullName:     data.FullName,
		IsActive:     true,
	}

	if err := m.repo.Create(user); err != nil {
		return nil, err
	}

	return user, nil
}

// GetByID retrieves a user by ID
func (m *Model) GetByID(id int64) (*db.User, error) {
	return m.repo.GetByID(id)
}

// GetByUUID retrieves a user by UUID
func (m *Model) GetByUUID(uuid string) (*db.User, error) {
	return m.repo.GetByUUID(uuid)
}

// GetByUsername retrieves a user by username
func (m *Model) GetByUsername(username string) (*db.User, error) {
	return m.repo.GetByUsername(username)
}

// GetByEmail retrieves a user by email
func (m *Model) GetByEmail(email string) (*db.User, error) {
	return m.repo.GetByEmail(email)
}

// List retrieves users with pagination
func (m *Model) List(page, pageSize int) ([]*db.User, int64, error) {
	offset := (page - 1) * pageSize
	return m.repo.List(offset, pageSize)
}

// Update updates a user
func (m *Model) Update(id int64, data UpdateUserData) (*db.User, error) {
	user, err := m.repo.GetByID(id)
	if err != nil {
		return nil, err
	}

	if data.FullName != nil {
		user.FullName = *data.FullName
	}
	if data.Role != nil {
		user.Role = *data.Role
	}
	if data.IsActive != nil {
		user.IsActive = *data.IsActive
	}

	if err := m.repo.Update(user); err != nil {
		return nil, err
	}

	return user, nil
}

// UpdatePassword updates a user's password
func (m *Model) UpdatePassword(id int64, passwordHash string) error {
	user, err := m.repo.GetByID(id)
	if err != nil {
		return err
	}

	user.PasswordHash = passwordHash
	return m.repo.Update(user)
}

// Delete deletes a user
func (m *Model) Delete(id int64) error {
	return m.repo.Delete(id)
}

// RecordLogin records a login for a user
func (m *Model) RecordLogin(id int64) error {
	return m.repo.UpdateLastLogin(id)
}

// Activate activates a user
func (m *Model) Activate(id int64) error {
	return m.repo.SetActive(id, true)
}

// Deactivate deactivates a user
func (m *Model) Deactivate(id int64) error {
	return m.repo.SetActive(id, false)
}

// ExistsByUsername checks if a username exists
func (m *Model) ExistsByUsername(username string) (bool, error) {
	return m.repo.ExistsByUsername(username)
}

// ExistsByEmail checks if an email exists
func (m *Model) ExistsByEmail(email string) (bool, error) {
	return m.repo.ExistsByEmail(email)
}

// IsValidRole checks if a role is valid
func IsValidRole(role db.Role) bool {
	switch role {
	case db.RoleAdmin, db.RoleUser, db.RoleReadOnly:
		return true
	default:
		return false
	}
}

// DefaultRoleForUser returns the default role for a new user
func DefaultRoleForUser() db.Role {
	return db.RoleUser
}
