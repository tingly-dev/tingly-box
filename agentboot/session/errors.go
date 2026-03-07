package session

import "fmt"

// ErrSessionNotFound is returned when a session ID cannot be found
type ErrSessionNotFound struct {
	SessionID string
}

func (e ErrSessionNotFound) Error() string {
	return fmt.Sprintf("session not found: %s", e.SessionID)
}

// ErrProjectNotFound is returned when a project path has no sessions
type ErrProjectNotFound struct {
	ProjectPath string
}

func (e ErrProjectNotFound) Error() string {
	return fmt.Sprintf("project not found or no sessions: %s", e.ProjectPath)
}

// ErrInvalidSessionFormat is returned when a session file has invalid format
type ErrInvalidSessionFormat struct {
	File string
	Err  error
}

func (e ErrInvalidSessionFormat) Error() string {
	return fmt.Sprintf("invalid session format in %s: %v", e.File, e.Err)
}

// Unwrap implements the errors.Unwrap interface
func (e ErrInvalidSessionFormat) Unwrap() error {
	return e.Err
}

// ErrInvalidPath is returned when a path cannot be resolved
type ErrInvalidPath struct {
	Path string
	Err  error
}

func (e ErrInvalidPath) Error() string {
	return fmt.Sprintf("invalid path %s: %v", e.Path, e.Err)
}

// Unwrap implements the errors.Unwrap interface
func (e ErrInvalidPath) Unwrap() error {
	return e.Err
}
