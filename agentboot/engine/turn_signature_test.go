package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// The chat launch flags are fixed when a process starts: a change to them must
// change the signature so the resident process restarts, while per-message
// content must not.
func TestLaunchSignatureChatFlags(t *testing.T) {
	base := agentboot.ExecutionOptions{Entrypoint: "tingly-box"}

	tools := base
	tools.Tools = []string{"Read", "Glob", "Grep"}
	assert.NotEqual(t, launchSignature(base), launchSignature(tools), "Tools must be in the signature")

	restricted := base
	restricted.Restricted = true
	assert.NotEqual(t, launchSignature(base), launchSignature(restricted), "Restricted must be in the signature")

	safe := base
	safe.SafeMode = true
	assert.NotEqual(t, launchSignature(base), launchSignature(safe), "SafeMode must be in the signature")

	image := base
	image.Content = []agentboot.ContentBlock{agentboot.ImageContent("image/png", "AAAA")}
	assert.Equal(t, launchSignature(base), launchSignature(image), "per-message content must not restart the process")
}
