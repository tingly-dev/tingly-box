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
	// Each table's new column and its backfill commit together: a half-applied
	// backfill would leave "column exists" true and the backfill skipped for good.
	steps := []struct {
		model  any
		column string // the column whose absence means "not migrated yet"
		field  string // its Go field name
		fill   func(tx *gorm.DB) error
	}{
		{&remoteActorRecord{}, "display_name_source", "DisplayNameSource", func(tx *gorm.DB) error {
			// A name equal to the external id, or to its part after "platform:", is
			// an adapter echoing the id rather than a name. Plain string comparison
			// (not LIKE) so '%', '_' and letter case in real names cannot match.
			if err := tx.Exec(`UPDATE remote_actors SET display_name = ''
				WHERE display_name != '' AND (display_name = external_actor_id
				OR substr(external_actor_id, -(length(display_name) + 1)) = ':' || display_name)`).Error; err != nil {
				return err
			}
			return tx.Exec(`UPDATE remote_actors SET display_name_source = ? WHERE display_name IS NOT NULL AND display_name != ''`, access.AliasSourcePlatform).Error
		}},
		{&remoteGroupRecord{}, "alias_source", "AliasSource", func(tx *gorm.DB) error {
			if err := tx.Exec(`UPDATE remote_groups SET alias_source = ? WHERE alias IS NOT NULL AND alias != ''`, access.AliasSourceUser).Error; err != nil {
				return err
			}
			return tx.Exec(`UPDATE remote_groups SET alias = name, alias_source = ? WHERE (alias IS NULL OR alias = '') AND name IS NOT NULL AND name != '' AND name != external_group_id`, access.AliasSourcePlatform).Error
		}},
		{&remoteDirectChatRecord{}, "alias_source", "AliasSource", func(tx *gorm.DB) error {
			if err := tx.Exec(`UPDATE remote_direct_chats SET alias_source = ? WHERE alias IS NOT NULL AND alias != ''`, access.AliasSourceUser).Error; err != nil {
				return err
			}
			return tx.Exec(`UPDATE remote_direct_chats SET
				alias = (SELECT a.display_name FROM remote_actors a WHERE a.id = remote_direct_chats.peer_actor_id),
				alias_source = ?
				WHERE (alias IS NULL OR alias = '') AND peer_actor_id IS NOT NULL
				AND COALESCE((SELECT a.display_name FROM remote_actors a WHERE a.id = remote_direct_chats.peer_actor_id), '') != ''`, access.AliasSourcePlatform).Error
		}},
	}
	for _, step := range steps {
		if db.Migrator().HasColumn(step.model, step.column) {
			continue
		}
		err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Migrator().AddColumn(step.model, step.field); err != nil {
				return err
			}
			return step.fill(tx)
		})
		if err != nil {
			return err
		}
	}
	return nil
}
