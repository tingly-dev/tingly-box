package db

import (
	"gorm.io/gorm"

	"github.com/tingly-dev/tingly-box/remote/access"
)

// migrateNamingModel adds the alias/display-name provenance columns and, once,
// classifies rows written before provenance existed (design:
// .design/bot-im-alias-display.md §4.4). The backfill runs only when a column
// is first added, so it never re-labels data on later restarts.
func migrateNamingModel(db *gorm.DB) error {
	m := db.Migrator()

	if !m.HasColumn(&remoteActorRecord{}, "display_name_source") {
		if err := m.AddColumn(&remoteActorRecord{}, "DisplayNameSource"); err != nil {
			return err
		}
		// A name equal to the external id is an adapter echoing the id, not a name.
		if err := db.Exec(`UPDATE remote_actors SET display_name = '' WHERE display_name = external_actor_id OR (display_name != '' AND external_actor_id LIKE '%:' || display_name)`).Error; err != nil {
			return err
		}
		if err := db.Exec(`UPDATE remote_actors SET display_name_source = ? WHERE display_name IS NOT NULL AND display_name != ''`, access.AliasSourcePlatform).Error; err != nil {
			return err
		}
	}

	if !m.HasColumn(&remoteGroupRecord{}, "alias_source") {
		if err := m.AddColumn(&remoteGroupRecord{}, "AliasSource"); err != nil {
			return err
		}
		if err := db.Exec(`UPDATE remote_groups SET alias_source = ? WHERE alias IS NOT NULL AND alias != ''`, access.AliasSourceUser).Error; err != nil {
			return err
		}
		if err := db.Exec(`UPDATE remote_groups SET alias = name, alias_source = ? WHERE (alias IS NULL OR alias = '') AND name IS NOT NULL AND name != '' AND name != external_group_id`, access.AliasSourcePlatform).Error; err != nil {
			return err
		}
	}

	if !m.HasColumn(&remoteDirectChatRecord{}, "alias_source") {
		if err := m.AddColumn(&remoteDirectChatRecord{}, "AliasSource"); err != nil {
			return err
		}
		if err := db.Exec(`UPDATE remote_direct_chats SET alias_source = ? WHERE alias IS NOT NULL AND alias != ''`, access.AliasSourceUser).Error; err != nil {
			return err
		}
		if err := db.Exec(`UPDATE remote_direct_chats SET
			alias = (SELECT a.display_name FROM remote_actors a WHERE a.id = remote_direct_chats.peer_actor_id),
			alias_source = ?
			WHERE (alias IS NULL OR alias = '') AND peer_actor_id IS NOT NULL
			AND COALESCE((SELECT a.display_name FROM remote_actors a WHERE a.id = remote_direct_chats.peer_actor_id), '') != ''`, access.AliasSourcePlatform).Error; err != nil {
			return err
		}
	}
	return nil
}
