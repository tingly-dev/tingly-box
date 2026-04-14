package agentboot

import (
	ccsession "github.com/tingly-dev/tingly-box/agentboot/claude/session"
	"github.com/tingly-dev/tingly-box/agentboot/common"
)

// NewClaudeStore creates a new Claude session store
func NewClaudeStore(projectsDir string) (common.SessionStore, error) {
	return ccsession.NewStore(projectsDir)
}
