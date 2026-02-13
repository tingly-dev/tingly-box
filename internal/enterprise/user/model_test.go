package user

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/internal/enterprise/auth"
	"github.com/tingly-dev/tingly-box/internal/enterprise/db"
)

// mockUserRepository is a mock implementation for testing
type mockUserRepository struct {
	users  map[int64]*db.User
	nextID int64
}

func newMockUserRepository() *mockUserRepository {
	return &mockUserRepository{
		users:  make(map[int64]*db.User),
		nextID: 1,
	}
}

func (m *mockUserRepository) Create(user *db.User) error {
	user.ID = m.nextID
	m.users[m.nextID] = user
	m.nextID++
	return nil
}

func (m *mockUserRepository) GetByID(id int64) (*db.User, error) {
	user, exists := m.users[id]
	if !exists {
		return nil, ErrUserNotFound
	}
	return user, nil
}

func (m *mockUserRepository) GetByUsername(username string) (*db.User, error) {
	for _, user := range m.users {
		if user.Username == username {
			return user, nil
		}
	}
	return nil, ErrUserNotFound
}

func (m *mockUserRepository) GetByEmail(email string) (*db.User, error) {
	for _, user := range m.users {
		if user.Email == email {
			return user, nil
		}
	}
	return nil, ErrUserNotFound
}

func (m *mockUserRepository) List(offset, limit int) ([]*db.User, int64, error) {
	users := make([]*db.User, 0, len(m.users))
	for _, user := range m.users {
		users = append(users, user)
	}
	total := int64(len(users))

	if offset >= len(users) {
		return []*db.User{}, total, nil
	}

	end := offset + limit
	if end > len(users) {
		end = len(users)
	}

	return users[offset:end], total, nil
}

func (m *mockUserRepository) Update(user *db.User) error {
	m.users[user.ID] = user
	return nil
}

func (m *mockUserRepository) Delete(id int64) error {
	delete(m.users, id)
	return nil
}

func (m *mockUserRepository) UpdateLastLogin(id int64) error {
	user, exists := m.users[id]
	if !exists {
		return ErrUserNotFound
	}
	// In real implementation, this would update timestamp
	user.LastLoginAt = &[]time.Time{time.Now()}[0]
	m.users[id] = user
	return nil
}

func (m *mockUserRepository) SetActive(id int64, active bool) error {
	user, exists := m.users[id]
	if !exists {
		return ErrUserNotFound
	}
	user.IsActive = active
	m.users[id] = user
	return nil
}

func (m *mockUserRepository) ExistsByUsername(username string) (bool, error) {
	for _, user := range m.users {
		if user.Username == username {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockUserRepository) ExistsByEmail(email string) (bool, error) {
	for _, user := range m.users {
		if user.Email == email {
			return true, nil
		}
	}
	return false, nil
}

func TestModel_Create(t *testing.T) {
	repo := newMockUserRepository()
	model := NewModel(repo)

	createData := CreateUserData{
		Username:     "testuser",
		Email:        "test@example.com",
		PasswordHash: "hash",
		Role:         db.RoleUser,
		FullName:     "Test User",
	}

	user, err := model.Create(createData)

	require.NoError(t, err)
	assert.NotNil(t, user)
	assert.Equal(t, "testuser", user.Username)
	assert.Equal(t, "test@example.com", user.Email)
	assert.Equal(t, db.RoleUser, user.Role)
	assert.True(t, user.IsActive)
}

func TestModel_GetByUsername(t *testing.T) {
	repo := newMockUserRepository()
	model := NewModel(repo)

	// Create a user first
	createData := CreateUserData{
		Username:     "testuser",
		Email:        "test@example.com",
		PasswordHash: "hash",
		Role:         db.RoleUser,
		FullName:     "Test User",
	}
	_, err := model.Create(createData)
	require.NoError(t, err)

	// Get the user
	user, err := model.GetByUsername("testuser")

	require.NoError(t, err)
	assert.NotNil(t, user)
	assert.Equal(t, "testuser", user.Username)

	// Try to get non-existent user
	user, err = model.GetByUsername("nonexistent")
	assert.Error(t, err)
	assert.Nil(t, user)
}

func TestModel_ExistsByUsername(t *testing.T) {
	repo := newMockUserRepository()
	model := NewModel(repo)

	// Create a user first
	createData := CreateUserData{
		Username:     "testuser",
		Email:        "test@example.com",
		PasswordHash: "hash",
		Role:         db.RoleUser,
		FullName:     "Test User",
	}
	_, err := model.Create(createData)
	require.NoError(t, err)

	// Check existing username
	exists, err := model.ExistsByUsername("testuser")
	require.NoError(t, err)
	assert.True(t, exists)

	// Check non-existent username
	exists, err = model.ExistsByUsername("nonexistent")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestModel_ActivateDeactivate(t *testing.T) {
	repo := newMockUserRepository()
	model := NewModel(repo)

	// Create a user first
	createData := CreateUserData{
		Username:     "testuser",
		Email:        "test@example.com",
		PasswordHash: "hash",
		Role:         db.RoleUser,
		FullName:     "Test User",
	}
	user, err := model.Create(createData)
	require.NoError(t, err)

	// Deactivate user
	err = model.Deactivate(user.ID)
	require.NoError(t, err)

	deactivatedUser, err := model.GetByID(user.ID)
	require.NoError(t, err)
	assert.False(t, deactivatedUser.IsActive)

	// Activate user
	err = model.Activate(user.ID)
	require.NoError(t, err)

	activatedUser, err := model.GetByID(user.ID)
	require.NoError(t, err)
	assert.True(t, activatedUser.IsActive)
}

func TestModel_List(t *testing.T) {
	repo := newMockUserRepository()
	model := NewModel(repo)

	// Create multiple users
	for i := 0; i < 5; i++ {
		createData := CreateUserData{
			Username:     fmt.Sprintf("user%d", i),
			Email:        fmt.Sprintf("user%d@example.com", i),
			PasswordHash: "hash",
			Role:         db.RoleUser,
			FullName:     fmt.Sprintf("User %d", i),
		}
		_, err := model.Create(createData)
		require.NoError(t, err)
	}

	// List users
	users, total, err := model.List(1, 10)
	require.NoError(t, err)
	assert.Equal(t, 5, len(users))
	assert.Equal(t, int64(5), total)
}

func TestModel_UpdatePassword(t *testing.T) {
	repo := newMockUserRepository()
	model := NewModel(repo)

	// Create a user first
	createData := CreateUserData{
		Username:     "testuser",
		Email:        "test@example.com",
		PasswordHash: "oldhash",
		Role:         db.RoleUser,
		FullName:     "Test User",
	}
	user, err := model.Create(createData)
	require.NoError(t, err)

	// Update password
	err = model.UpdatePassword(user.ID, "newhash")
	require.NoError(t, err)

	updatedUser, err := model.GetByID(user.ID)
	require.NoError(t, err)
	assert.Equal(t, "newhash", updatedUser.PasswordHash)
}

func TestModel_Delete(t *testing.T) {
	repo := newMockUserRepository()
	model := NewModel(repo)

	// Create a user first
	createData := CreateTestData{
		Username:     "testuser",
		Email:        "test@example.com",
		PasswordHash: "hash",
		Role:         db.RoleUser,
		FullName:     "Test User",
	}
	user, err := model.Create(createData)
	require.NoError(t, err)

	// Delete user
	err = model.Delete(user.ID)
	require.NoError(t, err)

	// Verify user is deleted
	_, err = model.GetByID(user.ID)
	assert.Error(t, err)
}

func TestService_CreateUser(t *testing.T) {
	repo := newMockUserRepository()
	model := NewModel(repo)
	passwordSvc := auth.NewPasswordService(auth.DefaultPasswordConfig())
	service := NewService(model, passwordSvc, nil)

	req := CreateUserRequest{
		Username: "testuser",
		Email:    "test@example.com",
		Password: "TestPass123!",
		FullName: "Test User",
		Role:     db.RoleUser,
	}

	actor := &db.User{
		ID:       1,
		Username: "admin",
		Role:     db.RoleAdmin,
		IsActive: true,
	}

	user, err := service.CreateUser(context.Background(), req, actor)

	require.NoError(t, err)
	assert.NotNil(t, user)
	assert.Equal(t, "testuser", user.Username)
	assert.NotEqual(t, "TestPass123!", user.PasswordHash) // Password should be hashed
}

func TestService_CreateUser_DuplicateUsername(t *testing.T) {
	repo := newMockUserRepository()
	model := NewModel(repo)
	passwordSvc := auth.NewPasswordService(auth.DefaultPasswordConfig())
	service := NewService(model, passwordSvc, nil)

	// Create first user
	req := CreateUserRequest{
		Username: "testuser",
		Email:    "test1@example.com",
		Password: "TestPass123!",
		Role:     db.RoleUser,
	}

	actor := &db.User{
		ID:       1,
		Username: "admin",
		Role:     db.RoleAdmin,
		IsActive: true,
	}

	_, err := service.CreateUser(context.Background(), req, actor)
	require.NoError(t, err)

	// Try to create user with same username
	req2 := CreateUserRequest{
		Username: "testuser",
		Email:    "test2@example.com",
		Password: "TestPass123!",
		Role:     db.RoleUser,
	}

	_, err = service.CreateUser(context.Background(), req2, actor)
	assert.Error(t, err)
}

func TestService_ChangePassword(t *testing.T) {
	repo := newMockUserRepository()
	model := NewModel(repo)
	passwordSvc := auth.NewPasswordService(auth.DefaultPasswordConfig())
	service := NewService(model, passwordSvc, nil)

	// Create user with known password
	passwordHash, err := passwordSvc.HashPassword("OldPass123!")
	require.NoError(t, err)

	userData := CreateUserData{
		Username:     "testuser",
		Email:        "test@example.com",
		PasswordHash: passwordHash,
		Role:         db.RoleUser,
	}
	user, err := model.Create(userData)
	require.NoError(t, err)

	// Change password
	req := ChangePasswordRequest{
		CurrentPassword: "OldPass123!",
		NewPassword:     "NewPass456!",
	}

	err = service.ChangePassword(context.Background(), req, user)
	require.NoError(t, err)

	// Verify password was changed
	updatedUser, err := model.GetByID(user.ID)
	require.NoError(t, err)
	assert.NotEqual(t, passwordHash, updatedUser.PasswordHash)

	// Verify new password works
	valid, err := passwordSvc.ValidatePassword("NewPass456!", updatedUser.PasswordHash)
	require.NoError(t, err)
	assert.True(t, valid)

	// Verify old password doesn't work
	valid, err = passwordSvc.ValidatePassword("OldPass123!", updatedUser.PasswordHash)
	require.NoError(t, err)
	assert.False(t, valid)
}

func TestService_ChangePassword_WrongCurrent(t *testing.T) {
	repo := newMockUserRepository()
	model := NewModel(repo)
	passwordSvc := auth.NewPasswordService(auth.DefaultPasswordConfig())
	service := NewService(model, passwordSvc, nil)

	// Create user
	passwordHash, _ := passwordSvc.HashPassword("OldPass123!")
	userData := CreateUserData{
		Username:     "testuser",
		Email:        "test@example.com",
		PasswordHash: passwordHash,
		Role:         db.RoleUser,
	}
	user, err := model.Create(userData)
	require.NoError(t, err)

	// Try to change with wrong current password
	req := ChangePasswordRequest{
		CurrentPassword: "WrongPass123!",
		NewPassword:     "NewPass456!",
	}

	err = service.ChangePassword(context.Background(), req, user)
	assert.Error(t, err)
	assert.Equal(t, auth.ErrInvalidCredentials, err)
}
