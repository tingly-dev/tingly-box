package user

import (
	"context"
	"errors"
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/enterprise/auth"
	"github.com/tingly-dev/tingly-box/internal/enterprise/db"
)

var (
	// ErrUserAlreadyExists is returned when creating a duplicate user
	ErrUserAlreadyExists = errors.New("user already exists")
	// ErrUserNotFound is returned when a user is not found
	ErrUserNotFound = errors.New("user not found")
	// ErrSelfAction is returned when trying to perform an action on self
	ErrSelfAction = errors.New("cannot perform this action on yourself")
	// ErrLastAdmin is returned when trying to deactivate the last admin
	ErrLastAdmin = errors.New("cannot deactivate the last admin user")
)

// Service defines the user service interface
type Service interface {
	// CreateUser creates a new user
	CreateUser(ctx context.Context, req CreateUserRequest, actor *db.User) (*db.User, error)
	// GetUser retrieves a user by ID
	GetUser(ctx context.Context, id int64) (*db.User, error)
	// GetUserByUUID retrieves a user by UUID
	GetUserByUUID(ctx context.Context, uuid string) (*db.User, error)
	// ListUsers retrieves users with pagination
	ListUsers(ctx context.Context, page, pageSize int) (*UserListResponse, error)
	// UpdateUser updates a user
	UpdateUser(ctx context.Context, id int64, req UpdateUserRequest, actor *db.User) (*db.User, error)
	// DeleteUser deletes a user
	DeleteUser(ctx context.Context, id int64, actor *db.User) error
	// ActivateUser activates a user account
	ActivateUser(ctx context.Context, id int64, actor *db.User) error
	// DeactivateUser deactivates a user account
	DeactivateUser(ctx context.Context, id int64, actor *db.User) error
	// ResetPassword resets a user's password
	ResetPassword(ctx context.Context, id int64, actor *db.User) (string, error)
	// ChangePassword changes a user's password
	ChangePassword(ctx context.Context, req ChangePasswordRequest, user *db.User) error
}

// CreateUserRequest contains data for creating a user
type CreateUserRequest struct {
	Username string    `json:"username" binding:"required,min=3,max=64"`
	Email    string    `json:"email" binding:"required,email"`
	Password string    `json:"password" binding:"required,min=8"`
	FullName string    `json:"full_name"`
	Role     db.Role   `json:"role"`
}

// UpdateUserRequest contains data for updating a user
type UpdateUserRequest struct {
	FullName *string  `json:"full_name"`
	Role     *db.Role `json:"role"`
}

// ChangePasswordRequest contains data for changing password
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required,min=8"`
}

// UserListResponse contains paginated user list
type UserListResponse struct {
	Users []*db.User `json:"users"`
	Total int64      `json:"total"`
	Page  int        `json:"page"`
	Size  int        `json:"size"`
}

// serviceImpl implements the Service interface
type serviceImpl struct {
	userModel    *Model
	passwordSvc  *auth.PasswordService
	auditRepo    db.AuditLogRepository
}

// NewService creates a new user service
func NewService(userModel *Model, passwordSvc *auth.PasswordService, auditRepo db.AuditLogRepository) Service {
	return &serviceImpl{
		userModel:   userModel,
		passwordSvc: passwordSvc,
		auditRepo:   auditRepo,
	}
}

func (s *serviceImpl) CreateUser(ctx context.Context, req CreateUserRequest, actor *db.User) (*db.User, error) {
	// Hash password
	passwordHash, err := s.passwordSvc.HashPassword(req.Password)
	if err != nil {
		return nil, err
	}

	// Create user
	createData := CreateUserData{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: passwordHash,
		Role:         req.Role,
		FullName:     req.FullName,
	}

	user, err := s.userModel.Create(createData)
	if err != nil {
		return nil, err
	}

	// Log audit
	s.logAudit(ctx, actor, "user.create", "user", user.UUID, user.ID, nil)

	return user, nil
}

func (s *serviceImpl) GetUser(ctx context.Context, id int64) (*db.User, error) {
	return s.userModel.GetByID(id)
}

func (s *serviceImpl) GetUserByUUID(ctx context.Context, uuid string) (*db.User, error) {
	return s.userModel.GetByUUID(uuid)
}

func (s *serviceImpl) ListUsers(ctx context.Context, page, pageSize int) (*UserListResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	users, total, err := s.userModel.List(page, pageSize)
	if err != nil {
		return nil, err
	}

	return &UserListResponse{
		Users: users,
		Total: total,
		Page:  page,
		Size:  pageSize,
	}, nil
}

func (s *serviceImpl) UpdateUser(ctx context.Context, id int64, req UpdateUserRequest, actor *db.User) (*db.User, error) {
	user, err := s.userModel.GetByID(id)
	if err != nil {
		return nil, err
	}

	updateData := UpdateUserData{
		FullName: req.FullName,
		Role:     req.Role,
	}

	updatedUser, err := s.userModel.Update(id, updateData)
	if err != nil {
		return nil, err
	}

	// Log audit
	s.logAudit(ctx, actor, "user.update", "user", user.UUID, user.ID, map[string]interface{}{
		"changes": req,
	})

	return updatedUser, nil
}

func (s *serviceImpl) DeleteUser(ctx context.Context, id int64, actor *db.User) error {
	// Check if deleting self
	if actor.ID == id {
		return ErrSelfAction
	}

	user, err := s.userModel.GetByID(id)
	if err != nil {
		return err
	}

	// Check if user is the last admin
	if user.Role == db.RoleAdmin {
		// Count admin users
		admins, _, err := s.userModel.List(1, 1000)
		if err == nil {
			adminCount := 0
			for _, u := range admins {
				if u.Role == db.RoleAdmin && u.IsActive {
					adminCount++
				}
			}
			if adminCount <= 1 {
				return ErrLastAdmin
			}
		}
	}

	if err := s.userModel.Delete(id); err != nil {
		return err
	}

	// Log audit
	s.logAudit(ctx, actor, "user.delete", "user", user.UUID, user.ID, nil)

	return nil
}

func (s *serviceImpl) ActivateUser(ctx context.Context, id int64, actor *db.User) error {
	if err := s.userModel.Activate(id); err != nil {
		return err
	}

	user, _ := s.userModel.GetByID(id)
	if user != nil {
		s.logAudit(ctx, actor, "user.activate", "user", user.UUID, user.ID, nil)
	}

	return nil
}

func (s *serviceImpl) DeactivateUser(ctx context.Context, id int64, actor *db.User) error {
	// Check if deactivating self
	if actor.ID == id {
		return ErrSelfAction
	}

	user, err := s.userModel.GetByID(id)
	if err != nil {
		return err
	}

	// Check if user is the last admin
	if user.Role == db.RoleAdmin {
		admins, _, err := s.userModel.List(1, 1000)
		if err == nil {
			adminCount := 0
			for _, u := range admins {
				if u.Role == db.RoleAdmin && u.IsActive {
					adminCount++
				}
			}
			if adminCount <= 1 {
				return ErrLastAdmin
			}
		}
	}

	if err := s.userModel.Deactivate(id); err != nil {
		return err
	}

	s.logAudit(ctx, actor, "user.deactivate", "user", user.UUID, user.ID, nil)

	return nil
}

func (s *serviceImpl) ResetPassword(ctx context.Context, id int64, actor *db.User) (string, error) {
	// Generate random password
	newPassword, err := s.passwordSvc.GenerateRandomPassword(16)
	if err != nil {
		return "", err
	}

	// Hash new password
	passwordHash, err := s.passwordSvc.HashPassword(newPassword)
	if err != nil {
		return "", err
	}

	// Update password
	if err := s.userModel.UpdatePassword(id, passwordHash); err != nil {
		return "", err
	}

	user, _ := s.userModel.GetByID(id)
	if user != nil {
		s.logAudit(ctx, actor, "user.password_reset", "user", user.UUID, user.ID, nil)
	}

	return newPassword, nil
}

func (s *serviceImpl) ChangePassword(ctx context.Context, req ChangePasswordRequest, user *db.User) error {
	// Validate current password
	valid, err := s.passwordSvc.ValidatePassword(req.CurrentPassword, user.PasswordHash)
	if err != nil {
		return err
	}
	if !valid {
		return auth.ErrInvalidCredentials
	}

	// Validate new password strength
	if err := s.passwordSvc.ValidatePasswordStrength(req.NewPassword); err != nil {
		return err
	}

	// Hash new password
	passwordHash, err := s.passwordSvc.HashPassword(req.NewPassword)
	if err != nil {
		return err
	}

	// Update password
	return s.userModel.UpdatePassword(user.ID, passwordHash)
}

func (s *serviceImpl) logAudit(ctx context.Context, actor *db.User, action, resourceType, resourceID string, resourceDBID int64, details map[string]interface{}) {
	if s.auditRepo == nil {
		return
	}

	var actorID *int64
	if actor != nil {
		actorID = &actor.ID
	}

	detailsJSON := ""
	if details != nil {
		detailsJSON = formatMap(details)
	}

	auditLog := &db.AuditLog{
		UserID:       actorID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Details:      detailsJSON,
		Status:       "success",
	}

	_ = s.auditRepo.Create(auditLog)
}

func formatMap(m map[string]interface{}) string {
	if len(m) == 0 {
		return ""
	}

	result := "{"
	for k, v := range m {
		result += k + ":" + formatValue(v) + ","
	}
	result += "}"
	return result
}

func formatValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case int, int64, uint, uint64:
		return fmt.Sprintf("%d", val)
	case float64:
		return fmt.Sprintf("%f", val)
	case bool:
		return fmt.Sprintf("%t", val)
	default:
		return ""
	}
}
