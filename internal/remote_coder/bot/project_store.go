package bot

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ProjectStore handles project persistence
type ProjectStore struct {
	db *sql.DB
}

// NewProjectStore creates a new project store
// If dbPath is empty, it uses the same database as the main store
func NewProjectStore(db *sql.DB) (*ProjectStore, error) {
	if db == nil {
		return nil, fmt.Errorf("db is nil")
	}
	return &ProjectStore{db: db}, nil
}

// CreateProject creates a new project
func (s *ProjectStore) CreateProject(project *Project) error {
	if project.ID == "" {
		project.ID = uuid.New().String()
	}

	now := time.Now().UTC()
	project.CreatedAt = now
	project.UpdatedAt = now

	// Derive name from path if not set
	if project.Name == "" && project.Path != "" {
		project.Name = filepath.Base(project.Path)
	}

	_, err := s.db.Exec(`
		INSERT INTO remote_coder_projects (id, path, name, owner_id, platform, bot_uuid, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, project.ID, project.Path, project.Name, project.OwnerID, project.Platform, project.BotUUID,
		project.CreatedAt.Format(time.RFC3339), project.UpdatedAt.Format(time.RFC3339))

	return err
}

// GetProject retrieves a project by ID
func (s *ProjectStore) GetProject(id string) (*Project, error) {
	row := s.db.QueryRow(`
		SELECT id, path, name, owner_id, platform, bot_uuid, created_at, updated_at
		FROM remote_coder_projects WHERE id = ?
	`, id)

	return scanProject(row)
}

// GetProjectByPath retrieves a project by path and bot UUID
func (s *ProjectStore) GetProjectByPath(path, botUUID string) (*Project, error) {
	row := s.db.QueryRow(`
		SELECT id, path, name, owner_id, platform, bot_uuid, created_at, updated_at
		FROM remote_coder_projects WHERE path = ? AND bot_uuid = ?
	`, path, botUUID)

	return scanProject(row)
}

// ListProjectsByOwner lists all projects owned by a user on a platform
func (s *ProjectStore) ListProjectsByOwner(ownerID, platform string) ([]*Project, error) {
	rows, err := s.db.Query(`
		SELECT id, path, name, owner_id, platform, bot_uuid, created_at, updated_at
		FROM remote_coder_projects WHERE owner_id = ? AND platform = ?
		ORDER BY created_at DESC
	`, ownerID, platform)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []*Project
	for rows.Next() {
		project, err := scanProjectFromRows(rows)
		if err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}

	return projects, rows.Err()
}

// UpdateProject updates an existing project
func (s *ProjectStore) UpdateProject(project *Project) error {
	project.UpdatedAt = time.Now().UTC()

	// Derive name from path if not set
	if project.Name == "" && project.Path != "" {
		project.Name = filepath.Base(project.Path)
	}

	result, err := s.db.Exec(`
		UPDATE remote_coder_projects SET
			path = ?, name = ?, owner_id = ?, platform = ?, bot_uuid = ?, updated_at = ?
		WHERE id = ?
	`, project.Path, project.Name, project.OwnerID, project.Platform, project.BotUUID,
		project.UpdatedAt.Format(time.RFC3339), project.ID)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("project with id %s not found", project.ID)
	}

	return nil
}

// DeleteProject deletes a project by ID
func (s *ProjectStore) DeleteProject(id string) error {
	result, err := s.db.Exec(`DELETE FROM remote_coder_projects WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("project with id %s not found", id)
	}

	return nil
}

// ValidateProjectPath checks if the path exists and is accessible
func ValidateProjectPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	// Expand ~ to home directory
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot get home directory: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}

	// Check if path exists
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("path does not exist: %s", path)
		}
		return fmt.Errorf("cannot access path: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}

	return nil
}

// ExpandPath expands ~ to home directory and returns absolute path
func ExpandPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path cannot be empty")
	}

	// Expand ~ to home directory
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot get home directory: %w", err)
		}
		path = filepath.Join(home, path[2:])
	}

	// Get absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("cannot get absolute path: %w", err)
	}

	return absPath, nil
}

func scanProject(row *sql.Row) (*Project, error) {
	var project Project
	var createdAt, updatedAt sql.NullString

	err := row.Scan(
		&project.ID, &project.Path, &project.Name, &project.OwnerID,
		&project.Platform, &project.BotUUID, &createdAt, &updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if createdAt.Valid {
		project.CreatedAt, _ = time.Parse(time.RFC3339, createdAt.String)
	}
	if updatedAt.Valid {
		project.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt.String)
	}

	return &project, nil
}

func scanProjectFromRows(rows *sql.Rows) (*Project, error) {
	var project Project
	var createdAt, updatedAt sql.NullString

	err := rows.Scan(
		&project.ID, &project.Path, &project.Name, &project.OwnerID,
		&project.Platform, &project.BotUUID, &createdAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	if createdAt.Valid {
		project.CreatedAt, _ = time.Parse(time.RFC3339, createdAt.String)
	}
	if updatedAt.Valid {
		project.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt.String)
	}

	return &project, nil
}
