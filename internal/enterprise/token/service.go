package token

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tingly-dev/tingly-box/internal/enterprise/db"
)

var (
	// ErrForbidden is returned when user doesn't have permission
	ErrForbidden = errors.New("forbidden")
)

// Service defines the token service interface
type Service interface {
	// CreateToken creates a new API token
	CreateToken(ctx context.Context, req CreateTokenRequest, actor *db.User) (*db.APIToken, string, error)
	// GetToken retrieves a token by ID
	GetToken(ctx context.Context, id int64) (*db.APIToken, error)
	// GetTokenByUUID retrieves a token by UUID
	GetTokenByUUID(ctx context.Context, uuid string) (*db.APIToken, error)
	// ListTokens retrieves all tokens with pagination
	ListTokens(ctx context.Context, page, pageSize int) (*TokenListResponse, error)
	// ListMyTokens retrieves tokens for the current user
	ListMyTokens(ctx context.Context, user *db.User, page, pageSize int) (*TokenListResponse, error)
	// UpdateToken updates a token
	UpdateToken(ctx context.Context, id int64, req UpdateTokenRequest, actor *db.User) (*db.APIToken, error)
	// DeleteToken deletes a token
	DeleteToken(ctx context.Context, id int64, actor *db.User) error
	// DeleteTokenByUUID deletes a token by UUID
	DeleteTokenByUUID(ctx context.Context, uuid string, actor *db.User) error
	// ValidateToken validates a raw token string
	ValidateToken(ctx context.Context, rawToken string) (*db.APIToken, error)
	// RecordUsage records that a token was used
	RecordUsage(ctx context.Context, tokenID int64) error
	// RecordUsageByUUID records that a token was used by UUID
	RecordUsageByUUID(ctx context.Context, uuid string) error
}

// CreateTokenRequest contains data for creating a token
type CreateTokenRequest struct {
	Name      string     `json:"name" binding:"required,min=1,max=255"`
	Scopes    []db.Scope `json:"scopes" binding:"required,min=1"`
	ExpiresAt *time.Time `json:"expires_at"`
	UserID    *int64     `json:"user_id"` // Optional: defaults to current user
}

// UpdateTokenRequest contains data for updating a token
type UpdateTokenRequest struct {
	Name      *string     `json:"name"`
	Scopes    *[]db.Scope `json:"scopes"`
	ExpiresAt *time.Time  `json:"expires_at"`
}

// TokenListResponse contains paginated token list
type TokenListResponse struct {
	Tokens []*TokenWithUserData `json:"tokens"`
	Total  int64                `json:"total"`
	Page   int                  `json:"page"`
	Size   int                  `json:"size"`
}

// serviceImpl implements the Service interface
type serviceImpl struct {
	tokenModel *Model
	auditRepo  db.AuditLogRepository
}

// NewService creates a new token service
func NewService(tokenModel *Model, auditRepo db.AuditLogRepository) Service {
	return &serviceImpl{
		tokenModel: tokenModel,
		auditRepo:  auditRepo,
	}
}

func (s *serviceImpl) CreateToken(ctx context.Context, req CreateTokenRequest, actor *db.User) (*db.APIToken, string, error) {
	// Determine user ID
	userID := actor.ID
	if req.UserID != nil {
		// Only admins can create tokens for other users
		if actor.Role != db.RoleAdmin {
			return nil, ErrForbidden
		}
		userID = *req.UserID
	}

	// Create token
	data := CreateTokenData{
		UserID:    userID,
		Name:      req.Name,
		Scopes:    req.Scopes,
		ExpiresAt: req.ExpiresAt,
	}

	token, rawToken, err := s.tokenModel.Create(data)
	if err != nil {
		return nil, "", err
	}

	// Log audit
	s.logAudit(ctx, actor, "token.create", "token", token.UUID, nil)

	return token, rawToken, nil
}

func (s *serviceImpl) GetToken(ctx context.Context, id int64) (*db.APIToken, error) {
	return s.tokenModel.GetByID(id)
}

func (s *serviceImpl) GetTokenByUUID(ctx context.Context, uuid string) (*db.APIToken, error) {
	return s.tokenModel.GetByUUID(uuid)
}

func (s *serviceImpl) ListTokens(ctx context.Context, page, pageSize int) (*TokenListResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	tokens, total, err := s.tokenModel.List(page, pageSize)
	if err != nil {
		return nil, err
	}

	// Convert to TokenWithUserData
	result := make([]*TokenWithUserData, len(tokens))
	for i, token := range tokens {
		result[i] = &TokenWithUserData{
			APIToken: token,
			Username: token.User.Username,
			Email:    token.User.Email,
		}
	}

	return &TokenListResponse{
		Tokens: result,
		Total:  total,
		Page:   page,
		Size:   pageSize,
	}, nil
}

func (s *serviceImpl) ListMyTokens(ctx context.Context, user *db.User, page, pageSize int) (*TokenListResponse, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	tokens, err := s.tokenModel.ListByUserID(user.ID)
	if err != nil {
		return nil, err
	}

	// Pagination
	total := int64(len(tokens))
	start := (page - 1) * pageSize
	end := start + pageSize

	if start >= len(tokens) {
		return &TokenListResponse{
			Tokens: []*TokenWithUserData{},
			Total:  total,
			Page:   page,
			Size:   pageSize,
		}, nil
	}

	if end > len(tokens) {
		end = len(tokens)
	}

	// Convert to TokenWithUserData
	result := make([]*TokenWithUserData, end-start)
	for i, token := range tokens[start:end] {
		result[i] = &TokenWithUserData{
			APIToken: token,
			Username: user.Username,
			Email:    user.Email,
		}
	}

	return &TokenListResponse{
		Tokens: result,
		Total:  total,
		Page:   page,
		Size:   pageSize,
	}, nil
}

func (s *serviceImpl) UpdateToken(ctx context.Context, id int64, req UpdateTokenRequest, actor *db.User) (*db.APIToken, error) {
	token, err := s.tokenModel.GetByID(id)
	if err != nil {
		return nil, err
	}

	// Check ownership or admin access
	if token.UserID != actor.ID && actor.Role != db.RoleAdmin {
		return nil, ErrForbidden
	}

	// Build update data
	name := token.Name
	scopes, _ := GetScopes(token)
	expiresAt := token.ExpiresAt

	if req.Name != nil {
		name = *req.Name
	}
	if req.Scopes != nil {
		scopes = *req.Scopes
	}
	if req.ExpiresAt != nil {
		expiresAt = req.ExpiresAt
	}

	updatedToken, err := s.tokenModel.Update(id, name, scopes, expiresAt)
	if err != nil {
		return nil, err
	}

	// Log audit
	s.logAudit(ctx, actor, "token.update", "token", token.UUID, nil)

	return updatedToken, nil
}

func (s *serviceImpl) DeleteToken(ctx context.Context, id int64, actor *db.User) error {
	token, err := s.tokenModel.GetByID(id)
	if err != nil {
		return err
	}

	// Check ownership or admin access
	if token.UserID != actor.ID && actor.Role != db.RoleAdmin {
		return ErrForbidden
	}

	if err := s.tokenModel.Delete(id); err != nil {
		return err
	}

	// Log audit
	s.logAudit(ctx, actor, "token.delete", "token", token.UUID, nil)

	return nil
}

func (s *serviceImpl) DeleteTokenByUUID(ctx context.Context, uuid string, actor *db.User) error {
	token, err := s.tokenModel.GetByUUID(uuid)
	if err != nil {
		return err
	}

	// Check ownership or admin access
	if token.UserID != actor.ID && actor.Role != db.RoleAdmin {
		return ErrForbidden
	}

	if err := s.tokenModel.DeleteByUUID(uuid); err != nil {
		return err
	}

	// Log audit
	s.logAudit(ctx, actor, "token.delete", "token", token.UUID, nil)

	return nil
}

func (s *serviceImpl) ValidateToken(ctx context.Context, rawToken string) (*db.APIToken, error) {
	return s.tokenModel.ValidateToken(rawToken)
}

func (s *serviceImpl) RecordUsage(ctx context.Context, tokenID int64) error {
	return s.tokenModel.RecordUsage(tokenID)
}

func (s *serviceImpl) RecordUsageByUUID(ctx context.Context, uuid string) error {
	return s.tokenModel.RecordUsageByUUID(uuid)
}

func (s *serviceImpl) logAudit(ctx context.Context, actor *db.User, action, resourceID string, details map[string]interface{}) {
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
		ResourceType: "token",
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
