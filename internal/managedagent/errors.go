package managedagent

import (
	"errors"
	"fmt"
)

// ErrNotFound is returned by stores and the Service for a missing id. It
// wraps so callers can errors.Is it and still read which entity was missing.
var ErrNotFound = errors.New("not found")

// ErrConflict means the operation is not allowed in the entity's current
// state (deleting an environment that still has workspaces, steering an
// archived session, ...).
var ErrConflict = errors.New("conflict")

// ErrValidation is a rejected input. HTTP maps it to 400.
var ErrValidation = errors.New("validation")

func notFound(entity, id string) error {
	return fmt.Errorf("%s %s: %w", entity, id, ErrNotFound)
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), ErrValidation)
}

func conflict(format string, args ...any) error {
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), ErrConflict)
}
