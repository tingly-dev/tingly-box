package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/tingly-dev/tingly-box/remote/access"
)

var (
	ErrInvalidCapability    = errors.New("invalid bot capability")
	ErrInvalidPermission    = errors.New("invalid permission")
	ErrAccessTargetNotFound = errors.New("access target not found")
	ErrCrossBotTarget       = errors.New("route target belongs to another bot")
)

type botCapabilityRecord struct {
	BotUUID    string    `gorm:"column:bot_uuid;primaryKey"`
	Capability string    `gorm:"column:capability;primaryKey"`
	Enabled    bool      `gorm:"column:enabled;not null"`
	Config     string    `gorm:"column:config;not null"`
	CreatedAt  time.Time `gorm:"column:created_at;not null"`
	UpdatedAt  time.Time `gorm:"column:updated_at;not null"`
}

func (botCapabilityRecord) TableName() string { return "bot_capabilities" }

type remoteActorRecord struct {
	ID              string    `gorm:"column:id;primaryKey"`
	BotUUID         string    `gorm:"column:bot_uuid;not null"`
	Platform        string    `gorm:"column:platform;not null"`
	ExternalActorID string    `gorm:"column:external_actor_id;not null"`
	DisplayName     string    `gorm:"column:display_name"`
	LastSeenAt      time.Time `gorm:"column:last_seen_at"`
	CreatedAt       time.Time `gorm:"column:created_at;not null"`
	UpdatedAt       time.Time `gorm:"column:updated_at;not null"`
}

func (remoteActorRecord) TableName() string { return "remote_actors" }

type remoteDirectChatRecord struct {
	ID             string     `gorm:"column:id;primaryKey"`
	BotUUID        string     `gorm:"column:bot_uuid;not null"`
	Platform       string     `gorm:"column:platform;not null"`
	ExternalChatID string     `gorm:"column:external_chat_id;not null"`
	PeerActorID    *string    `gorm:"column:peer_actor_id"`
	Blocked        bool       `gorm:"column:blocked;not null"`
	PairedAt       *time.Time `gorm:"column:paired_at"`
	ProjectPath    string     `gorm:"column:project_path"`
	BashCwd        string     `gorm:"column:bash_cwd"`
	CurrentAgent   string     `gorm:"column:current_agent"`
	Verbose        *bool      `gorm:"column:verbose"`
	CreatedAt      time.Time  `gorm:"column:created_at;not null"`
	UpdatedAt      time.Time  `gorm:"column:updated_at;not null"`
}

func (remoteDirectChatRecord) TableName() string { return "remote_direct_chats" }

type directChatPermissionRecord struct {
	DirectChatID string    `gorm:"column:direct_chat_id;primaryKey"`
	Capability   string    `gorm:"column:capability;primaryKey"`
	Action       string    `gorm:"column:action;primaryKey"`
	Effect       string    `gorm:"column:effect;not null"`
	UpdatedAt    time.Time `gorm:"column:updated_at;not null"`
}

func (directChatPermissionRecord) TableName() string { return "direct_chat_permissions" }

type remoteGroupRecord struct {
	ID              string    `gorm:"column:id;primaryKey"`
	BotUUID         string    `gorm:"column:bot_uuid;not null"`
	Platform        string    `gorm:"column:platform;not null"`
	ExternalGroupID string    `gorm:"column:external_group_id;not null"`
	Name            string    `gorm:"column:name"`
	Blocked         bool      `gorm:"column:blocked;not null"`
	ProjectPath     string    `gorm:"column:project_path"`
	BashCwd         string    `gorm:"column:bash_cwd"`
	CurrentAgent    string    `gorm:"column:current_agent"`
	Verbose         *bool     `gorm:"column:verbose"`
	CreatedAt       time.Time `gorm:"column:created_at;not null"`
	UpdatedAt       time.Time `gorm:"column:updated_at;not null"`
}

func (remoteGroupRecord) TableName() string { return "remote_groups" }

type groupCapabilityAccessRecord struct {
	GroupID    string    `gorm:"column:group_id;primaryKey"`
	Capability string    `gorm:"column:capability;primaryKey"`
	Effect     string    `gorm:"column:effect;not null"`
	UpdatedAt  time.Time `gorm:"column:updated_at;not null"`
}

func (groupCapabilityAccessRecord) TableName() string { return "group_capability_access" }

type remoteGroupActorRecord struct {
	GroupID   string    `gorm:"column:group_id;primaryKey"`
	ActorID   string    `gorm:"column:actor_id;primaryKey"`
	Label     string    `gorm:"column:label"`
	CreatedAt time.Time `gorm:"column:created_at;not null"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null"`
}

func (remoteGroupActorRecord) TableName() string { return "remote_group_actors" }

type groupActorPermissionRecord struct {
	GroupID    string    `gorm:"column:group_id;primaryKey"`
	ActorID    string    `gorm:"column:actor_id;primaryKey"`
	Capability string    `gorm:"column:capability;primaryKey"`
	Action     string    `gorm:"column:action;primaryKey"`
	Effect     string    `gorm:"column:effect;not null"`
	UpdatedAt  time.Time `gorm:"column:updated_at;not null"`
}

func (groupActorPermissionRecord) TableName() string { return "group_actor_permissions" }

type remoteRouteRecord struct {
	ID           string    `gorm:"column:id;primaryKey"`
	BotUUID      string    `gorm:"column:bot_uuid;not null"`
	Name         string    `gorm:"column:name;not null"`
	Source       string    `gorm:"column:source;not null"`
	EventFilter  string    `gorm:"column:event_filter;not null"`
	DirectChatID *string   `gorm:"column:direct_chat_id"`
	GroupID      *string   `gorm:"column:group_id"`
	Enabled      bool      `gorm:"column:enabled;not null"`
	Options      string    `gorm:"column:options;not null"`
	CreatedAt    time.Time `gorm:"column:created_at;not null"`
	UpdatedAt    time.Time `gorm:"column:updated_at;not null"`
}

func (remoteRouteRecord) TableName() string { return "remote_routes" }

// BotAccessStore owns the final Bot Capability, DirectChat, Group, Actor,
// permission, and Route facts. All multi-row defaults are written in one
// transaction; reads never infer a missing policy.
type BotAccessStore struct {
	db        *gorm.DB
	transport access.TransportFactsSource
}

func NewBotAccessStore(db *gorm.DB) *BotAccessStore { return &BotAccessStore{db: db} }
func (s *BotAccessStore) SetTransportFactsSource(source access.TransportFactsSource) {
	s.transport = source
}

func migrateBotAccessTables(db *gorm.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS bot_capabilities (bot_uuid TEXT NOT NULL, capability TEXT NOT NULL, enabled BOOL NOT NULL, config JSON NOT NULL DEFAULT '{}', created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, PRIMARY KEY(bot_uuid, capability), FOREIGN KEY(bot_uuid) REFERENCES imbot_settings(bot_uuid) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS remote_actors (id TEXT PRIMARY KEY, bot_uuid TEXT NOT NULL, platform TEXT NOT NULL, external_actor_id TEXT NOT NULL, display_name TEXT, last_seen_at DATETIME, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, UNIQUE(bot_uuid, platform, external_actor_id), FOREIGN KEY(bot_uuid) REFERENCES imbot_settings(bot_uuid) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS remote_direct_chats (id TEXT PRIMARY KEY, bot_uuid TEXT NOT NULL, platform TEXT NOT NULL, external_chat_id TEXT NOT NULL, peer_actor_id TEXT, blocked BOOL NOT NULL DEFAULT FALSE, paired_at DATETIME, project_path TEXT, bash_cwd TEXT, current_agent TEXT, verbose BOOL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, UNIQUE(bot_uuid, platform, external_chat_id), FOREIGN KEY(bot_uuid) REFERENCES imbot_settings(bot_uuid) ON DELETE CASCADE, FOREIGN KEY(peer_actor_id) REFERENCES remote_actors(id))`,
		`CREATE TABLE IF NOT EXISTS direct_chat_permissions (direct_chat_id TEXT NOT NULL, capability TEXT NOT NULL, action TEXT NOT NULL, effect TEXT NOT NULL CHECK(effect IN ('allow','deny')), updated_at DATETIME NOT NULL, PRIMARY KEY(direct_chat_id, capability, action), FOREIGN KEY(direct_chat_id) REFERENCES remote_direct_chats(id) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS remote_groups (id TEXT PRIMARY KEY, bot_uuid TEXT NOT NULL, platform TEXT NOT NULL, external_group_id TEXT NOT NULL, name TEXT, blocked BOOL NOT NULL DEFAULT FALSE, project_path TEXT, bash_cwd TEXT, current_agent TEXT, verbose BOOL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, UNIQUE(bot_uuid, platform, external_group_id), FOREIGN KEY(bot_uuid) REFERENCES imbot_settings(bot_uuid) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS group_capability_access (group_id TEXT NOT NULL, capability TEXT NOT NULL, effect TEXT NOT NULL CHECK(effect IN ('allow','deny')), updated_at DATETIME NOT NULL, PRIMARY KEY(group_id, capability), FOREIGN KEY(group_id) REFERENCES remote_groups(id) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS remote_group_actors (group_id TEXT NOT NULL, actor_id TEXT NOT NULL, label TEXT, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, PRIMARY KEY(group_id, actor_id), FOREIGN KEY(group_id) REFERENCES remote_groups(id) ON DELETE CASCADE, FOREIGN KEY(actor_id) REFERENCES remote_actors(id) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS group_actor_permissions (group_id TEXT NOT NULL, actor_id TEXT NOT NULL, capability TEXT NOT NULL, action TEXT NOT NULL, effect TEXT NOT NULL CHECK(effect IN ('allow','deny')), updated_at DATETIME NOT NULL, PRIMARY KEY(group_id, actor_id, capability, action), FOREIGN KEY(group_id, actor_id) REFERENCES remote_group_actors(group_id, actor_id) ON DELETE CASCADE)`,
		`CREATE TABLE IF NOT EXISTS remote_routes (id TEXT PRIMARY KEY, bot_uuid TEXT NOT NULL, name TEXT NOT NULL, source TEXT NOT NULL, event_filter JSON NOT NULL, direct_chat_id TEXT, group_id TEXT, enabled BOOL NOT NULL, options JSON NOT NULL DEFAULT '{}', created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, UNIQUE(bot_uuid, name), CHECK((direct_chat_id IS NOT NULL) != (group_id IS NOT NULL)), FOREIGN KEY(bot_uuid) REFERENCES imbot_settings(bot_uuid) ON DELETE CASCADE, FOREIGN KEY(direct_chat_id) REFERENCES remote_direct_chats(id), FOREIGN KEY(group_id) REFERENCES remote_groups(id))`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}

	// Upgrade legacy Bots to the same explicit defaults used by Bot creation.
	// INSERT OR IGNORE is important here: a user-authored disabled row is a
	// decision, not a missing default, and must survive every restart.
	defaultCapabilities := []string{
		`INSERT OR IGNORE INTO bot_capabilities (bot_uuid, capability, enabled, config, created_at, updated_at) SELECT bot_uuid, 'remote_control', TRUE, '{}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP FROM imbot_settings`,
		`INSERT OR IGNORE INTO bot_capabilities (bot_uuid, capability, enabled, config, created_at, updated_at) SELECT bot_uuid, 'notify', FALSE, '{}', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP FROM imbot_settings`,
	}
	for _, statement := range defaultCapabilities {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func validEffect(effect access.AccessEffect) bool {
	return effect == access.EffectAllow || effect == access.EffectDeny
}
func rawJSONOrObject(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "{}"
	}
	return string(raw)
}

func (s *BotAccessStore) ListCapabilities(ctx context.Context, botUUID string) ([]access.BotCapability, error) {
	var rows []botCapabilityRecord
	if err := s.db.WithContext(ctx).Where("bot_uuid = ?", botUUID).Order("capability").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]access.BotCapability, 0, len(rows))
	for _, r := range rows {
		out = append(out, access.BotCapability{BotUUID: r.BotUUID, Name: access.CapabilityName(r.Capability), Enabled: r.Enabled, Config: json.RawMessage(r.Config), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt})
	}
	return out, nil
}

func (s *BotAccessStore) GetCapability(ctx context.Context, botUUID string, name access.CapabilityName) (access.BotCapability, bool, error) {
	var r botCapabilityRecord
	err := s.db.WithContext(ctx).Where("bot_uuid = ? AND capability = ?", botUUID, name).First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return access.BotCapability{}, false, nil
	}
	if err != nil {
		return access.BotCapability{}, false, err
	}
	return access.BotCapability{BotUUID: r.BotUUID, Name: name, Enabled: r.Enabled, Config: json.RawMessage(r.Config), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}, true, nil
}

func (s *BotAccessStore) PutCapability(ctx context.Context, capability access.BotCapability) error {
	if !capability.Name.Valid() || capability.BotUUID == "" {
		return ErrInvalidCapability
	}
	now := time.Now().UTC()
	row := botCapabilityRecord{BotUUID: capability.BotUUID, Capability: string(capability.Name), Enabled: capability.Enabled, Config: rawJSONOrObject(capability.Config), CreatedAt: now, UpdatedAt: now}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "bot_uuid"}, {Name: "capability"}}, DoUpdates: clause.AssignmentColumns([]string{"enabled", "config", "updated_at"})}).Create(&row).Error
}

func (s *BotAccessStore) AnyCapabilityEnabled(ctx context.Context, botUUID string) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&botCapabilityRecord{}).Where("bot_uuid = ? AND enabled = ?", botUUID, true).Count(&count).Error
	return count > 0, err
}

func upsertActor(tx *gorm.DB, botUUID, platform, externalID, displayName string) (remoteActorRecord, error) {
	if botUUID == "" || platform == "" || externalID == "" {
		return remoteActorRecord{}, fmt.Errorf("actor identity is required")
	}
	now := time.Now().UTC()
	row := remoteActorRecord{ID: uuid.NewString(), BotUUID: botUUID, Platform: platform, ExternalActorID: externalID, DisplayName: displayName, LastSeenAt: now, CreatedAt: now, UpdatedAt: now}
	err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "bot_uuid"}, {Name: "platform"}, {Name: "external_actor_id"}}, DoUpdates: clause.Assignments(map[string]any{"display_name": displayName, "last_seen_at": now, "updated_at": now})}).Create(&row).Error
	if err != nil {
		return remoteActorRecord{}, err
	}
	var stored remoteActorRecord
	err = tx.Where("bot_uuid = ? AND platform = ? AND external_actor_id = ?", botUUID, platform, externalID).First(&stored).Error
	return stored, err
}

func (s *BotAccessStore) UpsertActor(ctx context.Context, actor access.Actor) (access.Actor, error) {
	var row remoteActorRecord
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		row, err = upsertActor(tx, actor.BotUUID, actor.Platform, actor.ExternalActorID, actor.DisplayName)
		return err
	})
	if err != nil {
		return access.Actor{}, err
	}
	return actorFromRecord(row), nil
}

func actorFromRecord(r remoteActorRecord) access.Actor {
	return access.Actor{ID: r.ID, BotUUID: r.BotUUID, Platform: r.Platform, ExternalActorID: r.ExternalActorID, DisplayName: r.DisplayName, LastSeenAt: r.LastSeenAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

func putDirectPermission(tx *gorm.DB, chatID string, capability access.CapabilityName, action access.ActionName, effect access.AccessEffect) error {
	if !capability.Valid() || !validEffect(effect) {
		return ErrInvalidPermission
	}
	row := directChatPermissionRecord{DirectChatID: chatID, Capability: string(capability), Action: string(action), Effect: string(effect), UpdatedAt: time.Now().UTC()}
	return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error
}

func seedDirectPermissions(tx *gorm.DB, chatID string) error {
	rows := []struct {
		c access.CapabilityName
		a access.ActionName
	}{
		{access.CapabilityNotify, access.ActionAccess}, {access.CapabilityNotify, access.ActionNotifyReceive}, {access.CapabilityNotify, access.ActionNotifyReply},
		{access.CapabilityRemoteControl, access.ActionAccess}, {access.CapabilityRemoteControl, access.ActionRemoteControlStart}, {access.CapabilityRemoteControl, access.ActionRemoteControlApprove}, {access.CapabilityRemoteControl, access.ActionRemoteControlPrivileged},
	}
	for _, row := range rows {
		if err := putDirectPermission(tx, chatID, row.c, row.a, access.EffectDeny); err != nil {
			return err
		}
	}
	return nil
}

func (s *BotAccessStore) DiscoverDirectChat(ctx context.Context, botUUID, platform, externalChatID string) (access.DirectChat, error) {
	var row remoteDirectChatRecord
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("bot_uuid = ? AND platform = ? AND external_chat_id = ?", botUUID, platform, externalChatID).First(&row).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		now := time.Now().UTC()
		row = remoteDirectChatRecord{ID: uuid.NewString(), BotUUID: botUUID, Platform: platform, ExternalChatID: externalChatID, CurrentAgent: DefaultChatAgent, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		return seedDirectPermissions(tx, row.ID)
	})
	if err != nil {
		return access.DirectChat{}, err
	}
	return directFromRecord(row), nil
}

func directFromRecord(r remoteDirectChatRecord) access.DirectChat {
	var peer string
	if r.PeerActorID != nil {
		peer = *r.PeerActorID
	}
	return access.DirectChat{ID: r.ID, BotUUID: r.BotUUID, Platform: r.Platform, ExternalChatID: r.ExternalChatID, PeerActorID: peer, Blocked: r.Blocked, PairedAt: r.PairedAt, ProjectPath: r.ProjectPath, BashCwd: r.BashCwd, CurrentAgent: r.CurrentAgent, Verbose: r.Verbose, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

func (s *BotAccessStore) PairDirectChat(ctx context.Context, chatID, externalActorID, displayName string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var chat remoteDirectChatRecord
		if err := tx.Where("id = ?", chatID).First(&chat).Error; err != nil {
			return err
		}
		actor, err := upsertActor(tx, chat.BotUUID, chat.Platform, externalActorID, displayName)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if err := tx.Model(&chat).Updates(map[string]any{"peer_actor_id": actor.ID, "paired_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		for _, a := range []access.ActionName{access.ActionAccess, access.ActionRemoteControlStart, access.ActionRemoteControlApprove} {
			if err := putDirectPermission(tx, chat.ID, access.CapabilityRemoteControl, a, access.EffectAllow); err != nil {
				return err
			}
		}
		return putDirectPermission(tx, chat.ID, access.CapabilityRemoteControl, access.ActionRemoteControlPrivileged, access.EffectDeny)
	})
}

func (s *BotAccessStore) ListDirectChats(ctx context.Context, botUUID string) ([]access.DirectChat, error) {
	var rows []remoteDirectChatRecord
	if err := s.db.WithContext(ctx).Where("bot_uuid = ?", botUUID).Order("updated_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]access.DirectChat, 0, len(rows))
	for _, r := range rows {
		out = append(out, directFromRecord(r))
	}
	return out, nil
}

func (s *BotAccessStore) GetDirectChat(ctx context.Context, botUUID, id string) (access.DirectChat, bool, error) {
	var row remoteDirectChatRecord
	err := s.db.WithContext(ctx).Where("id = ? AND bot_uuid = ?", id, botUUID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return access.DirectChat{}, false, nil
	}
	if err != nil {
		return access.DirectChat{}, false, err
	}
	return directFromRecord(row), true, nil
}

func (s *BotAccessStore) ListDirectChatPermissions(ctx context.Context, botUUID, id string) ([]access.Permission, error) {
	if _, ok, err := s.GetDirectChat(ctx, botUUID, id); err != nil || !ok {
		if err != nil {
			return nil, err
		}
		return nil, ErrAccessTargetNotFound
	}
	var rows []directChatPermissionRecord
	if err := s.db.WithContext(ctx).Where("direct_chat_id = ?", id).Order("capability, action").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]access.Permission, 0, len(rows))
	for _, r := range rows {
		out = append(out, access.Permission{Capability: access.CapabilityName(r.Capability), Action: access.ActionName(r.Action), Effect: access.AccessEffect(r.Effect), UpdatedAt: r.UpdatedAt})
	}
	return out, nil
}

func (s *BotAccessStore) DeleteDirectChat(ctx context.Context, botUUID, id string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var n int64
		if err := tx.Model(&remoteRouteRecord{}).Where("direct_chat_id = ?", id).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			return fmt.Errorf("target_has_routes: %d", n)
		}
		result := tx.Where("id = ? AND bot_uuid = ?", id, botUUID).Delete(&remoteDirectChatRecord{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrAccessTargetNotFound
		}
		return nil
	})
}
func (s *BotAccessStore) SetDirectChatBlocked(ctx context.Context, botUUID, id string, blocked bool) error {
	result := s.db.WithContext(ctx).Model(&remoteDirectChatRecord{}).Where("id = ? AND bot_uuid = ?", id, botUUID).Updates(map[string]any{"blocked": blocked, "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrAccessTargetNotFound
	}
	return nil
}
// SetDirectChatPermissions writes a set of explicit permission rows for one
// Direct Chat in a single transaction. Presets use it so a partial network
// failure can never leave a half-applied permission state (e.g. start=allow
// with approve=deny).
func (s *BotAccessStore) SetDirectChatPermissions(ctx context.Context, botUUID, id string, perms []access.Permission) error {
	if len(perms) == 0 {
		return ErrInvalidPermission
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var n int64
		if err := tx.Model(&remoteDirectChatRecord{}).Where("id = ? AND bot_uuid = ?", id, botUUID).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return ErrAccessTargetNotFound
		}
		for _, p := range perms {
			if err := putDirectPermission(tx, id, p.Capability, p.Action, p.Effect); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *BotAccessStore) SetDirectChatPermission(ctx context.Context, botUUID, id string, capability access.CapabilityName, action access.ActionName, effect access.AccessEffect) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var n int64
		if err := tx.Model(&remoteDirectChatRecord{}).Where("id = ? AND bot_uuid = ?", id, botUUID).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return ErrAccessTargetNotFound
		}
		return putDirectPermission(tx, id, capability, action, effect)
	})
}
func (s *BotAccessStore) UnpairDirectChat(ctx context.Context, botUUID, id string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&remoteDirectChatRecord{}).Where("id = ? AND bot_uuid = ?", id, botUUID).Updates(map[string]any{"peer_actor_id": nil, "paired_at": nil, "updated_at": time.Now().UTC()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrAccessTargetNotFound
		}
		for _, a := range []access.ActionName{access.ActionAccess, access.ActionRemoteControlStart, access.ActionRemoteControlApprove, access.ActionRemoteControlPrivileged} {
			if err := putDirectPermission(tx, id, access.CapabilityRemoteControl, a, access.EffectDeny); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *BotAccessStore) DiscoverGroup(ctx context.Context, botUUID, platform, externalGroupID, name string) (access.Group, error) {
	var row remoteGroupRecord
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("bot_uuid = ? AND platform = ? AND external_group_id = ?", botUUID, platform, externalGroupID).First(&row).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		now := time.Now().UTC()
		row = remoteGroupRecord{ID: uuid.NewString(), BotUUID: botUUID, Platform: platform, ExternalGroupID: externalGroupID, Name: name, CurrentAgent: DefaultChatAgent, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		for _, c := range []access.CapabilityName{access.CapabilityNotify, access.CapabilityRemoteControl} {
			r := groupCapabilityAccessRecord{GroupID: row.ID, Capability: string(c), Effect: string(access.EffectDeny), UpdatedAt: now}
			if err := tx.Create(&r).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return access.Group{}, err
	}
	return groupFromRecord(row), nil
}
func groupFromRecord(r remoteGroupRecord) access.Group {
	return access.Group{ID: r.ID, BotUUID: r.BotUUID, Platform: r.Platform, ExternalGroupID: r.ExternalGroupID, Name: r.Name, Blocked: r.Blocked, ProjectPath: r.ProjectPath, BashCwd: r.BashCwd, CurrentAgent: r.CurrentAgent, Verbose: r.Verbose, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}
func (s *BotAccessStore) ListGroups(ctx context.Context, botUUID string) ([]access.Group, error) {
	var rows []remoteGroupRecord
	if err := s.db.WithContext(ctx).Where("bot_uuid = ?", botUUID).Order("updated_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]access.Group, 0, len(rows))
	for _, r := range rows {
		out = append(out, groupFromRecord(r))
	}
	return out, nil
}

func (s *BotAccessStore) GetGroup(ctx context.Context, botUUID, id string) (access.Group, bool, error) {
	var row remoteGroupRecord
	err := s.db.WithContext(ctx).Where("id = ? AND bot_uuid = ?", id, botUUID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return access.Group{}, false, nil
	}
	if err != nil {
		return access.Group{}, false, err
	}
	return groupFromRecord(row), true, nil
}
func (s *BotAccessStore) ListGroupCapabilities(ctx context.Context, botUUID, id string) (map[access.CapabilityName]access.AccessEffect, error) {
	if _, ok, err := s.GetGroup(ctx, botUUID, id); err != nil || !ok {
		if err != nil {
			return nil, err
		}
		return nil, ErrAccessTargetNotFound
	}
	var rows []groupCapabilityAccessRecord
	if err := s.db.WithContext(ctx).Where("group_id = ?", id).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[access.CapabilityName]access.AccessEffect, len(rows))
	for _, r := range rows {
		out[access.CapabilityName(r.Capability)] = access.AccessEffect(r.Effect)
	}
	return out, nil
}
func (s *BotAccessStore) DeleteGroup(ctx context.Context, botUUID, id string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var n int64
		if err := tx.Model(&remoteRouteRecord{}).Where("group_id = ?", id).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			return fmt.Errorf("target_has_routes: %d", n)
		}
		result := tx.Where("id = ? AND bot_uuid = ?", id, botUUID).Delete(&remoteGroupRecord{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrAccessTargetNotFound
		}
		return nil
	})
}
func (s *BotAccessStore) SetGroupBlocked(ctx context.Context, botUUID, id string, blocked bool) error {
	result := s.db.WithContext(ctx).Model(&remoteGroupRecord{}).Where("id = ? AND bot_uuid = ?", id, botUUID).Updates(map[string]any{"blocked": blocked, "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrAccessTargetNotFound
	}
	return nil
}
func (s *BotAccessStore) SetGroupCapability(ctx context.Context, botUUID, id string, capability access.CapabilityName, effect access.AccessEffect) error {
	if !capability.Valid() || !validEffect(effect) {
		return ErrInvalidPermission
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var n int64
		if err := tx.Model(&remoteGroupRecord{}).Where("id = ? AND bot_uuid = ?", id, botUUID).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return ErrAccessTargetNotFound
		}
		row := groupCapabilityAccessRecord{GroupID: id, Capability: string(capability), Effect: string(effect), UpdatedAt: time.Now().UTC()}
		return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error
	})
}

func putGroupActorPermission(tx *gorm.DB, groupID, actorID string, capability access.CapabilityName, action access.ActionName, effect access.AccessEffect) error {
	if !capability.Valid() || !validEffect(effect) {
		return ErrInvalidPermission
	}
	row := groupActorPermissionRecord{GroupID: groupID, ActorID: actorID, Capability: string(capability), Action: string(action), Effect: string(effect), UpdatedAt: time.Now().UTC()}
	return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error
}
func (s *BotAccessStore) AddGroupActor(ctx context.Context, botUUID, groupID, externalActorID, displayName, label string) (access.Actor, error) {
	var actor remoteActorRecord
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var group remoteGroupRecord
		if err := tx.Where("id = ? AND bot_uuid = ?", groupID, botUUID).First(&group).Error; err != nil {
			return err
		}
		var err error
		actor, err = upsertActor(tx, botUUID, group.Platform, externalActorID, displayName)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		binding := remoteGroupActorRecord{GroupID: groupID, ActorID: actor.ID, Label: label, CreatedAt: now, UpdatedAt: now}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "group_id"}, {Name: "actor_id"}}, DoUpdates: clause.AssignmentColumns([]string{"label", "updated_at"})}).Create(&binding).Error; err != nil {
			return err
		}
		for _, a := range []access.ActionName{access.ActionRemoteControlStart, access.ActionRemoteControlApprove} {
			if err := putGroupActorPermission(tx, groupID, actor.ID, access.CapabilityRemoteControl, a, access.EffectAllow); err != nil {
				return err
			}
		}
		return putGroupActorPermission(tx, groupID, actor.ID, access.CapabilityRemoteControl, access.ActionRemoteControlPrivileged, access.EffectDeny)
	})
	return actorFromRecord(actor), err
}
func (s *BotAccessStore) SetGroupActorPermission(ctx context.Context, botUUID, groupID, actorID string, capability access.CapabilityName, action access.ActionName, effect access.AccessEffect) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var n int64
		if err := tx.Table("remote_groups g").Joins("JOIN remote_group_actors ga ON ga.group_id = g.id").Where("g.id = ? AND g.bot_uuid = ? AND ga.actor_id = ?", groupID, botUUID, actorID).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return ErrAccessTargetNotFound
		}
		return putGroupActorPermission(tx, groupID, actorID, capability, action, effect)
	})
}

func (s *BotAccessStore) ListGroupActors(ctx context.Context, botUUID, groupID string) ([]access.GroupActor, error) {
	if _, ok, err := s.GetGroup(ctx, botUUID, groupID); err != nil || !ok {
		if err != nil {
			return nil, err
		}
		return nil, ErrAccessTargetNotFound
	}
	var bindings []remoteGroupActorRecord
	if err := s.db.WithContext(ctx).Where("group_id = ?", groupID).Order("created_at").Find(&bindings).Error; err != nil {
		return nil, err
	}
	out := make([]access.GroupActor, 0, len(bindings))
	for _, b := range bindings {
		var ar remoteActorRecord
		if err := s.db.WithContext(ctx).Where("id = ?", b.ActorID).First(&ar).Error; err != nil {
			return nil, err
		}
		var prs []groupActorPermissionRecord
		if err := s.db.WithContext(ctx).Where("group_id = ? AND actor_id = ?", groupID, b.ActorID).Order("capability, action").Find(&prs).Error; err != nil {
			return nil, err
		}
		perms := make([]access.Permission, 0, len(prs))
		for _, p := range prs {
			perms = append(perms, access.Permission{Capability: access.CapabilityName(p.Capability), Action: access.ActionName(p.Action), Effect: access.AccessEffect(p.Effect), UpdatedAt: p.UpdatedAt})
		}
		out = append(out, access.GroupActor{GroupID: groupID, Actor: actorFromRecord(ar), Label: b.Label, Permissions: perms, CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt})
	}
	return out, nil
}
func (s *BotAccessStore) RemoveGroupActor(ctx context.Context, botUUID, groupID, actorID string) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var n int64
		if err := tx.Model(&remoteGroupRecord{}).Where("id = ? AND bot_uuid = ?", groupID, botUUID).Count(&n).Error; err != nil {
			return err
		}
		if n == 0 {
			return ErrAccessTargetNotFound
		}
		result := tx.Where("group_id = ? AND actor_id = ?", groupID, actorID).Delete(&remoteGroupActorRecord{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrAccessTargetNotFound
		}
		return nil
	})
}

func routeTargetBotUUID(tx *gorm.DB, target access.TargetRef) (string, error) {
	var botUUID string
	var err error
	switch target.Kind {
	case access.TargetDirectChat:
		err = tx.Model(&remoteDirectChatRecord{}).Select("bot_uuid").Where("id = ?", target.ID).Scan(&botUUID).Error
	case access.TargetGroup:
		err = tx.Model(&remoteGroupRecord{}).Select("bot_uuid").Where("id = ?", target.ID).Scan(&botUUID).Error
	default:
		return "", ErrAccessTargetNotFound
	}
	if err != nil {
		return "", err
	}
	if botUUID == "" {
		return "", ErrAccessTargetNotFound
	}
	return botUUID, nil
}

func grantRouteNotify(tx *gorm.DB, target access.TargetRef, now time.Time) error {
	if target.Kind == access.TargetDirectChat {
		if err := putDirectPermission(tx, target.ID, access.CapabilityNotify, access.ActionAccess, access.EffectAllow); err != nil {
			return err
		}
		return putDirectPermission(tx, target.ID, access.CapabilityNotify, access.ActionNotifyReceive, access.EffectAllow)
	}
	row := groupCapabilityAccessRecord{GroupID: target.ID, Capability: string(access.CapabilityNotify), Effect: string(access.EffectAllow), UpdatedAt: now}
	return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error
}

func (s *BotAccessStore) CreateRoute(ctx context.Context, route access.Route, grantNotify bool) (access.Route, error) {
	if route.ID == "" {
		route.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	route.CreatedAt = now
	route.UpdatedAt = now
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		targetBot, err := routeTargetBotUUID(tx, route.Target)
		if err != nil {
			return err
		}
		if targetBot != route.BotUUID {
			return ErrCrossBotTarget
		}
		row := remoteRouteRecord{ID: route.ID, BotUUID: route.BotUUID, Name: route.Name, Source: route.Source, EventFilter: rawJSONOrObject(route.EventFilter), Enabled: route.Enabled, Options: rawJSONOrObject(route.Options), CreatedAt: now, UpdatedAt: now}
		if route.Target.Kind == access.TargetDirectChat {
			row.DirectChatID = &route.Target.ID
		} else {
			row.GroupID = &route.Target.ID
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if !grantNotify {
			return nil
		}
		return grantRouteNotify(tx, route.Target, now)
	})
	return route, err
}

// UpdateRoute replaces the mutable route definition while preserving its
// stable identity and creation timestamp. Target validation and optional
// Notify grant happen in the same transaction so a cross-bot target can never
// be persisted, even briefly.
func (s *BotAccessStore) UpdateRoute(ctx context.Context, route access.Route, grantNotify bool) (access.Route, error) {
	var updated access.Route
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current remoteRouteRecord
		if err := tx.Where("id = ? AND bot_uuid = ?", route.ID, route.BotUUID).First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAccessTargetNotFound
			}
			return err
		}

		targetBot, err := routeTargetBotUUID(tx, route.Target)
		if err != nil {
			return err
		}
		if targetBot != route.BotUUID {
			return ErrCrossBotTarget
		}

		now := time.Now().UTC()
		current.Name = route.Name
		current.Source = route.Source
		current.EventFilter = rawJSONOrObject(route.EventFilter)
		current.Enabled = route.Enabled
		current.Options = rawJSONOrObject(route.Options)
		current.DirectChatID = nil
		current.GroupID = nil
		current.UpdatedAt = now
		if route.Target.Kind == access.TargetDirectChat {
			current.DirectChatID = &route.Target.ID
		} else {
			current.GroupID = &route.Target.ID
		}
		if err := tx.Save(&current).Error; err != nil {
			return err
		}
		if grantNotify {
			if err := grantRouteNotify(tx, route.Target, now); err != nil {
				return err
			}
		}
		updated = routeFromRecord(current)
		return nil
	})
	return updated, err
}

func routeFromRecord(r remoteRouteRecord) access.Route {
	target := access.TargetRef{}
	if r.DirectChatID != nil {
		target = access.TargetRef{Kind: access.TargetDirectChat, ID: *r.DirectChatID}
	} else if r.GroupID != nil {
		target = access.TargetRef{Kind: access.TargetGroup, ID: *r.GroupID}
	}
	return access.Route{ID: r.ID, BotUUID: r.BotUUID, Name: r.Name, Source: r.Source, EventFilter: json.RawMessage(r.EventFilter), Target: target, Enabled: r.Enabled, Options: json.RawMessage(r.Options), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}
func (s *BotAccessStore) ListRoutes(ctx context.Context, botUUID string) ([]access.Route, error) {
	var rows []remoteRouteRecord
	if err := s.db.WithContext(ctx).Where("bot_uuid = ?", botUUID).Order("name").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]access.Route, 0, len(rows))
	for _, r := range rows {
		out = append(out, routeFromRecord(r))
	}
	return out, nil
}
func (s *BotAccessStore) GetRoute(ctx context.Context, botUUID, id string) (access.Route, bool, error) {
	var row remoteRouteRecord
	err := s.db.WithContext(ctx).Where("id = ? AND bot_uuid = ?", id, botUUID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return access.Route{}, false, nil
	}
	if err != nil {
		return access.Route{}, false, err
	}
	return routeFromRecord(row), true, nil
}
func (s *BotAccessStore) DeleteRoute(ctx context.Context, botUUID, id string) error {
	result := s.db.WithContext(ctx).Where("id = ? AND bot_uuid = ?", id, botUUID).Delete(&remoteRouteRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrAccessTargetNotFound
	}
	return nil
}

func routeAllowsEvent(raw, event string) bool {
	if event == "" {
		return true
	}
	var events []string
	if err := json.Unmarshal([]byte(raw), &events); err != nil {
		return false
	}
	if len(events) == 0 {
		return true
	}
	for _, candidate := range events {
		if strings.EqualFold(candidate, event) {
			return true
		}
	}
	return false
}

func (s *BotAccessStore) ResolveRoute(ctx context.Context, source, event string) (*access.ResolvedRoute, error) {
	var rows []remoteRouteRecord
	if err := s.db.WithContext(ctx).Where("source = ? AND enabled = ?", source, true).Order("created_at").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if !routeAllowsEvent(row.EventFilter, event) {
			continue
		}
		resolved := routeFromRecord(row)
		var external string
		if resolved.Target.Kind == access.TargetDirectChat {
			var target remoteDirectChatRecord
			if err := s.db.WithContext(ctx).Where("id = ? AND bot_uuid = ?", resolved.Target.ID, row.BotUUID).First(&target).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					continue
				}
				return nil, err
			}
			external = target.ExternalChatID
		} else {
			var target remoteGroupRecord
			if err := s.db.WithContext(ctx).Where("id = ? AND bot_uuid = ?", resolved.Target.ID, row.BotUUID).First(&target).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					continue
				}
				return nil, err
			}
			external = target.ExternalGroupID
		}
		return &access.ResolvedRoute{Route: resolved, ExternalTargetID: external}, nil
	}
	return nil, nil
}

func getEffect(tx *gorm.DB, table, where string, args ...any) access.AccessEffect {
	var effect string
	if err := tx.Table(table).Select("effect").Where(where, args...).Scan(&effect).Error; err != nil {
		return ""
	}
	return access.AccessEffect(effect)
}

// Snapshot implements access.FactSource. Missing rows stay zero-valued and
// therefore fail closed in access.Evaluate.
func (s *BotAccessStore) Snapshot(ctx context.Context, req access.AuthorizationRequest) (facts access.DecisionFacts, err error) {
	if s.transport != nil {
		facts.TransportStatus, facts.TransportSupports = s.transport.TransportFacts(req.BotUUID, req.Capability, req.Action)
	} else {
		facts.TransportStatus = access.TransportOffline
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var enabled bool
		if e := tx.Model(&ImBotSettingsRecord{}).Select("enabled").Where("bot_uuid = ?", req.BotUUID).Scan(&enabled).Error; e != nil {
			return e
		}
		facts.BotEnabled = enabled
		var capRow botCapabilityRecord
		e := tx.Where("bot_uuid = ? AND capability = ?", req.BotUUID, req.Capability).First(&capRow).Error
		if e == nil {
			facts.CapabilityEnabled = capRow.Enabled
		} else if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		switch req.Target.Kind {
		case access.TargetDirectChat:
			var target remoteDirectChatRecord
			e := tx.Where("id = ? AND bot_uuid = ?", req.Target.ID, req.BotUUID).First(&target).Error
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return nil
			}
			if e != nil {
				return e
			}
			facts.TargetFound = true
			facts.TargetBlocked = target.Blocked
			if target.PeerActorID != nil {
				facts.PeerActorID = *target.PeerActorID
			}
			facts.TargetCapability = getEffect(tx, "direct_chat_permissions", "direct_chat_id = ? AND capability = ? AND action = ?", target.ID, req.Capability, access.ActionAccess)
			facts.TargetAction = getEffect(tx, "direct_chat_permissions", "direct_chat_id = ? AND capability = ? AND action = ?", target.ID, req.Capability, req.Action)
			facts.ActorAction = facts.TargetAction
		case access.TargetGroup:
			var target remoteGroupRecord
			e := tx.Where("id = ? AND bot_uuid = ?", req.Target.ID, req.BotUUID).First(&target).Error
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return nil
			}
			if e != nil {
				return e
			}
			facts.TargetFound = true
			facts.TargetBlocked = target.Blocked
			facts.TargetCapability = getEffect(tx, "group_capability_access", "group_id = ? AND capability = ?", target.ID, req.Capability)
			facts.TargetAction = facts.TargetCapability
			if req.ActorID != "" {
				var n int64
				if e := tx.Model(&remoteGroupActorRecord{}).Where("group_id = ? AND actor_id = ?", target.ID, req.ActorID).Count(&n).Error; e != nil {
					return e
				}
				facts.ActorRegistered = n > 0
				facts.ActorAction = getEffect(tx, "group_actor_permissions", "group_id = ? AND actor_id = ? AND capability = ? AND action = ?", target.ID, req.ActorID, req.Capability, req.Action)
			}
		default:
			return nil
		}
		if req.RouteID != "" {
			var route remoteRouteRecord
			e := tx.Where("id = ? AND bot_uuid = ?", req.RouteID, req.BotUUID).First(&route).Error
			if e == nil {
				facts.RouteFound = true
				facts.RouteEnabled = route.Enabled
				if route.DirectChatID != nil {
					facts.RouteTarget = access.TargetRef{Kind: access.TargetDirectChat, ID: *route.DirectChatID}
				} else if route.GroupID != nil {
					facts.RouteTarget = access.TargetRef{Kind: access.TargetGroup, ID: *route.GroupID}
				}
			} else if !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
		}
		return nil
	})
	return facts, err
}

var _ access.FactSource = (*BotAccessStore)(nil)
var _ access.RouteResolver = (*BotAccessStore)(nil)
