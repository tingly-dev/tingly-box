package db

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SessionRepository defines session data access interface
type SessionRepository interface {
	// Create creates a new session
	Create(session *Session) error
	// GetByUUID retrieves a session by UUID
	GetByUUID(uuid string) (*Session, error)
	// GetBySessionToken retrieves a session by session token
	GetBySessionToken(token string) (*Session, error)
	// GetByRefreshToken retrieves a session by refresh token
	GetByRefreshToken(token string) (*Session, error)
	// ListByUserID retrieves all sessions for a user
	ListByUserID(userID int64) ([]*Session, error)
	// Delete deletes a session by ID
	Delete(id int64) error
	// DeleteByUUID deletes a session by UUID
	DeleteByUUID(uuid string) error
	// DeleteByUserID deletes all sessions for a user
	DeleteByUserID(userID int64) error
	// CleanupExpired deletes all expired sessions
	CleanupExpired() (int64, error)
}

type sessionRepositoryImpl struct {
	db *gorm.DB
}

// NewSessionRepository creates a new session repository
func NewSessionRepository(gormDB *gorm.DB) SessionRepository {
	return &sessionRepositoryImpl{db: gormDB}
}

func (r *sessionRepositoryImpl) Create(session *Session) error {
	return r.db.Create(session).Error
}

func (r *sessionRepositoryImpl) GetByUUID(uuid string) (*Session, error) {
	var session Session
	err := r.db.Preload("User").Where("uuid = ?", uuid).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *sessionRepositoryImpl) GetBySessionToken(token string) (*Session, error) {
	var session Session
	err := r.db.Preload("User").Where("session_token = ?", token).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *sessionRepositoryImpl) GetByRefreshToken(token string) (*Session, error) {
	var session Session
	err := r.db.Preload("User").Where("refresh_token = ?", token).First(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *sessionRepositoryImpl) ListByUserID(userID int64) ([]*Session, error) {
	var sessions []*Session
	err := r.db.Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&sessions).Error
	return sessions, err
}

func (r *sessionRepositoryImpl) Delete(id int64) error {
	return r.db.Delete(&Session{}, id).Error
}

func (r *sessionRepositoryImpl) DeleteByUUID(uuid string) error {
	return r.db.Where("uuid = ?", uuid).Delete(&Session{}).Error
}

func (r *sessionRepositoryImpl) DeleteByUserID(userID int64) error {
	return r.db.Where("user_id = ?", userID).Delete(&Session{}).Error
}

func (r *sessionRepositoryImpl) CleanupExpired() (int64, error) {
	now := time.Now()
	result := r.db.Where("expires_at < ?", now).Delete(&Session{})
	return result.RowsAffected, result.Error
}

// AuditLogRepository defines audit log data access interface
type AuditLogRepository interface {
	// Create creates a new audit log entry
	Create(log *AuditLog) error
	// GetByID retrieves an audit log by ID
	GetByID(id int64) (*AuditLog, error)
	// List retrieves audit logs with pagination and filters
	List(offset, limit int, filters AuditLogFilters) ([]*AuditLog, int64, error)
	// Delete deletes an audit log by ID
	Delete(id int64) error
	// DeleteOlderThan deletes audit logs older than the specified time
	DeleteOlderThan(cutoff time.Time) (int64, error)
}

type AuditLogFilters struct {
	UserID       *int64
	Action       string
	ResourceType string
	Status       string
	StartDate    *time.Time
	EndDate      *time.Time
}

type auditLogRepositoryImpl struct {
	db *gorm.DB
}

// NewAuditLogRepository creates a new audit log repository
func NewAuditLogRepository(gormDB *gorm.DB) AuditLogRepository {
	return &auditLogRepositoryImpl{db: gormDB}
}

func (r *auditLogRepositoryImpl) Create(log *AuditLog) error {
	return r.db.Create(log).Error
}

func (r *auditLogRepositoryImpl) GetByID(id int64) (*AuditLog, error) {
	var log AuditLog
	err := r.db.Preload("User").Where("id = ?", id).First(&log).Error
	if err != nil {
		return nil, err
	}
	return &log, nil
}

func (r *auditLogRepositoryImpl) List(offset, limit int, filters AuditLogFilters) ([]*AuditLog, int64, error) {
	var logs []*AuditLog
	var total int64

	query := r.db.Model(&AuditLog{})

	// Apply filters
	if filters.UserID != nil {
		query = query.Where("user_id = ?", *filters.UserID)
	}
	if filters.Action != "" {
		query = query.Where("action = ?", filters.Action)
	}
	if filters.ResourceType != "" {
		query = query.Where("resource_type = ?", filters.ResourceType)
	}
	if filters.Status != "" {
		query = query.Where("status = ?", filters.Status)
	}
	if filters.StartDate != nil {
		query = query.Where("created_at >= ?", *filters.StartDate)
	}
	if filters.EndDate != nil {
		query = query.Where("created_at <= ?", *filters.EndDate)
	}

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	err := query.Preload("User").
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&logs).Error

	return logs, total, err
}

func (r *auditLogRepositoryImpl) Delete(id int64) error {
	return r.db.Delete(&AuditLog{}, id).Error
}

func (r *auditLogRepositoryImpl) DeleteOlderThan(cutoff time.Time) (int64, error) {
	result := r.db.Where("created_at < ?", cutoff).Delete(&AuditLog{})
	return result.RowsAffected, result.Error
}

// SessionModel provides session domain operations
type SessionModel struct {
	repo SessionRepository
}

// NewSessionModel creates a new session model
func NewSessionModel(repo SessionRepository) *SessionModel {
	return &SessionModel{repo: repo}
}

// Create creates a new session with hashed tokens
func (m *SessionModel) Create(userID int64, expiryDuration time.Duration) (*Session, string, string, error) {
	sessionUUID := uuid.New().String()
	sessionToken := uuid.New().String()
	refreshToken := uuid.New().String()
	expiresAt := time.Now().Add(expiryDuration)

	// Hash tokens for storage
	sessionTokenHash := hashToken(sessionToken)
	refreshTokenHash := hashToken(refreshToken)

	session := &Session{
		UUID:         sessionUUID,
		UserID:       userID,
		SessionToken: sessionTokenHash,
		RefreshToken: refreshTokenHash,
		ExpiresAt:    expiresAt,
	}

	if err := m.repo.Create(session); err != nil {
		return nil, "", "", err
	}

	return session, sessionToken, refreshToken, nil
}

// ValidateSessionToken validates a session token and returns the session
func (m *SessionModel) ValidateSessionToken(token string) (*Session, error) {
	tokenHash := hashToken(token)
	session, err := m.repo.GetBySessionToken(tokenHash)
	if err != nil {
		return nil, err
	}

	// Check if session has expired
	if session.ExpiresAt.Before(time.Now()) {
		return nil, gorm.ErrRecordNotFound
	}

	return session, nil
}

// ValidateRefreshToken validates a refresh token and returns the session
func (m *SessionModel) ValidateRefreshToken(token string) (*Session, error) {
	tokenHash := hashToken(token)
	session, err := m.repo.GetByRefreshToken(tokenHash)
	if err != nil {
		return nil, err
	}

	// Check if session has expired
	if session.ExpiresAt.Before(time.Now()) {
		return nil, gorm.ErrRecordNotFound
	}

	return session, nil
}

// DeleteByUUID deletes a session by UUID
func (m *SessionModel) DeleteByUUID(uuid string) error {
	return m.repo.DeleteByUUID(uuid)
}

// DeleteByUserID deletes all sessions for a user
func (m *SessionModel) DeleteByUserID(userID int64) error {
	return m.repo.DeleteByUserID(userID)
}

// CleanupExpired removes expired sessions
func (m *SessionModel) CleanupExpired() (int64, error) {
	return m.repo.CleanupExpired()
}

func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}
