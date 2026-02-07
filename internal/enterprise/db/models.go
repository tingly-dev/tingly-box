package db

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

// Role defines user roles in the enterprise system
type Role string

const (
	RoleAdmin    Role = "admin"
	RoleUser     Role = "user"
	RoleReadOnly Role = "readonly"
)

// User represents an enterprise user
type User struct {
	ID           int64      `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	UUID         string     `gorm:"uniqueIndex;column:uuid;type:varchar(36);not null" json:"uuid"`
	Username     string     `gorm:"uniqueIndex;column:username;type:varchar(64);not null" json:"username"`
	Email        string     `gorm:"uniqueIndex;column:email;type:varchar(255);not null" json:"email"`
	PasswordHash string     `gorm:"column:password_hash;type:varchar(255);not null" json:"-"`
	Role         Role       `gorm:"column:role;type:varchar(20);not null;default:'user'" json:"role"`
	FullName     string     `gorm:"column:full_name;type:varchar(255)" json:"full_name"`
	IsActive     bool       `gorm:"column:is_active;type:boolean;not null;default:true" json:"is_active"`
	CreatedAt    time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
	LastLoginAt  *time.Time `gorm:"column:last_login_at" json:"last_login_at"`

	// Relations
	Tokens []APIToken `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"tokens,omitempty"`
}

// TableName specifies the table name for GORM
func (User) TableName() string {
	return "ent_users"
}

// Scope defines token permission scopes
type Scope string

const (
	ScopeReadProviders  Scope = "read:providers"
	ScopeWriteProviders Scope = "write:providers"
	ScopeReadRules      Scope = "read:rules"
	ScopeWriteRules     Scope = "write:rules"
	ScopeReadUsage      Scope = "read:usage"
	ScopeReadUsers      Scope = "read:users"
	ScopeWriteUsers     Scope = "write:users"
	ScopeReadTokens     Scope = "read:tokens"
	ScopeWriteTokens    Scope = "write:tokens"
	ScopeAdminAll       Scope = "admin:all"
)

// AllScopes returns all available scopes
func AllScopes() []Scope {
	return []Scope{
		ScopeReadProviders,
		ScopeWriteProviders,
		ScopeReadRules,
		ScopeWriteRules,
		ScopeReadUsage,
		ScopeReadUsers,
		ScopeWriteUsers,
		ScopeReadTokens,
		ScopeWriteTokens,
		ScopeAdminAll,
	}
}

// APIToken represents an enterprise API token
type APIToken struct {
	ID          int64      `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	UUID        string     `gorm:"uniqueIndex;column:uuid;type:varchar(36);not null" json:"uuid"`
	UserID      int64      `gorm:"column:user_id;not null;index:idx_ent_api_tokens_user_id" json:"user_id"`
	TokenHash   string     `gorm:"uniqueIndex;column:token_hash;type:varchar(64);not null" json:"-"`
	TokenPrefix string     `gorm:"column:token_prefix;type:varchar(16);not null" json:"token_prefix"`
	Name        string     `gorm:"column:name;type:varchar(255);not null" json:"name"`
	Scopes      string     `gorm:"column:scopes;type:text;not null" json:"scopes"` // JSON array
	ExpiresAt   *time.Time `gorm:"column:expires_at;index" json:"expires_at"`
	LastUsedAt  *time.Time `gorm:"column:last_used_at" json:"last_used_at"`
	IsActive    bool       `gorm:"column:is_active;type:boolean;not null;default:true" json:"is_active"`
	CreatedAt   time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`

	// Relations
	User *User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"user,omitempty"`
}

// TableName specifies the table name for GORM
func (APIToken) TableName() string {
	return "ent_api_tokens"
}

// Session represents a user session
type Session struct {
	ID           int64     `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	UUID         string    `gorm:"uniqueIndex;column:uuid;type:varchar(36);not null" json:"uuid"`
	UserID       int64     `gorm:"column:user_id;not null;index:idx_ent_sessions_user_id" json:"user_id"`
	SessionToken string    `gorm:"uniqueIndex;column:session_token;type:varchar(64);not null" json:"-"`
	RefreshToken string    `gorm:"uniqueIndex;column:refresh_token;type:varchar(64)" json:"-"`
	ExpiresAt    time.Time `gorm:"column:expires_at;not null;index" json:"expires_at"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`

	// Relations
	User *User `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE" json:"user,omitempty"`
}

// TableName specifies the table name for GORM
func (Session) TableName() string {
	return "ent_sessions"
}

// AuditLog represents an audit log entry
type AuditLog struct {
	ID           int64     `gorm:"primaryKey;autoIncrement;column:id" json:"id"`
	UserID       *int64    `gorm:"column:user_id;index:idx_ent_audit_logs_user_id" json:"user_id"`
	Action       string    `gorm:"column:action;type:varchar(100);not null;index:idx_ent_audit_logs_action" json:"action"`
	ResourceType string    `gorm:"column:resource_type;type:varchar(50)" json:"resource_type"`
	ResourceID   string    `gorm:"column:resource_id;type:varchar(36)" json:"resource_id"`
	Details      string    `gorm:"column:details;type:text" json:"details"` // JSON
	IPAddress    string    `gorm:"column:ip_address;type:varchar(45)" json:"ip_address"`
	UserAgent    string    `gorm:"column:user_agent;type:varchar(500)" json:"user_agent"`
	Status       string    `gorm:"column:status;type:varchar(20);not null" json:"status"` // success, failure
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime;index:idx_ent_audit_logs_created_at" json:"created_at"`

	// Relations
	User *User `gorm:"foreignKey:UserID;constraint:OnDelete:SET NULL" json:"user,omitempty"`
}

// TableName specifies the table name for GORM
func (AuditLog) TableName() string {
	return "ent_audit_logs"
}

// EnterpriseDB manages enterprise-specific database operations
type EnterpriseDB struct {
	db *gorm.DB
}

// UserRepository defines user data access interface
type UserRepository interface {
	GetByID(id int64) (*User, error)
	GetByUsername(username string) (*User, error)
	GetByEmail(email string) (*User, error)
	UpdateLastLogin(id int64) error
}

type userRepositoryImpl struct {
	db *gorm.DB
}

// NewUserRepository creates a new user repository
func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepositoryImpl{db: db}
}

func (r *userRepositoryImpl) GetByID(id int64) (*User, error) {
	var user User
	err := r.db.Where("id = ?", id).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}
	return &user, nil
}

func (r *userRepositoryImpl) GetByUsername(username string) (*User, error) {
	var user User
	err := r.db.Where("username = ?", username).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}
	return &user, nil
}

func (r *userRepositoryImpl) GetByEmail(email string) (*User, error) {
	var user User
	err := r.db.Where("email = ?", email).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gorm.ErrRecordNotFound
		}
		return nil, err
	}
	return &user, nil
}

func (r *userRepositoryImpl) UpdateLastLogin(id int64) error {
	return r.db.Model(&User{}).Where("id = ?", id).Update("last_login_at", time.Now()).Error
}

// NewEnterpriseDB creates a new enterprise database manager
func NewEnterpriseDB(db *gorm.DB) *EnterpriseDB {
	return &EnterpriseDB{db: db}
}

// AutoMigrate runs auto-migration for all enterprise models
func (edb *EnterpriseDB) AutoMigrate() error {
	return edb.db.AutoMigrate(
		&User{},
		&APIToken{},
		&Session{},
		&AuditLog{},
	)
}

// GetDB returns the underlying GORM DB instance
func (edb *EnterpriseDB) GetDB() *gorm.DB {
	return edb.db
}
