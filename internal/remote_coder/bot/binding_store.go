package bot

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// BindingStore handles group-project binding persistence
type BindingStore struct {
	db *sql.DB
}

// NewBindingStore creates a new binding store
func NewBindingStore(db *sql.DB) (*BindingStore, error) {
	if db == nil {
		return nil, fmt.Errorf("db is nil")
	}
	return &BindingStore{db: db}, nil
}

// CreateGroupBinding creates a new group-project binding
func (s *BindingStore) CreateGroupBinding(binding *GroupProjectBinding) error {
	if binding.ID == "" {
		binding.ID = uuid.New().String()
	}

	now := time.Now().UTC()
	binding.CreatedAt = now
	binding.UpdatedAt = now

	_, err := s.db.Exec(`
		INSERT INTO remote_coder_group_bindings (id, group_id, platform, project_id, bot_uuid, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, binding.ID, binding.GroupID, binding.Platform, binding.ProjectID, binding.BotUUID,
		binding.CreatedAt.Format(time.RFC3339), binding.UpdatedAt.Format(time.RFC3339))

	return err
}

// GetGroupBinding retrieves a group-project binding by group ID and platform
func (s *BindingStore) GetGroupBinding(groupID, platform string) (*GroupProjectBinding, error) {
	row := s.db.QueryRow(`
		SELECT id, group_id, platform, project_id, bot_uuid, created_at, updated_at
		FROM remote_coder_group_bindings WHERE group_id = ? AND platform = ?
	`, groupID, platform)

	return scanGroupBinding(row)
}

// GetGroupBindingByProject retrieves all group bindings for a project
func (s *BindingStore) GetGroupBindingsByProject(projectID string) ([]*GroupProjectBinding, error) {
	rows, err := s.db.Query(`
		SELECT id, group_id, platform, project_id, bot_uuid, created_at, updated_at
		FROM remote_coder_group_bindings WHERE project_id = ?
		ORDER BY created_at DESC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bindings []*GroupProjectBinding
	for rows.Next() {
		binding, err := scanGroupBindingFromRows(rows)
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, binding)
	}

	return bindings, rows.Err()
}

// UpdateGroupBinding updates the project binding for a group
func (s *BindingStore) UpdateGroupBinding(binding *GroupProjectBinding) error {
	binding.UpdatedAt = time.Now().UTC()

	result, err := s.db.Exec(`
		UPDATE remote_coder_group_bindings SET
			project_id = ?, bot_uuid = ?, updated_at = ?
		WHERE group_id = ? AND platform = ?
	`, binding.ProjectID, binding.BotUUID, binding.UpdatedAt.Format(time.RFC3339),
		binding.GroupID, binding.Platform)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("group binding for %s/%s not found", binding.Platform, binding.GroupID)
	}

	return nil
}

// UpsertGroupBinding creates or updates a group-project binding
func (s *BindingStore) UpsertGroupBinding(binding *GroupProjectBinding) error {
	existing, err := s.GetGroupBinding(binding.GroupID, binding.Platform)
	if err != nil {
		return err
	}

	if existing == nil {
		return s.CreateGroupBinding(binding)
	}

	// Update existing binding
	existing.ProjectID = binding.ProjectID
	existing.BotUUID = binding.BotUUID
	return s.UpdateGroupBinding(existing)
}

// ListGroupBindingsByOwner lists all group bindings for projects owned by a user
func (s *BindingStore) ListGroupBindingsByOwner(ownerID, platform string) ([]*ProjectWithBinding, error) {
	rows, err := s.db.Query(`
		SELECT
			p.id, p.path, p.name, p.owner_id, p.platform, p.bot_uuid, p.created_at, p.updated_at,
			b.id, b.group_id, b.platform, b.project_id, b.bot_uuid, b.created_at, b.updated_at
		FROM remote_coder_projects p
		LEFT JOIN remote_coder_group_bindings b ON p.id = b.project_id
		WHERE p.owner_id = ? AND p.platform = ?
		ORDER BY p.created_at DESC
	`, ownerID, platform)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*ProjectWithBinding
	for rows.Next() {
		var project Project
		var binding GroupProjectBinding
		var projectCreatedAt, projectUpdatedAt, bindingCreatedAt, bindingUpdatedAt sql.NullString
		var bindingID, bindingGroupID, bindingPlatform, bindingProjectID, bindingBotUUID sql.NullString

		err := rows.Scan(
			&project.ID, &project.Path, &project.Name, &project.OwnerID,
			&project.Platform, &project.BotUUID, &projectCreatedAt, &projectUpdatedAt,
			&bindingID, &bindingGroupID, &bindingPlatform, &bindingProjectID,
			&bindingBotUUID, &bindingCreatedAt, &bindingUpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		if projectCreatedAt.Valid {
			project.CreatedAt, _ = time.Parse(time.RFC3339, projectCreatedAt.String)
		}
		if projectUpdatedAt.Valid {
			project.UpdatedAt, _ = time.Parse(time.RFC3339, projectUpdatedAt.String)
		}

		result := &ProjectWithBinding{
			Project: &project,
		}

		// Only include binding if it exists (not null)
		if bindingID.Valid {
			binding.ID = bindingID.String
			binding.GroupID = bindingGroupID.String
			binding.Platform = bindingPlatform.String
			binding.ProjectID = bindingProjectID.String
			binding.BotUUID = bindingBotUUID.String
			if bindingCreatedAt.Valid {
				binding.CreatedAt, _ = time.Parse(time.RFC3339, bindingCreatedAt.String)
			}
			if bindingUpdatedAt.Valid {
				binding.UpdatedAt, _ = time.Parse(time.RFC3339, bindingUpdatedAt.String)
			}
			result.Binding = &binding
		}

		results = append(results, result)
	}

	return results, rows.Err()
}

// DeleteGroupBinding deletes a group-project binding by ID
func (s *BindingStore) DeleteGroupBinding(id string) error {
	result, err := s.db.Exec(`DELETE FROM remote_coder_group_bindings WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("group binding with id %s not found", id)
	}

	return nil
}

func scanGroupBinding(row *sql.Row) (*GroupProjectBinding, error) {
	var binding GroupProjectBinding
	var createdAt, updatedAt sql.NullString

	err := row.Scan(
		&binding.ID, &binding.GroupID, &binding.Platform, &binding.ProjectID,
		&binding.BotUUID, &createdAt, &updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if createdAt.Valid {
		binding.CreatedAt, _ = time.Parse(time.RFC3339, createdAt.String)
	}
	if updatedAt.Valid {
		binding.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt.String)
	}

	return &binding, nil
}

func scanGroupBindingFromRows(rows *sql.Rows) (*GroupProjectBinding, error) {
	var binding GroupProjectBinding
	var createdAt, updatedAt sql.NullString

	err := rows.Scan(
		&binding.ID, &binding.GroupID, &binding.Platform, &binding.ProjectID,
		&binding.BotUUID, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	if createdAt.Valid {
		binding.CreatedAt, _ = time.Parse(time.RFC3339, createdAt.String)
	}
	if updatedAt.Valid {
		binding.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt.String)
	}

	return &binding, nil
}
