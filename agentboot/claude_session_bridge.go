package agentboot

import (
	"github.com/tingly-dev/tingly-box/agentboot/session"
)

// newClaudeStore creates a new Claude session store
// This is a bridge function to avoid circular imports between agentboot and agentboot/session/claude
func newClaudeStore(projectsDir string) (session.Store, error) {
	// Import the claude session store package
	// We need to use a type alias or interface to avoid circular dependency
	// For the actual implementation, we need to directly instantiate the claude.Store

	// Since we can't import agentboot/session/claude from here (circular dependency),
	// we create a wrapper that will be initialized elsewhere

	// The actual implementation is done by importing the claude package in the consuming code
	// For now, we return a nil store - users should use the session/claude package directly

	// To use this properly, users should:
	// 1. Import "github.com/tingly-dev/tingly-box/agentboot/session/claude"
	// 2. Create store: store, _ := claude.NewStore(projectsDir)
	// 3. Use the session.Store interface

	return nil, nil
}
